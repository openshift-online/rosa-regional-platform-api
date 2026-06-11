package zoa

import (
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	JobNamespace = "zoa-jobs"

	hcpNamespacePrefix = "clusters-"

	labelPrefix    = "zoa.rosa.io/"
	labelExecID    = labelPrefix + "execution-id"
	labelAction    = labelPrefix + "action"
	labelType      = labelPrefix + "type"
	labelScope     = labelPrefix + "scope"
	labelOperator  = labelPrefix + "operator"
	labelRevision  = labelPrefix + "revision"
	labelTarget    = labelPrefix + "target-cluster"
	labelManaged   = labelPrefix + "managed"
	labelRole      = labelPrefix + "role"
	annotCreatedAt = labelPrefix + "created-at"

	uploaderSAName = "zoa-uploader"
)

// BuildManifestWork generates a complete ManifestWork with two Jobs (runner + uploader).
func BuildManifestWork(tmpl *TATemplate, ctx RenderContext) (*workv1.ManifestWork, error) {
	if err := validateSecretsPolicy(tmpl, ctx); err != nil {
		return nil, err
	}

	labels := buildLabels(ctx)
	manifests := make([]workv1.Manifest, 0, 10)

	runnerSAName := scopeTypeToRunnerSA(ctx.Scope, ctx.Type, ctx.ExecID)

	// Only create the SA manifest for dynamic (per-execution) SAs.
	// Static SAs (zoa-aws-read, zoa-aws-write) are pre-provisioned via ArgoCD
	// and must not be lifecycle-managed by individual ManifestWorks.
	if isRunnerSADynamic(ctx.Scope) {
		saManifest, err := buildServiceAccount(runnerSAName, ctx, labels)
		if err != nil {
			return nil, fmt.Errorf("building service account: %w", err)
		}
		manifests = append(manifests, saManifest)
	}

	rbacManifests, err := buildRBACManifests(tmpl, ctx, runnerSAName, labels)
	if err != nil {
		return nil, fmt.Errorf("building RBAC manifests: %w", err)
	}
	manifests = append(manifests, rbacManifests...)

	outputCMManifest, err := buildOutputConfigMap(ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building output configmap: %w", err)
	}
	manifests = append(manifests, outputCMManifest)

	outputRBACManifests, err := buildOutputRBAC(ctx, runnerSAName, labels)
	if err != nil {
		return nil, fmt.Errorf("building output RBAC: %w", err)
	}
	manifests = append(manifests, outputRBACManifests...)

	scriptCMManifest, err := buildScriptConfigMap(tmpl, ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building script configmap: %w", err)
	}
	manifests = append(manifests, scriptCMManifest)

	runnerJobManifest, err := buildRunnerJob(tmpl, ctx, runnerSAName, labels)
	if err != nil {
		return nil, fmt.Errorf("building runner job: %w", err)
	}
	manifests = append(manifests, runnerJobManifest)

	uploadJobManifest, err := buildUploadJob(ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building upload job: %w", err)
	}
	manifests = append(manifests, uploadJobManifest)

	runnerJobName := "zoa-" + ctx.ExecID
	uploadJobName := "zoa-" + ctx.ExecID + "-upload"

	mw := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "work.open-cluster-management.io/v1",
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      runnerJobName,
			Namespace: ctx.TargetCluster,
			Labels:    labels,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
			ManifestConfigs: []workv1.ManifestConfigOption{
				{
					ResourceIdentifier: workv1.ResourceIdentifier{
						Group:     "batch",
						Resource:  "jobs",
						Name:      runnerJobName,
						Namespace: ctx.Namespace,
					},
					FeedbackRules: []workv1.FeedbackRule{
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{Name: "taSucceeded", Path: ".status.succeeded"},
								{Name: "taFailed", Path: ".status.failed"},
								{Name: "runnerStartTime", Path: ".status.startTime"},
								{Name: "runnerCompletionTime", Path: ".status.completionTime"},
							},
						},
					},
				},
				{
					ResourceIdentifier: workv1.ResourceIdentifier{
						Group:     "batch",
						Resource:  "jobs",
						Name:      uploadJobName,
						Namespace: ctx.Namespace,
					},
					FeedbackRules: []workv1.FeedbackRule{
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{Name: "uploadSucceeded", Path: ".status.succeeded"},
								{Name: "uploadFailed", Path: ".status.failed"},
								{Name: "uploadCompletionTime", Path: ".status.completionTime"},
							},
						},
					},
				},
			},
		},
	}

	return mw, nil
}

