package zoa

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func templateTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestTemplateRegistry_LoadFromDir(t *testing.T) {
	dir := t.TempDir()
	content := `name: get_nodes
scope: kube-api
type: read
description: List all nodes
rbac:
  cluster_scoped: true
  rules:
    - apiGroups: [""]
      resources: ["nodes"]
      verbs: ["get", "list"]
script: |
  kubectl get nodes -o json > /artifacts/output.json
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_nodes.yaml"), []byte(content), 0644))

	registry := NewTemplateRegistry(templateTestLogger())
	err := registry.LoadFromDir(dir)
	require.NoError(t, err)

	tmpl, ok := registry.Get("get_nodes")
	assert.True(t, ok)
	assert.Equal(t, "get_nodes", tmpl.Name)
	assert.Equal(t, "kube-api", tmpl.Scope)
	assert.Equal(t, "read", tmpl.Type)
	assert.Equal(t, "List all nodes", tmpl.Description)
	assert.NotNil(t, tmpl.RBAC)
	assert.True(t, tmpl.RBAC.ClusterScoped)
	assert.NotEmpty(t, tmpl.Script)
}

func TestTemplateRegistry_LoadFromDir_Empty(t *testing.T) {
	dir := t.TempDir()

	registry := NewTemplateRegistry(templateTestLogger())
	err := registry.LoadFromDir(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid templates found")
}

func TestTemplateRegistry_LoadFromDir_MissingScript(t *testing.T) {
	dir := t.TempDir()
	content := `name: bad_template
scope: kube-api
type: read
description: Missing script
rbac:
  cluster_scoped: true
  rules:
    - apiGroups: [""]
      resources: ["nodes"]
      verbs: ["get"]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(content), 0644))

	registry := NewTemplateRegistry(templateTestLogger())
	err := registry.LoadFromDir(dir)
	assert.Error(t, err)
}

func TestBuildManifest_ClusterScoped(t *testing.T) {
	tmpl := &TATemplate{
		Name:  "get_nodes",
		Scope: "kube-api",
		Type:  "read",
		RBAC: &TARBAC{
			ClusterScoped: true,
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"get", "list"}},
			},
		},
		Script: "kubectl get nodes -o json > /artifacts/output.json\n",
	}

	ctx := RenderContext{
		ExecID:        "abc123",
		ActionName:    "get_nodes",
		TargetCluster: "local-cluster",
		Namespace:     "zoa-jobs",
		OutputBucket:  "my-bucket",
		Operator:      "slopezma",
		Revision:      "a1b2c3d",
		Type:          "read",
		Scope:         "kube-api",
		Params:        nil,
		Config: JobConfig{
			Image:            "quay.io/test/zoa-tools:latest",
			CPURequest:       "100m",
			MemoryRequest:    "128Mi",
			CPULimit:         "500m",
			MemoryLimit:      "512Mi",
			TTLSeconds:       3600,
			EntrypointScript: "#!/bin/bash\n/zoa/run.sh\n",
		},
	}

	hfm, err := BuildManifest(tmpl, ctx)
	require.NoError(t, err)

	assert.Equal(t, "zoa-abc123", hfm.Name)
	assert.Equal(t, "local-cluster", hfm.Spec.ManagementCluster)
	// SA + ClusterRole + ClusterRoleBinding + OutputCM + OutputRole + OutputRoleBinding + UploaderRole + UploaderRoleBinding + ScriptCM + RunnerJob + UploadJob = 11
	assert.Len(t, hfm.Spec.Resources, 11)
	// Runner and upload jobs should have Watch: true
	watchCount := 0
	for _, r := range hfm.Spec.Resources {
		if r.Watch {
			watchCount++
		}
	}
	assert.Equal(t, 2, watchCount)
	assert.Equal(t, "slopezma", hfm.Labels[labelOperator])
	assert.Equal(t, "a1b2c3d", hfm.Labels[labelRevision])
}

