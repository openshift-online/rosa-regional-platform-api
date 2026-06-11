# ZOA API — Endpoint Reference

**Last Updated Date**: 2026-06-11

**Base Path**: `/api/v0/trusted-actions`

**Authentication**: AWS SigV4 (via API Gateway). Caller identity is extracted from the SigV4 signature and recorded with every operation.

## Endpoints Overview

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| `POST` | `/{action}/run` | `Create` | Execute a Trusted Action |
| `GET` | `/runs/{id}` | `Get` | Retrieve execution details |
| `GET` | `/runs` | `List` | List executions (filtered, paginated) |
| `GET` | `/` | `Catalog` | List all available Trusted Actions |
| `GET` | `/{action}` | `Describe` | Describe a specific Trusted Action |

---

## POST /{action}/run

Execute a Trusted Action on a target cluster.

### Path Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | Yes | TA name (e.g., `get_pods`, `rollout_restart`) |

### Request Body

```json
{
  "target_cluster": "mc-useast1-1",
  "params": {
    "namespace": "maestro",
    "label_selector": "app=maestro",
    "verbose": "false"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target_cluster` | string | Yes | Target management cluster identifier |
| `params` | object | No | Key-value pairs of TA parameters (all values are strings) |

### Parameter Validation

Parameters are validated against the TA template definition:

1. **Required params**: If a TA declares a parameter as `required: true`, it must be present and non-empty
2. **Namespace scoping**: If a TA declares both `namespace` (default: `""`) and `all_namespaces` (default: `"false"`) parameters, the API enforces that either `namespace` is provided OR `all_namespaces=true` is set

### Responses

#### 202 Accepted

Execution created and dispatched to Maestro.

```json
{
  "execution_id": "fa65418c-f4eb-4f5c-8314-baaeb695ba7d",
  "account_id": "123456789012",
  "caller_arn": "arn:aws:sts::123456789012:assumed-role/DevAccess/slopezma",
  "operator": "slopezma",
  "action": "get_pods",
  "target_cluster": "mc-useast1-1",
  "scope": "kube-api",
  "type": "read",
  "status": "pending",
  "output_status": "pending",
  "revision": "a1b2c3d",
  "output_path": "s3://bucket-name/fa65418c-.../output.json",
  "manifest_work_name": "zoa-fa65418c-...",
  "created_at": "2026-06-10T12:00:00Z"
}
```

#### 400 Bad Request

```json
{
  "kind": "Error",
  "code": "invalid-params",
  "reason": "specify namespace or set all_namespaces=true"
}
```

| Error Code | Condition |
|-----------|-----------|
| `invalid-request` | Request body is not valid JSON |
| `missing-target-cluster` | `target_cluster` field is empty |
| `invalid-params` | Required parameter missing or namespace scoping violated |

#### 404 Not Found

```json
{
  "kind": "Error",
  "code": "unknown-action",
  "reason": "Trusted action not found: get_secretz"
}
```

#### 500 Internal Server Error

```json
{
  "kind": "Error",
  "code": "store-error",
  "reason": "Failed to create execution"
}
```

#### 502 Bad Gateway

```json
{
  "kind": "Error",
  "code": "maestro-error",
  "reason": "Failed to dispatch trusted action"
}
```

Indicates Maestro gRPC call failed. The execution record exists in DynamoDB with `status: failed`.

---

## GET /runs/{id}

Retrieve an execution's metadata and optionally its output/logs.

### Path Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string (UUID) | Yes | Execution ID |

### Query Parameters

| Parameter | Values | Default | Description |
|-----------|--------|---------|-------------|
| `fields` | `output`, `logs`, `all`, `none`, or comma-separated combination | `output` | Controls which S3 content is included |

**Field selection behavior:**

| `fields` Value | Metadata | Output | Logs |
|----------------|----------|--------|------|
| (empty/omitted) | Yes | Yes | No |
| `output` | Yes | Yes | No |
| `logs` | Yes | No | Yes |
| `output,logs` | Yes | Yes | Yes |
| `all` | Yes | Yes | Yes |
| `none` | Yes | No | No |

