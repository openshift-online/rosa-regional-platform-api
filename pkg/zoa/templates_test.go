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
profile: kube
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
	assert.Equal(t, "kube", tmpl.Profile)
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
profile: kube
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

func TestBuildManifestWork_ClusterScoped(t *testing.T) {
	tmpl := &TATemplate{
		Name:    "get_nodes",
		Profile: "kube",
		Scope:   "kube-api",
		Type:    "read",
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
		Profile:       "kube",
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

	mw, err := BuildManifestWork(tmpl, ctx)
	require.NoError(t, err)

	assert.Equal(t, "zoa-abc123", mw.Name)
	assert.Equal(t, "local-cluster", mw.Namespace)
	// ClusterRole + ClusterRoleBinding + ConfigMap + Job = 4 manifests
	assert.Len(t, mw.Spec.Workload.Manifests, 4)
	require.Len(t, mw.Spec.ManifestConfigs, 1)
	assert.Equal(t, "zoa-abc123", mw.Spec.ManifestConfigs[0].ResourceIdentifier.Name)
	assert.Equal(t, "zoa-jobs", mw.Spec.ManifestConfigs[0].ResourceIdentifier.Namespace)
	assert.Equal(t, "slopezma", mw.Labels[labelOperator])
	assert.Equal(t, "a1b2c3d", mw.Labels[labelRevision])
}

func TestBuildManifestWork_NamespaceScoped(t *testing.T) {
	tmpl := &TATemplate{
		Name:    "get_pods",
		Profile: "kube",
		Scope:   "kube-api",
		Type:    "read",
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
		Profile:       "kube",
		Type:          "read",
		Scope:         "kube-api",
		Params:        map[string]string{"namespace": "maestro"},
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

	mw, err := BuildManifestWork(tmpl, ctx)
	require.NoError(t, err)

	assert.Equal(t, "zoa-def456", mw.Name)
	assert.Equal(t, "mc01", mw.Namespace)
	// Role + RoleBinding + ConfigMap + Job = 4 manifests
	assert.Len(t, mw.Spec.Workload.Manifests, 4)
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

func TestTemplateRegistry_Get_NotFound(t *testing.T) {
	registry := NewTemplateRegistry(templateTestLogger())
	registry.templates["exists"] = &TATemplate{Name: "exists", Profile: "kube", Script: "echo"}

	_, ok := registry.Get("nonexistent")
	assert.False(t, ok)
}

func TestTemplateRegistry_List(t *testing.T) {
	registry := NewTemplateRegistry(templateTestLogger())
	registry.templates["action_a"] = &TATemplate{Name: "action_a", Profile: "kube", Script: "echo a"}
	registry.templates["action_b"] = &TATemplate{Name: "action_b", Profile: "kube", Script: "echo b"}

	names := registry.List()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "action_a")
	assert.Contains(t, names, "action_b")
}

func TestTemplateRegistry_ListAll(t *testing.T) {
	registry := NewTemplateRegistry(templateTestLogger())
	registry.templates["action_a"] = &TATemplate{Name: "action_a", Profile: "kube", Script: "echo a"}
	registry.templates["action_b"] = &TATemplate{Name: "action_b", Profile: "aws-read", Script: "echo b"}

	all := registry.ListAll()
	assert.Len(t, all, 2)
}

func TestProfileToSAName(t *testing.T) {
	tests := []struct {
		profile  string
		expected string
	}{
		{"kube", "zoa-kube-sa"},
		{"aws-read", "zoa-aws-read-sa"},
		{"aws-write", "zoa-aws-write-sa"},
		{"breakglass-read", "zoa-breakglass-read-sa"},
		{"breakglass-write", "zoa-breakglass-write-sa"},
		{"unknown", "zoa-kube-sa"},
	}

	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			assert.Equal(t, tt.expected, profileToSAName(tt.profile))
		})
	}
}