func TestBuildManifest_NamespaceScoped(t *testing.T) {
	tmpl := &TATemplate{
		Name:  "get_pods",
		Scope: "kube-api",
		Type:  "read",
		Params: []TAParameter{
			{Name: "namespace", Required: true},
		},
		RBAC: &TARBAC{
			ClusterScoped:  false,
			NamespaceParam: "namespace",
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
			},
		},
		Script: "kubectl get pods -n ${PARAM_NAMESPACE} -o json > /artifacts/output.json\n",
	}

	ctx := RenderContext{
		ExecID:        "def456",
		ActionName:    "get_pods",
		TargetCluster: "mc01",
		Namespace:     "zoa-jobs",
		OutputBucket:  "bucket",
		Operator:      "user1",
		Revision:      "HEAD",
		Type:          "read",
		Scope:         "kube-api",
		Params:        map[string]string{"namespace": "default"},
		Config: JobConfig{
			Image:            "quay.io/test/zoa-tools:latest",
			CPURequest:       "100m",
			MemoryRequest:    "128Mi",
			CPULimit:         "500m",
			MemoryLimit:      "512Mi",
			TTLSeconds:       3600,
			EntrypointScript: "#!/bin/bash\n/zoa/run.sh\n",
		},
	}

	hfm, err := BuildManifest(tmpl, ctx)
	require.NoError(t, err)

	assert.Equal(t, "zoa-def456", hfm.Name)
	assert.Equal(t, "mc01", hfm.Spec.ManagementCluster)
	// SA + Role + RoleBinding + OutputCM + OutputRole + OutputRoleBinding + UploaderRole + UploaderRoleBinding + ScriptCM + RunnerJob + UploadJob = 11
	assert.Len(t, hfm.Spec.Resources, 11)
}

func TestBuildManifest_AWSScope_NoSAManifest(t *testing.T) {
	tmpl := &TATemplate{
		Name:   "describe_instance",
		Scope:  "aws-api",
		Type:   "read",
		RBAC:   nil,
		Script: "aws ec2 describe-instances > /artifacts/output.json\n",
	}

	ctx := RenderContext{
		ExecID:        "aws789",
		ActionName:    "describe_instance",
		TargetCluster: "mc01",
		Namespace:     "zoa-jobs",
		OutputBucket:  "bucket",
		Operator:      "slopezma",
		Revision:      "abc",
		Type:          "read",
		Scope:         "aws-api",
		Params:        nil,
		Config: JobConfig{
			Image:            "quay.io/test/zoa-tools:latest",
			CPURequest:       "100m",
			MemoryRequest:    "128Mi",
			CPULimit:         "500m",
			MemoryLimit:      "512Mi",
			TTLSeconds:       3600,
			EntrypointScript: "#!/bin/bash\n/zoa/run.sh\n",
		},
	}

	hfm, err := BuildManifest(tmpl, ctx)
	require.NoError(t, err)

	// No SA manifest (static SA pre-provisioned), no RBAC from template (nil):
	// OutputCM + OutputRole + OutputRoleBinding + UploaderRole + UploaderRoleBinding + ScriptCM + RunnerJob + UploadJob = 8
	assert.Len(t, hfm.Spec.Resources, 8)
}

func TestLoadJobConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image"), []byte("quay.io/test/tools:v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cpu_request"), []byte("200m"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "memory_request"), []byte("256Mi"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cpu_limit"), []byte("1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "memory_limit"), []byte("1Gi"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ttl_seconds"), []byte("7200"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entrypoint.sh"), []byte("#!/bin/bash\n/zoa/run.sh\n"), 0644))

	cfg, err := LoadJobConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/test/tools:v1", cfg.Image)
	assert.Equal(t, "200m", cfg.CPURequest)
	assert.Equal(t, "256Mi", cfg.MemoryRequest)
	assert.Equal(t, "1", cfg.CPULimit)
	assert.Equal(t, "1Gi", cfg.MemoryLimit)
	assert.Equal(t, int32(7200), cfg.TTLSeconds)
	assert.Contains(t, cfg.EntrypointScript, "/zoa/run.sh")
}

