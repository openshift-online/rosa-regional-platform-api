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

	labelPrefix     = "zoa.rosa.io/"
	labelExecID     = labelPrefix + "execution-id"
	labelAction     = labelPrefix + "action"
	labelProfile    = labelPrefix + "profile"
	labelType       = labelPrefix + "type"
	labelScope      = labelPrefix + "scope"
	labelOperator   = labelPrefix + "operator"
	labelRevision   = labelPrefix + "revision"
	labelTarget     = labelPrefix + "target-cluster"
	labelManaged    = labelPrefix + "managed"
	annotCreatedAt  = labelPrefix + "created-at"
)

// BuildManifestWork generates a complete ManifestWork from a TATemplate and RenderContext.
func BuildManifestWork(tmpl *TATemplate, ctx RenderContext) (*workv1.ManifestWork, error) {
	labels := buildLabels(ctx)
	manifests := make([]workv1.Manifest, 0, 5)

	rbacManifests, err := buildRBACManifests(tmpl, ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building RBAC manifests: %w", err)
	}
	manifests = append(manifests, rbacManifests...)

	jobPatchManifests, err := buildJobPatchRBAC(ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building job patch RBAC: %w", err)
	}
	manifests = append(manifests, jobPatchManifests...)

	cmManifest, err := buildScriptConfigMap(tmpl, ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building script configmap: %w", err)
	}
	manifests = append(manifests, cmManifest)

	jobManifest, err := buildJob(tmpl, ctx, labels)
	if err != nil {
		return nil, fmt.Errorf("building job: %w", err)
	}
	manifests = append(manifests, jobManifest)

	jobName := "zoa-" + ctx.ExecID

	mw := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "work.open-cluster-management.io/v1",
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
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
						Name:      jobName,
						Namespace: ctx.Namespace,
					},
					FeedbackRules: []workv1.FeedbackRule{
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{Name: "succeeded", Path: ".status.succeeded"},
								{Name: "failed", Path: ".status.failed"},
								{Name: "uploadOk", Path: ".metadata.annotations.zoa\\.rosa\\.io/upload-ok"},
								{Name: "actionExitCode", Path: ".metadata.annotations.zoa\\.rosa\\.io/action-exit-code"},
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
		labelProfile:  ctx.Profile,
		labelType:     ctx.Type,
		labelScope:    ctx.Scope,
		labelOperator: ctx.Operator,
		labelRevision: ctx.Revision,
		labelTarget:   ctx.TargetCluster,
		labelManaged:  "true",
	}
}

func buildRBACManifests(tmpl *TATemplate, ctx RenderContext, labels map[string]string) ([]workv1.Manifest, error) {
	if tmpl.RBAC == nil || len(tmpl.RBAC.Rules) == 0 {
		return nil, nil
	}

	manifests := make([]workv1.Manifest, 0, 2)
	roleName := fmt.Sprintf("zoa-%s-%s", tmpl.Name, ctx.ExecID)
	saName := profileToSAName(ctx.Profile)

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

func buildScriptConfigMap(tmpl *TATemplate, ctx RenderContext, labels map[string]string) (workv1.Manifest, error) {
	cm := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "zoa-scripts-" + ctx.ExecID,
			"namespace": ctx.Namespace,
			"labels":    labels,
		},
		"data": map[string]interface{}{
			"entrypoint.sh": ctx.Config.EntrypointScript,
			"run.sh":        tmpl.Script,
		},
	}

	return toManifest(cm)
}

func buildJob(tmpl *TATemplate, ctx RenderContext, labels map[string]string) (workv1.Manifest, error) {
	jobName := "zoa-" + ctx.ExecID

	envVars := []map[string]interface{}{
		{"name": "RUN_ID", "value": ctx.ExecID},
		{"name": "JOB_NAME", "value": jobName},
		{"name": "JOB_NAMESPACE", "value": ctx.Namespace},
		{"name": "CLUSTER_ID", "value": ctx.TargetCluster},
		{"name": "ARTIFACT_BUCKET", "value": ctx.OutputBucket},
		{"name": "ACTION_NAME", "value": ctx.ActionName},
		{"name": "OPERATOR", "value": ctx.Operator},
		{"name": "REVISION", "value": ctx.Revision},
		{"name": "PROFILE", "value": ctx.Profile},
		{"name": "TYPE", "value": ctx.Type},
		{"name": "SCOPE", "value": ctx.Scope},
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

	saName := profileToSAName(ctx.Profile)

	job := map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      jobName,
			"namespace": ctx.Namespace,
			"labels":    labels,
		},
		"spec": map[string]interface{}{
			"ttlSecondsAfterFinished": ctx.Config.TTLSeconds,
			"backoffLimit":            0,
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": labels,
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

// buildJobPatchRBAC creates Role+RoleBinding allowing the SA to annotate its own Job.
func buildJobPatchRBAC(ctx RenderContext, labels map[string]string) ([]workv1.Manifest, error) {
	roleName := fmt.Sprintf("zoa-job-patch-%s", ctx.ExecID)
	saName := profileToSAName(ctx.Profile)

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
				"apiGroups":     []string{"batch"},
				"resources":     []string{"jobs"},
				"verbs":         []string{"get", "patch"},
				"resourceNames": []string{"zoa-" + ctx.ExecID},
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

func profileToSAName(profile string) string {
	switch profile {
	case "kube":
		return "zoa-kube-sa"
	case "aws-read":
		return "zoa-aws-read-sa"
	case "aws-write":
		return "zoa-aws-write-sa"
	case "breakglass-read":
		return "zoa-breakglass-read-sa"
	case "breakglass-write":
		return "zoa-breakglass-write-sa"
	default:
		return "zoa-kube-sa"
	}
}

func resolveTargetNamespace(tmpl *TATemplate, ctx RenderContext) string {
	if tmpl.RBAC != nil && tmpl.RBAC.NamespaceParam != "" {
		if ns, ok := ctx.Params[tmpl.RBAC.NamespaceParam]; ok && ns != "" {
			return ns
		}
	}
	return ctx.Namespace
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
