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
description: List all nodes
manifests:
  - apiVersion: v1
    kind: ServiceAccount
    metadata:
      name: zoa-get-nodes-{{ .ExecID }}
      namespace: {{ .Namespace }}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_nodes.yaml"), []byte(content), 0644))

	registry := NewTemplateRegistry(templateTestLogger())
	err := registry.LoadFromDir(dir)
	require.NoError(t, err)

	tmpl, ok := registry.Get("get_nodes")
	assert.True(t, ok)
	assert.Equal(t, "get_nodes", tmpl.Name)
	assert.Equal(t, "kube-api", tmpl.Scope)
	assert.Equal(t, "List all nodes", tmpl.Description)
}

func TestTemplateRegistry_LoadFromDir_Empty(t *testing.T) {
	dir := t.TempDir()

	registry := NewTemplateRegistry(templateTestLogger())
	err := registry.LoadFromDir(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid templates found")
}

func TestTemplateRegistry_LoadFromDir_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("not: valid: yaml: ["), 0644))

	registry := NewTemplateRegistry(templateTestLogger())
	err := registry.LoadFromDir(dir)
	assert.Error(t, err)
}

func TestTATemplate_BuildManifestWork(t *testing.T) {
	dir := t.TempDir()
	content := `name: get_nodes
scope: kube-api
description: List nodes
manifests:
  - apiVersion: v1
    kind: ServiceAccount
    metadata:
      name: zoa-get-nodes-{{ .ExecID }}
      namespace: {{ .Namespace }}
  - apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    metadata:
      name: zoa-get-nodes-{{ .ExecID }}
    rules:
      - apiGroups: [""]
        resources: ["nodes"]
        verbs: ["get", "list"]
  - apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRoleBinding
    metadata:
      name: zoa-get-nodes-{{ .ExecID }}
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: ClusterRole
      name: zoa-get-nodes-{{ .ExecID }}
    subjects:
      - kind: ServiceAccount
        name: zoa-get-nodes-{{ .ExecID }}
        namespace: {{ .Namespace }}
  - apiVersion: batch/v1
    kind: Job
    metadata:
      name: zoa-get-nodes-{{ .ExecID }}
      namespace: {{ .Namespace }}
    spec:
      backoffLimit: 0
      template:
        spec:
          serviceAccountName: zoa-get-nodes-{{ .ExecID }}
          restartPolicy: Never
          containers:
            - name: ta
              image: amazon/aws-cli:latest
              command: ["kubectl", "get", "nodes"]
              env:
                - name: S3_BUCKET
                  value: "{{ .OutputBucket }}"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_nodes.yaml"), []byte(content), 0644))

	registry := NewTemplateRegistry(templateTestLogger())
	require.NoError(t, registry.LoadFromDir(dir))

	tmpl, ok := registry.Get("get_nodes")
	require.True(t, ok)

	ctx := RenderContext{
		ExecID:        "abc123",
		ActionName:    "get_nodes",
		TargetCluster: "local-cluster",
		Namespace:     "zoa-jobs",
		OutputBucket:  "my-bucket",
		JobRoleARN:    "arn:aws:iam::123:role/job",
		Params:        nil,
	}

	mw, err := tmpl.BuildManifestWork(ctx)
	require.NoError(t, err)

	assert.Equal(t, "zoa-abc123", mw.Name)
	assert.Equal(t, "local-cluster", mw.Namespace)
	assert.Len(t, mw.Spec.Workload.Manifests, 4)
	require.Len(t, mw.Spec.ManifestConfigs, 1)
	assert.Equal(t, "zoa-get-nodes-abc123", mw.Spec.ManifestConfigs[0].ResourceIdentifier.Name)
	assert.Equal(t, "zoa-jobs", mw.Spec.ManifestConfigs[0].ResourceIdentifier.Namespace)
}

func TestTATemplate_BuildManifestWork_InvalidTemplate(t *testing.T) {
	tmpl := &TATemplate{
		Name:        "bad",
		rawTemplate: `{{ .InvalidFunc | badPipe }}`,
	}

	ctx := RenderContext{ExecID: "test"}
	_, err := tmpl.BuildManifestWork(ctx)
	assert.Error(t, err)
}

func TestTemplateRegistry_Get_NotFound(t *testing.T) {
	registry := NewTemplateRegistry(templateTestLogger())
	registry.templates["exists"] = &TATemplate{Name: "exists"}

	_, ok := registry.Get("nonexistent")
	assert.False(t, ok)
}

func TestTemplateRegistry_List(t *testing.T) {
	registry := NewTemplateRegistry(templateTestLogger())
	registry.templates["action_a"] = &TATemplate{Name: "action_a"}
	registry.templates["action_b"] = &TATemplate{Name: "action_b"}

	names := registry.List()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "action_a")
	assert.Contains(t, names, "action_b")
}