func TestLoadJobConfig_MissingImage(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entrypoint.sh"), []byte("#!/bin/bash\n"), 0644))

	_, err := LoadJobConfig(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image")
}

func TestLoadJobConfig_DynamoDBTTLDays(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image"), []byte("quay.io/test/tools:v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entrypoint.sh"), []byte("#!/bin/bash\n/zoa/run.sh\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dynamodb_ttl_days"), []byte("90"), 0644))

	cfg, err := LoadJobConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, 90, cfg.DynamoDBTTLDays)
}

func TestLoadJobConfig_DynamoDBTTLDays_Default(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image"), []byte("quay.io/test/tools:v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entrypoint.sh"), []byte("#!/bin/bash\n/zoa/run.sh\n"), 0644))

	cfg, err := LoadJobConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, 365, cfg.DynamoDBTTLDays)
}

func TestLoadJobConfig_DynamoDBTTLDays_InvalidIgnored(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image"), []byte("quay.io/test/tools:v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entrypoint.sh"), []byte("#!/bin/bash\n/zoa/run.sh\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dynamodb_ttl_days"), []byte("not-a-number"), 0644))

	cfg, err := LoadJobConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, 365, cfg.DynamoDBTTLDays)
}

func TestLoadJobConfig_DynamoDBTTLDays_ZeroIgnored(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image"), []byte("quay.io/test/tools:v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entrypoint.sh"), []byte("#!/bin/bash\n/zoa/run.sh\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dynamodb_ttl_days"), []byte("0"), 0644))

	cfg, err := LoadJobConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, 365, cfg.DynamoDBTTLDays)
}

func TestTemplateRegistry_Get_NotFound(t *testing.T) {
	registry := NewTemplateRegistry(templateTestLogger())
	registry.templates["exists"] = &TATemplate{Name: "exists", Scope: "kube-api", Script: "echo"}

	_, ok := registry.Get("nonexistent")
	assert.False(t, ok)
}

func TestTemplateRegistry_List(t *testing.T) {
	registry := NewTemplateRegistry(templateTestLogger())
	registry.templates["action_a"] = &TATemplate{Name: "action_a", Scope: "kube-api", Script: "echo a"}
	registry.templates["action_b"] = &TATemplate{Name: "action_b", Scope: "kube-api", Script: "echo b"}

	names := registry.List()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "action_a")
	assert.Contains(t, names, "action_b")
}

func TestTemplateRegistry_ListAll(t *testing.T) {
	registry := NewTemplateRegistry(templateTestLogger())
	registry.templates["action_a"] = &TATemplate{Name: "action_a", Scope: "kube-api", Script: "echo a"}
	registry.templates["action_b"] = &TATemplate{Name: "action_b", Scope: "aws-api", Script: "echo b"}

	all := registry.ListAll()
	assert.Len(t, all, 2)
}

func TestIsRunnerSADynamic(t *testing.T) {
	assert.True(t, isRunnerSADynamic("kube-api"))
	assert.True(t, isRunnerSADynamic("custom"))
	assert.True(t, isRunnerSADynamic(""))
	assert.False(t, isRunnerSADynamic("aws-api"))
}

func TestScopeTypeToRunnerSA(t *testing.T) {
	tests := []struct {
		name     string
		scope    string
		taType   string
		execID   string
		expected string
	}{
		{"kube-api read uses per-exec SA", "kube-api", "read", "abc123", "zoa-runner-abc123"},
		{"kube-api write uses per-exec SA", "kube-api", "write", "def456", "zoa-runner-def456"},
		{"aws-api read uses static SA", "aws-api", "read", "xyz789", "zoa-aws-read"},
		{"aws-api write uses static SA", "aws-api", "write", "xyz789", "zoa-aws-write"},
		{"unknown scope uses per-exec SA", "custom", "read", "abc123", "zoa-runner-abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, scopeTypeToRunnerSA(tt.scope, tt.taType, tt.execID))
		})
	}
}