S3 content (output/logs) is only fetched for terminal executions (`succeeded`, `failed`, `timed_out`). For `pending` or `running` executions, only metadata is returned regardless of `fields`.

### Responses

#### 200 OK

```json
{
  "execution_id": "fa65418c-f4eb-4f5c-8314-baaeb695ba7d",
  "account_id": "123456789012",
  "caller_arn": "arn:aws:sts::123456789012:assumed-role/DevAccess/slopezma",
  "operator": "slopezma",
  "action": "get_pods",
  "target_cluster": "mc-useast1-1",
  "scope": "kube-api",
  "type": "read",
  "status": "succeeded",
  "output_status": "uploaded",
  "revision": "a1b2c3d",
  "params": {"namespace": "maestro"},
  "created_at": "2026-06-10T12:00:00Z",
  "completed_at": "2026-06-10T12:00:29Z",
  "runner_seconds": 5,
  "upload_seconds": 12,
  "duration_seconds": 29,

  "output": [
    {"name": "maestro-abc-123", "namespace": "maestro", "status": "Running", "restarts": 0, "age": "3d"}
  ],

  "logs": "[11:00:01] runner starting\n[zoa] execution_id=fa65418c-... action=get_pods target=mc-useast1-1\n...\n--- upload ---\n[11:00:06] upload starting\n[11:00:09] runner waited (3s)\n[11:00:10] configmap read (1s)\n[11:00:10] decoded (0s), uploading to s3\n"
}
```

**Notes:**

- `output` is the parsed JSON from `/artifacts/output.json` (structure depends on the TA)
- `logs` is the raw text content of `execution.log` (includes both runner and upload timeline)
- `params` records the parameters passed at submission time (audit trail)
- `output` and `logs` are only fetched when `output_status` is `"uploaded"`
- If `output_status` is `"pending"` or `"failed"`, these fields are omitted
- If S3 fetch fails for output/logs, the field is omitted (not an error response)

#### 404 Not Found

```json
{
  "kind": "Error",
  "code": "not-found",
  "reason": "Execution not found"
}
```

---

## GET /runs

List executions for the authenticated account, with filtering and pagination.

### Query Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer (1-100) | 20 | Max results per page |
| `status` | string | — | Filter: `pending`, `running`, `succeeded`, `failed`, `timed_out` |
| `action` | string | — | Filter by TA name (exact match) |
| `target` | string | — | Filter by target cluster (exact match) |
| `operator` | string | — | Filter by operator name (exact match) |
| `scope` | string | — | Filter by scope: `kube-api`, `aws-api` |
| `type` | string | — | Filter by type: `read`, `write` |
| `since` | string | — | Time filter (see below) |

**`since` format:**

- Duration shorthand: `30s`, `5m`, `1h`, `24h`, `7d`
- RFC3339 timestamp: `2026-06-10T00:00:00Z`

Duration values are converted to an absolute RFC3339 timestamp at query time.

### Query Execution

Filters are applied at DynamoDB level:

- `since` is applied as a `KeyConditionExpression` on the `createdAt` sort key (efficient — no scan)
- All other filters are applied as `FilterExpression` (post-read filter)
- Results are scoped to the caller's `account_id` (partition key on the GSI)
- Sorted by `createdAt` descending (most recent first)

### Responses

#### 200 OK

```json
{
  "items": [
    {
      "execution_id": "fa65418c-...",
      "action": "get_pods",
      "operator": "slopezma",
      "target_cluster": "mc-useast1-1",
      "scope": "kube-api",
      "type": "read",
      "status": "succeeded",
      "output_status": "uploaded",
      "params": {"namespace": "maestro"},
      "created_at": "2026-06-10T12:00:00Z",
      "completed_at": "2026-06-10T12:00:29Z",
      "runner_seconds": 5,
      "upload_seconds": 12,
      "duration_seconds": 29
    }
  ],
  "total": 1,
  "page": 1,
  "limit": 20,
  "has_more": false
}
```