func buildLabels(ctx RenderContext) map[string]string {
	return map[string]string{
		labelExecID:   ctx.ExecID,
		labelAction:   ctx.ActionName,
		labelType:     ctx.Type,
		labelScope:    ctx.Scope,
		labelOperator: ctx.Operator,
		labelRevision: ctx.Revision,
		labelTarget:   ctx.TargetCluster,
		labelManaged:  "true",
	}
}

// scopeTypeToRunnerSA derives the runner ServiceAccount name from scope + type.
// For kube-api TAs, a per-execution SA is created. For AWS TAs, static SAs are used.
func scopeTypeToRunnerSA(scope, taType, execID string) string {
	switch scope {
	case "aws":
		if taType == "write" {
			return "zoa-aws-write"
		}
		return "zoa-aws-read"
	default:
		return "zoa-runner-" + execID
	}
}

// isRunnerSADynamic returns true when the runner SA is per-execution (created and
// destroyed with the ManifestWork). Static SAs (aws scope) are pre-provisioned.
func isRunnerSADynamic(scope string) bool {
	return scope != "aws"
}

func buildServiceAccount(saName string, ctx RenderContext, labels map[string]string) (workv1.Manifest, error) {
	saLabels := copyLabels(labels)
	saLabels[labelRole] = "runner"

	sa := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata": map[string]interface{}{
			"name":      saName,
			"namespace": ctx.Namespace,
			"labels":    saLabels,
		},
	}

	return toManifest(sa)
}

func buildRBACManifests(tmpl *TATemplate, ctx RenderContext, saName string, labels map[string]string) ([]workv1.Manifest, error) {
	if tmpl.RBAC == nil || len(tmpl.RBAC.Rules) == 0 {
		return nil, nil
	}

	manifests := make([]workv1.Manifest, 0, 2)
	roleName := fmt.Sprintf("zoa-%s-%s", tmpl.Name, ctx.ExecID)

	rules := make([]map[string]interface{}, 0, len(tmpl.RBAC.Rules))
	for _, r := range tmpl.RBAC.Rules {
		rules = append(rules, map[string]interface{}{
			"apiGroups": r.APIGroups,
			"resources": r.Resources,
			"verbs":     r.Verbs,
		})
	}

	if tmpl.RBAC.ClusterScoped {
		role := map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name":   roleName,
				"labels": labels,
			},
			"rules": rules,
		}
		m, err := toManifest(role)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)

		binding := map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name":   roleName,
				"labels": labels,
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     roleName,
			},
			"subjects": []map[string]interface{}{
				{
					"kind":      "ServiceAccount",
					"name":      saName,
					"namespace": ctx.Namespace,
				},
			},
		}
		m, err = toManifest(binding)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
	} else {
		targetNS := resolveTargetNamespace(tmpl, ctx)

		role := map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"name":      roleName,
				"namespace": targetNS,
				"labels":    labels,
			},
			"rules": rules,
		}
		m, err := toManifest(role)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)

		binding := map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":      roleName,
				"namespace": targetNS,
				"labels":    labels,
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     roleName,
			},
			"subjects": []map[string]interface{}{
				{
					"kind":      "ServiceAccount",
					"name":      saName,
					"namespace": ctx.Namespace,
				},
			},
		}
		m, err = toManifest(binding)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
	}

	return manifests, nil
}

// buildOutputConfigMap creates the empty output ConfigMap that the runner writes to.
func buildOutputConfigMap(ctx RenderContext, labels map[string]string) (workv1.Manifest, error) {
	cmLabels := copyLabels(labels)
	cmLabels[labelRole] = "output"

	cm := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "zoa-output-" + ctx.ExecID,
			"namespace": ctx.Namespace,
			"labels":    cmLabels,
		},
		"data": map[string]interface{}{},
	}

	return toManifest(cm)
}

