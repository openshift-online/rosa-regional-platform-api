# rosa-regional-platform Helm Chart

Helm chart for rosa-regional-platform API with Envoy sidecar and optional **ArgoCD PostSync hook** that patches TargetGroupBinding.spec.targetGroupARN from a cluster ConfigMap (e.g. `kube-system/bootstrap-output`).

## Prerequisites

- A ConfigMap in the cluster (e.g. `kube-system/bootstrap-output`) with the key `api_target_group_arn` (or as configured in `postSyncHook.bootstrapConfigMap`).
- ArgoCD (or another controller that honors PostSync hooks) when using the hook.

## Configuration

See [values.yaml](values.yaml). Key addition:

```yaml
postSyncHook:
  enabled: true
  bootstrapConfigMap:
    namespace: kube-system
    name: bootstrap-output
    key: api_target_group_arn
  image: bitnami/kubectl:latest
```

## Installation

```bash
helm install rosa-regional-platform ./deployment/helm/rosa-regional-platform \
  --namespace rosa-regional-platform \
  --create-namespace
```

## ArgoCD

Use this chart as the source for an Application; the PostSync hook runs after each sync and creates/updates the TargetGroupBinding from the cluster ConfigMap.