**Notes:**

- List responses do NOT include `output` or `logs` (metadata only)
- Use `GET /runs/{id}` with `fields` parameter for full content

---

## GET /

List all available Trusted Actions (catalog).

### Responses

#### 200 OK

```json
{
  "items": [
    {
      "name": "get_pods",
      "scope": "kube-api",
      "type": "read",
      "description": "List pods with status, restarts, age, and node placement"
    },
    {
      "name": "get_nodes",
      "scope": "kube-api",
      "type": "read",
      "description": "List all nodes in the target cluster"
    },
    {
      "name": "rollout_restart",
      "scope": "kube-api",
      "type": "write",
      "description": "Perform a rolling restart of a deployment"
    }
  ],
  "total": 15
}
```

**Notes:**

- Parameters are NOT included in catalog responses (use Describe for full details)
- Items are sorted alphabetically by name

---

## GET /{action}

Describe a specific Trusted Action — includes full parameter definitions.

### Path Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | Yes | TA name |

### Responses

#### 200 OK

```json
{
  "name": "get_pods",
  "scope": "kube-api",
  "type": "read",
  "description": "List pods with status, restarts, age, and node placement",
  "params": [
    {
      "name": "namespace",
      "required": false,
      "default": "",
      "description": "Target namespace (required unless all_namespaces=true)"
    },
    {
      "name": "all_namespaces",
      "required": false,
      "default": "false",
      "description": "List pods across all namespaces"
    },
    {
      "name": "label_selector",
      "required": false,
      "default": "",
      "description": "Label selector to filter pods (e.g. app=maestro)"
    },
    {
      "name": "verbose",
      "required": false,
      "default": "false",
      "description": "Return full JSON output instead of compact summary"
    }
  ]
}
```

#### 404 Not Found

```json
{
  "kind": "Error",
  "code": "unknown-action",
  "reason": "Trusted action not found: get_secretz"
}
```

---

## Error Response Format

All errors follow a consistent structure:

```json
{
  "kind": "Error",
  "code": "<error-code>",
  "reason": "<human-readable message>"
}
```

### Error Codes Reference

| HTTP Status | Code | When |
|-------------|------|------|
| 400 | `invalid-request` | Request body is not valid JSON |
| 400 | `missing-target-cluster` | `target_cluster` not provided |
| 400 | `invalid-params` | Parameter validation failed |
| 404 | `unknown-action` | TA name not found in registry |
| 404 | `not-found` | Execution ID not found in DynamoDB |
| 500 | `store-error` | DynamoDB operation failed |
| 500 | `render-error` | ManifestWork generation failed |
| 502 | `maestro-error` | Maestro gRPC call failed |

---

## Execution Lifecycle

### Status Transitions

```
pending → running → succeeded
                  → failed
                  → timed_out
```

### Output Status Transitions

```
pending → uploaded    (uploader Job succeeded)
        → failed     (uploader Job failed or timed out)
```

- `pending`: DynamoDB record created, ManifestWork dispatched to Maestro
- `running`: ManifestWork applied on MC, runner Job pod started
- `succeeded`: Runner Job completed with exit code 0
- `failed`: Runner Job completed with non-zero exit code
- `timed_out`: Execution exceeded timeout, cleaned up by reconciler

**Output status:**
- `pending`: Uploader Job not yet completed
- `uploaded`: Uploader Job succeeded, artifacts available in S3
- `failed`: Uploader Job failed (logs still available via execution metadata)

### Timing Fields

| Field | Set When | Meaning |
|-------|----------|---------|
| `created_at` | On POST (submission) | When the execution was requested |
| `completed_at` | On overall completion | When the reconciler detected both Jobs done |
| `runner_seconds` | On overall completion | Runner Job wall-clock time (from K8s `.status.startTime` to `.status.completionTime`) |
| `upload_seconds` | On overall completion | Time from runner completion to uploader completion (wait + configmap + decode + S3 upload) |
| `duration_seconds` | On overall completion | Total wall-clock: `completed_at - created_at` (includes Maestro dispatch overhead) |

