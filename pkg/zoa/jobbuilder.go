package zoa

import (
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hyperfleetv1alpha1 "github.com/typeid/hyperfleet-operator/api/v1alpha1"
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

// BuildManifest generates a Manifest CR with two Jobs (runner + uploader)
// and all supporting resources (SAs, RBAC, ConfigMaps). The runner and uploader Jobs
// are watched so the reconciler can read their completion status.
func BuildManifest(tmpl *TATemplate, ctx RenderContext) (*hyperfleetv1alpha1.Manifest, error) {
	if err := validateSecretsPolicy(tmpl, ctx); err != nil {
		return nil, err
	}

	labels := buildLabels(ctx)
	resources := make([]hyperfleetv1alpha1.ResourceTemplate, 0, 10)

	runnerSAName := scopeTypeToRunnerSA(ctx.Scope, ctx.Type, ctx.ExecID)

	if isRunnerSADynamic(ctx.Scope) {
		saResource, err := buildServiceAccount(runnerSAName, ctx, labels)
		if err != nil {
			return nil, fmt.Errorf("building service account: %w", err)
		}
		resources = append(resources, saResource)
	}

	rbacResources, err := buildRBACResources(tmpl, ctx, runnerSAName, labels)
	if err != nil {
		return nil, fmt.Errorf("building RBAC resources: %w", err)
	}
	resources = append(resources, rbacResources...)

	outputCMResource, err := buildOutputConfigMap(ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building output configmap: %w", err)
	}
	resources = append(resources, outputCMResource)

	outputRBACResources, err := buildOutputRBAC(ctx, runnerSAName, labels)
	if err != nil {
		return nil, fmt.Errorf("building output RBAC: %w", err)
	}
	resources = append(resources, outputRBACResources...)

	uploaderRBACResources, err := buildUploaderRBAC(ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building uploader RBAC: %w", err)
	}
	resources = append(resources, uploaderRBACResources...)

	scriptCMResource, err := buildScriptConfigMap(tmpl, ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building script configmap: %w", err)
	}
	resources = append(resources, scriptCMResource)

	runnerJobResource, err := buildRunnerJob(tmpl, ctx, runnerSAName, labels)
	if err != nil {
		return nil, fmt.Errorf("building runner job: %w", err)
	}
	resources = append(resources, runnerJobResource)

	uploadJobResource, err := buildUploadJob(tmpl, ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building upload job: %w", err)
	}
	resources = append(resources, uploadJobResource)

	hfm := &hyperfleetv1alpha1.Manifest{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "zoa-" + ctx.ExecID,
			Labels: labels,
		},
		Spec: hyperfleetv1alpha1.ManifestSpec{
			ManagementCluster: ctx.TargetCluster,
			Resources:         resources,
		},
	}

	return hfm, nil
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

func scopeTypeToRunnerSA(scope, taType, execID string) string {
	switch scope {
	case "aws-api":
		if taType == "write" {
			return "zoa-aws-write"
		}
		return "zoa-aws-read"
	default:
		return "zoa-runner-" + execID
	}
}

func isRunnerSADynamic(scope string) bool {
	return scope != "aws-api"
}

func buildServiceAccount(saName string, ctx RenderContext, labels map[string]string) (hyperfleetv1alpha1.ResourceTemplate, error) {
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

	return toResourceTemplate("serviceaccounts", sa, false)
}

func buildRBACResources(tmpl *TATemplate, ctx RenderContext, saName string, labels map[string]string) ([]hyperfleetv1alpha1.ResourceTemplate, error) {
	if tmpl.RBAC == nil || len(tmpl.RBAC.Rules) == 0 {
		return nil, nil
	}

	resources := make([]hyperfleetv1alpha1.ResourceTemplate, 0, 2)
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
		m, err := toResourceTemplate("clusterroles", role, false)
		if err != nil {
			return nil, err
		}
		resources = append(resources, m)

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
		m, err = toResourceTemplate("clusterrolebindings", binding, false)
		if err != nil {
			return nil, err
		}
		resources = append(resources, m)
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
		m, err := toResourceTemplate("roles", role, false)
		if err != nil {
			return nil, err
		}
		resources = append(resources, m)

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
		m, err = toResourceTemplate("rolebindings", binding, false)
		if err != nil {
			return nil, err
		}
		resources = append(resources, m)
	}

	return resources, nil
}

func buildOutputConfigMap(ctx RenderContext, labels map[string]string) (hyperfleetv1alpha1.ResourceTemplate, error) {
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

	return toResourceTemplate("configmaps", cm, false)
}