// buildOutputRBAC grants the runner SA permission to patch its output ConfigMap.
func buildOutputRBAC(ctx RenderContext, saName string, labels map[string]string) ([]workv1.Manifest, error) {
	roleName := fmt.Sprintf("zoa-output-%s", ctx.ExecID)

	role := map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
		"metadata": map[string]interface{}{
			"name":      roleName,
			"namespace": ctx.Namespace,
			"labels":    labels,
		},
		"rules": []map[string]interface{}{
			{
				"apiGroups":     []string{""},
				"resources":     []string{"configmaps"},
				"verbs":         []string{"get", "patch"},
				"resourceNames": []string{"zoa-output-" + ctx.ExecID},
			},
		},
	}
	roleManifest, err := toManifest(role)
	if err != nil {
		return nil, err
	}

	binding := map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "RoleBinding",
		"metadata": map[string]interface{}{
			"name":      roleName,
			"namespace": ctx.Namespace,
			"labels":    labels,
		},
		"roleRef": map[string]interface{}{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "Role",
			"name":     roleName,
		},
		"subjects": []map[string]interface{}{
			{
				"kind":      "ServiceAccount",
				"name":      saName,
				"namespace": ctx.Namespace,
			},
		},
	}
	bindingManifest, err := toManifest(binding)
	if err != nil {
		return nil, err
	}

	return []workv1.Manifest{roleManifest, bindingManifest}, nil
}

func buildScriptConfigMap(tmpl *TATemplate, ctx RenderContext, labels map[string]string) (workv1.Manifest, error) {
	data := map[string]interface{}{
		"entrypoint.sh": ctx.Config.EntrypointScript,
		"run.sh":        tmpl.Script,
	}
	if ctx.Config.UploadEntrypointScript != "" {
		data["upload_entrypoint.sh"] = ctx.Config.UploadEntrypointScript
	}

	cm := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "zoa-scripts-" + ctx.ExecID,
			"namespace": ctx.Namespace,
			"labels":    labels,
		},
		"data": data,
	}

	return toManifest(cm)
}

// buildRunnerJob creates the TA runner Job. It no longer uploads to S3;
// output is written to the output ConfigMap.
func buildRunnerJob(tmpl *TATemplate, ctx RenderContext, saName string, labels map[string]string) (workv1.Manifest, error) {
	jobName := "zoa-" + ctx.ExecID

	jobLabels := copyLabels(labels)
	jobLabels[labelRole] = "runner"

	envVars := []map[string]interface{}{
		{"name": "RUN_ID", "value": ctx.ExecID},
		{"name": "JOB_NAME", "value": jobName},
		{"name": "JOB_NAMESPACE", "value": ctx.Namespace},
		{"name": "CLUSTER_ID", "value": ctx.TargetCluster},
		{"name": "ACTION_NAME", "value": ctx.ActionName},
		{"name": "OPERATOR", "value": ctx.Operator},
		{"name": "REVISION", "value": ctx.Revision},
		{"name": "TYPE", "value": ctx.Type},
		{"name": "SCOPE", "value": ctx.Scope},
		{"name": "OUTPUT_CONFIGMAP", "value": "zoa-output-" + ctx.ExecID},
	}

	for _, p := range tmpl.Params {
		envName := "PARAM_" + strings.ToUpper(strings.ReplaceAll(p.Name, "-", "_"))
		value := p.Default
		if v, ok := ctx.Params[p.Name]; ok && v != "" {
			value = v
		}
		envVars = append(envVars, map[string]interface{}{
			"name":  envName,
			"value": value,
		})
	}

	job := map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      jobName,
			"namespace": ctx.Namespace,
			"labels":    jobLabels,
		},
		"spec": map[string]interface{}{
			"ttlSecondsAfterFinished": ctx.Config.TTLSeconds,
			"backoffLimit":            0,
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": jobLabels,
				},
				"spec": map[string]interface{}{
					"serviceAccountName": saName,
					"restartPolicy":      "Never",
					"containers": []map[string]interface{}{
						{
							"name":    "ta",
							"image":   ctx.Config.Image,
							"command": []string{"/bin/bash", "/zoa/entrypoint.sh"},
							"env":     envVars,
							"volumeMounts": []map[string]interface{}{
								{"name": "artifacts", "mountPath": "/artifacts"},
								{"name": "zoa-scripts", "mountPath": "/zoa"},
							},
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"cpu":    ctx.Config.CPURequest,
									"memory": ctx.Config.MemoryRequest,
								},
								"limits": map[string]interface{}{
									"cpu":    ctx.Config.CPULimit,
									"memory": ctx.Config.MemoryLimit,
								},
							},
							"securityContext": map[string]interface{}{
								"runAsNonRoot": true,
							},
						},
					},
					"volumes": []map[string]interface{}{
						{"name": "artifacts", "emptyDir": map[string]interface{}{}},
						{
							"name": "zoa-scripts",
							"configMap": map[string]interface{}{
								"name":        "zoa-scripts-" + ctx.ExecID,
								"defaultMode": 0o755,
							},
						},
					},
				},
			},
		},
	}

	return toManifest(job)
}