**Derived metric** (not stored): `dispatch_overhead = duration_seconds - runner_seconds - upload_seconds`

---

## DynamoDB Schema

### Table: `<env>-regional-zoa-executions`

| Attribute | Type | Key | Description |
|-----------|------|-----|-------------|
| `executionId` | String | PK | UUID v4 |
| `accountId` | String | — | AWS account ID of caller |
| `callerArn` | String | — | Full ARN of STS caller |
| `operator` | String | — | Extracted operator name |
| `action` | String | — | TA name |
| `targetCluster` | String | — | Target MC identifier |
| `scope` | String | — | `kube-api` or `aws-api` |
| `type` | String | — | `read` or `write` |
| `params` | Map | — | Execution parameters (audit trail) |
| `status` | String | — | Current status |
| `outputStatus` | String | — | `pending`, `uploaded`, or `failed` |
| `revision` | String | — | Git SHA of TA definition |
| `outputPath` | String | — | S3 URI for output.json |
| `manifestWorkName` | String | — | Maestro RB name |
| `createdAt` | String (RFC3339) | — | Submission timestamp |
| `completedAt` | String (RFC3339) | — | Overall completion timestamp |
| `runnerSeconds` | Number | — | Runner Job duration (startTime → completionTime) |
| `uploadSeconds` | Number | — | Upload duration (runner completion → uploader completion) |
| `durationSeconds` | Number | — | Total wall-clock (created → reconciler detected completion) |

### GSI: `account-index`

| Key | Attribute | Purpose |
|-----|-----------|---------|
| PK | `accountId` | Scope queries to caller's account |
| SK | `createdAt` | Enable time-range queries (`since` filter) |

Projection: ALL

### GSI: `status-index`

| Key | Attribute | Purpose |
|-----|-----------|---------|
| PK | `status` | Reconciler queries pending/running executions |
| SK | `createdAt` | Order by time |

Projection: ALL

---

## Usage Examples

### Execute a Trusted Action (synchronous via CLI)

```bash
# CLI wraps: POST + poll GET /runs/{id}?fields=none + final GET /runs/{id}?fields=output
$ zoa run get_pods -t mc-useast1-1 -n maestro
```

### Execute (raw curl)

```bash
curl -X POST "$ZOA_API/api/v0/trusted-actions/get_pods/run" \
  --aws-sigv4 "aws:amz:us-east-1:execute-api" \
  --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" \
  -H "x-amz-security-token: $AWS_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target_cluster": "mc-useast1-1", "params": {"namespace": "maestro"}}'
```

### Poll execution status

```bash
curl "$ZOA_API/api/v0/trusted-actions/runs/fa65418c-...?fields=none" \
  --aws-sigv4 "aws:amz:us-east-1:execute-api" \
  --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" \
  -H "x-amz-security-token: $AWS_SESSION_TOKEN"
```

### Retrieve output

```bash
curl "$ZOA_API/api/v0/trusted-actions/runs/fa65418c-...?fields=output" \
  --aws-sigv4 "aws:amz:us-east-1:execute-api" \
  --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" \
  -H "x-amz-security-token: $AWS_SESSION_TOKEN"
```

### List failed executions in the last 24h

```bash
curl "$ZOA_API/api/v0/trusted-actions/runs?status=failed&since=24h&limit=50" \
  --aws-sigv4 "aws:amz:us-east-1:execute-api" \
  --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" \
  -H "x-amz-security-token: $AWS_SESSION_TOKEN"
```

---

## Related Documentation

- [ZOA Architecture](https://github.com/openshift/rosa-regional-platform/blob/main/docs/design/zoa-architecture.md) — System architecture and network flows
- [ZOA Security Model](https://github.com/openshift/rosa-regional-platform/blob/main/docs/design/zoa-security-model.md) — SA isolation and RBAC
- [ZOA Trusted Actions](https://github.com/openshift/rosa-regional-platform/blob/main/docs/design/zoa-trusted-actions.md) — TA template format and CLI design