func buildOutputRBAC(ctx RenderContext, saName string, labels map[string]string) ([]hyperfleetv1alpha1.ResourceTemplate, error) {
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
	roleResource, err := toResourceTemplate("roles", role, false)
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
	bindingResource, err := toResourceTemplate("rolebindings", binding, false)
	if err != nil {
		return nil, err
	}

	return []hyperfleetv1alpha1.ResourceTemplate{roleResource, bindingResource}, nil
}

func buildUploaderRBAC(ctx RenderContext, labels map[string]string) ([]hyperfleetv1alpha1.ResourceTemplate, error) {
	roleName := fmt.Sprintf("zoa-uploader-%s", ctx.ExecID)
	runnerJobName := "zoa-" + ctx.ExecID
	outputCMName := "zoa-output-" + ctx.ExecID

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
				"verbs":         []string{"get"},
				"resourceNames": []string{outputCMName},
			},
			{
				"apiGroups":     []string{"batch"},
				"resources":     []string{"jobs"},
				"verbs":         []string{"get", "list", "watch"},
				"resourceNames": []string{runnerJobName},
			},
		},
	}
	roleResource, err := toResourceTemplate("roles", role, false)
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
				"name":      uploaderSAName,
				"namespace": ctx.Namespace,
			},
		},
	}
	bindingResource, err := toResourceTemplate("rolebindings", binding, false)
	if err != nil {
		return nil, err
	}

	return []hyperfleetv1alpha1.ResourceTemplate{roleResource, bindingResource}, nil
}

func buildScriptConfigMap(tmpl *TATemplate, ctx RenderContext, labels map[string]string) (hyperfleetv1alpha1.ResourceTemplate, error) {
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

	return toResourceTemplate("configmaps", cm, false)
}

func buildRunnerJob(tmpl *TATemplate, ctx RenderContext, saName string, labels map[string]string) (hyperfleetv1alpha1.ResourceTemplate, error) {
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

	runnerExecTimeout := ctx.Config.ExecutionTimeoutSeconds
	if tmpl.TimeoutSeconds > 0 {
		runnerExecTimeout = tmpl.TimeoutSeconds
	}
	runnerUploadTimeout := ctx.Config.UploadTimeoutSeconds
	if runnerUploadTimeout == 0 {
		runnerUploadTimeout = 120
	}
	activeDeadline := int64(runnerExecTimeout + runnerUploadTimeout + 180)

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
			"backoffLimit":            5,
			"activeDeadlineSeconds":   activeDeadline,
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

	return toResourceTemplate("jobs", job, true)
}

func buildUploadJob(tmpl *TATemplate, ctx RenderContext, labels map[string]string) (hyperfleetv1alpha1.ResourceTemplate, error) {
	jobName := "zoa-" + ctx.ExecID + "-upload"

	jobLabels := copyLabels(labels)
	jobLabels[labelRole] = "uploader"

	uploadTimeout := ctx.Config.UploadTimeoutSeconds
	if uploadTimeout == 0 {
		uploadTimeout = 120
	}

	execTimeout := ctx.Config.ExecutionTimeoutSeconds
	if tmpl != nil && tmpl.TimeoutSeconds > 0 {
		execTimeout = tmpl.TimeoutSeconds
	}

	envVars := []map[string]interface{}{
		{"name": "RUN_ID", "value": ctx.ExecID},
		{"name": "JOB_NAMESPACE", "value": ctx.Namespace},
		{"name": "ARTIFACT_BUCKET", "value": ctx.OutputBucket},
		{"name": "OUTPUT_CONFIGMAP", "value": "zoa-output-" + ctx.ExecID},
		{"name": "RUNNER_JOB_NAME", "value": "zoa-" + ctx.ExecID},
		{"name": "UPLOAD_TIMEOUT", "value": fmt.Sprintf("%d", uploadTimeout)},
		{"name": "EXECUTION_TIMEOUT", "value": fmt.Sprintf("%d", execTimeout)},
	}

	activeDeadline := int64(execTimeout + uploadTimeout + 180)

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
			"activeDeadlineSeconds":   activeDeadline,
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

	return toResourceTemplate("jobs", job, true)
}

func resolveTargetNamespace(tmpl *TATemplate, ctx RenderContext) string {
	if tmpl.RBAC != nil && tmpl.RBAC.NamespaceParam != "" {
		if ns, ok := ctx.Params[tmpl.RBAC.NamespaceParam]; ok && ns != "" {
			return ns
		}
	}
	return ctx.Namespace
}

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

func toResourceTemplate(resource string, obj interface{}, watch bool) (hyperfleetv1alpha1.ResourceTemplate, error) {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return hyperfleetv1alpha1.ResourceTemplate{}, fmt.Errorf("marshaling to JSON: %w", err)
	}
	return hyperfleetv1alpha1.ResourceTemplate{
		Resource: resource,
		Content:  runtime.RawExtension{Raw: jsonBytes},
		Watch:    watch,
	}, nil
}