// buildUploadJob creates the S3 uploader Job. It waits for the runner to write output
// to the ConfigMap, then uploads to S3.
func buildUploadJob(ctx RenderContext, labels map[string]string) (workv1.Manifest, error) {
	jobName := "zoa-" + ctx.ExecID + "-upload"

	jobLabels := copyLabels(labels)
	jobLabels[labelRole] = "uploader"

	uploadTimeout := ctx.Config.UploadTimeoutSeconds
	if uploadTimeout == 0 {
		uploadTimeout = 120
	}

	envVars := []map[string]interface{}{
		{"name": "RUN_ID", "value": ctx.ExecID},
		{"name": "JOB_NAMESPACE", "value": ctx.Namespace},
		{"name": "ARTIFACT_BUCKET", "value": ctx.OutputBucket},
		{"name": "OUTPUT_CONFIGMAP", "value": "zoa-output-" + ctx.ExecID},
		{"name": "RUNNER_JOB_NAME", "value": "zoa-" + ctx.ExecID},
		{"name": "UPLOAD_TIMEOUT", "value": fmt.Sprintf("%d", uploadTimeout)},
	}

	job := map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      jobName,
			"namespace": ctx.Namespace,
			"labels":    jobLabels,
		},
		"spec": map[string]interface{}{
			"ttlSecondsAfterFinished": ctx.Config.TTLSeconds,
			"backoffLimit":            0,
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": jobLabels,
				},
				"spec": map[string]interface{}{
					"serviceAccountName": uploaderSAName,
					"restartPolicy":      "Never",
					"containers": []map[string]interface{}{
						{
							"name":    "uploader",
							"image":   ctx.Config.Image,
							"command": []string{"/bin/bash", "/zoa/upload_entrypoint.sh"},
							"env":     envVars,
							"volumeMounts": []map[string]interface{}{
								{"name": "zoa-scripts", "mountPath": "/zoa"},
							},
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"cpu":    "50m",
									"memory": "64Mi",
								},
								"limits": map[string]interface{}{
									"cpu":    "200m",
									"memory": "128Mi",
								},
							},
							"securityContext": map[string]interface{}{
								"runAsNonRoot": true,
							},
						},
					},
					"volumes": []map[string]interface{}{
						{
							"name": "zoa-scripts",
							"configMap": map[string]interface{}{
								"name":        "zoa-scripts-" + ctx.ExecID,
								"defaultMode": 0o755,
							},
						},
					},
				},
			},
		},
	}

	return toManifest(job)
}

func resolveTargetNamespace(tmpl *TATemplate, ctx RenderContext) string {
	if tmpl.RBAC != nil && tmpl.RBAC.NamespaceParam != "" {
		if ns, ok := ctx.Params[tmpl.RBAC.NamespaceParam]; ok && ns != "" {
			return ns
		}
	}
	return ctx.Namespace
}

// validateSecretsPolicy enforces that no TA can access secrets in HCP namespaces (clusters-*).
func validateSecretsPolicy(tmpl *TATemplate, ctx RenderContext) error {
	if tmpl.RBAC == nil || len(tmpl.RBAC.Rules) == 0 {
		return nil
	}

	if tmpl.RBAC.ClusterScoped {
		return nil
	}

	targetNS := resolveTargetNamespace(tmpl, ctx)
	if !strings.HasPrefix(targetNS, hcpNamespacePrefix) {
		return nil
	}

	for _, rule := range tmpl.RBAC.Rules {
		if ruleGrantsSecrets(rule) {
			return fmt.Errorf("secrets access denied: namespace %q is an HCP namespace (prefix %q)", targetNS, hcpNamespacePrefix)
		}
	}
	return nil
}

func ruleGrantsSecrets(rule RBACRule) bool {
	for _, res := range rule.Resources {
		r := strings.ToLower(res)
		if r == "secrets" || r == "*" {
			return true
		}
	}
	return false
}

func copyLabels(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src)+1)
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func toManifest(obj interface{}) (workv1.Manifest, error) {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return workv1.Manifest{}, fmt.Errorf("marshaling to JSON: %w", err)
	}
	return workv1.Manifest{
		RawExtension: runtime.RawExtension{Raw: jsonBytes},
	}, nil
}
