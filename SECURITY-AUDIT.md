# Security Audit — rosa-regional-platform-api

**Audit Date:** 2026-06-15
**Auditor:** security-audit-agent (automated)
**Scope:** Full static analysis of Go source files, Kubernetes manifests, Envoy configuration, CI scripts, Dockerfiles
**Previous PRs:** #80 (closed), #85 (merged), #97 (superseded by this report)

> This PR supersedes PR #97. It incorporates all findings from #97 and #85, and adds new findings. No user comments in previous PRs dismissed any finding as a non-issue.
>
> **Note:** PR #85 was merged for tracking/visibility purposes, not to indicate its findings were resolved.

---

## CRITICAL Findings

### CRIT-1 — Authentication via Forgeable HTTP Headers **(carry-over from #85/#97, unresolved)**

**File:** `pkg/middleware/identity.go` lines 32–59

**Risk:** The entire authentication model trusts `X-Amz-Account-Id`, `X-Amz-Caller-Arn`, and related HTTP headers with no cryptographic verification that these headers were set by the Envoy sidecar or API Gateway. Any caller that bypasses Envoy and reaches the API pod directly can forge any identity, including privileged accounts.

**Attack vectors:**
1. **Direct pod IP access:** From within the EKS cluster, any pod can call `http://<pod-ip>:8000` and set `X-Amz-Account-Id: <target>` and `X-Amz-Caller-Arn: arn:aws:iam::<target>:role/OrganizationAccountAccessRole` to impersonate any account.
2. **Service port exposure:** The `deployment/manifests/api.yaml` Service exposes port 8000 as a named port, making the raw API directly accessible from any pod in the cluster without going through Envoy (which sets identity headers).
3. **Missing NetworkPolicy:** If Kubernetes NetworkPolicy is absent or uses default-allow, any cluster pod can forge headers.

**What to mitigate:**
- Validate requests using an HMAC or shared secret that only the Envoy/API Gateway can inject, which the Go service verifies before trusting `X-Amz-*` headers.
- Remove port 8000 from the Kubernetes Service spec — Envoy on port 8080 should be the only ingress path.
- Enforce a NetworkPolicy denying all ingress to pod port 8000 except from localhost (the Envoy sidecar).

---

### CRIT-2 — Kubernetes Service Exposes Raw API Port 8000, Bypassing Envoy Sidecar **(NEW)**

**File:** `deployment/manifests/api.yaml` lines ~177–190

```yaml
spec:
  ports:
    - name: http
      port: 8080    # Envoy — correct
      targetPort: 8080
    - name: api
      port: 8000    # Raw app port — bypasses Envoy
      targetPort: api
    - name: health
      port: 8081
      targetPort: health
    - name: metrics
      port: 9090
      targetPort: metrics
```

**Risk:** The Kubernetes Service exposes port 8000 cluster-wide. This means any pod in the cluster can call `http://rosa-regional-platform:8000` and reach the application *without going through the Envoy sidecar*. The Envoy sidecar is the component responsible for propagating IAM identity headers from the API Gateway. Bypassing Envoy means the request arrives with no `X-Amz-*` headers, and depending on how the identity middleware handles missing headers, the request may proceed with an empty or anonymous identity.

**Attack vector:** A compromised pod calls `http://rosa-regional-platform:8000/api/v0/clusters` with no identity headers. If the identity middleware treats a missing `X-Amz-Account-Id` as an empty string rather than rejecting the request, the subsequent authorization chain operates on an empty account ID — behavior that may allow or deny unpredictably based on DynamoDB contents.

**What to mitigate:** Remove the `api` (8000), `health` (8081), and `metrics` (9090) ports from the Kubernetes Service. Only expose port 8080 (Envoy). Add a NetworkPolicy that restricts ingress to port 8080 to the ALB target group source and monitoring namespace only.

---

## HIGH Findings

### HIGH-1 — Authorization Silently Disabled When Config Is Missing **(carry-over from #85/#97, unresolved)**

**File:** `pkg/server/server.go` lines ~87–102

**Risk:** When `cfg.Authz` is `nil` or `cfg.Authz.Enabled` is `false`, the server falls back to `RequireAllowedAccount` middleware only — which checks only account allowlist membership with no per-resource authorization and no admin requirements. Any misconfiguration (missing environment variable, failed DynamoDB connection returning nil config) silently degrades security to allowlist-only mode where any allowlisted account has full access to all operations.

**What to mitigate:** Fail closed: if authz is expected but config is nil or invalid, log a fatal error and exit. Add a runtime health endpoint that exposes the current authorization mode so monitoring can alert on degraded state.

---

### HIGH-2 — No Request Body Size Limits Enable Memory Exhaustion DoS **(carry-over from #97, unresolved)**

**Files:** `pkg/handlers/cluster.go:85`, `pkg/handlers/nodepool.go:81`, and 40+ handler locations

**Risk:** All HTTP request bodies are decoded with `json.NewDecoder(r.Body).Decode(&req)` without wrapping in `http.MaxBytesReader`. A client sending a large `Content-Length` forces the Go JSON decoder to allocate an arbitrarily large buffer, causing pod OOM.

**Attack vector:** `POST /api/v0/clusters` with `Content-Length: 1073741824` causes pod OOM restart. Any authorized account can trigger this — no authentication bypass required. Repeated requests prevent recovery.

**What to mitigate:** Add middleware that wraps every request body: `r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)`.

---

### HIGH-3 — Hardcoded Privileged Test Account in e2e Script **(carry-over from #97, unresolved)**

**File:** `scripts/e2e-init-dynamodb.sh` lines ~90–105

```bash
aws dynamodb put-item --endpoint-url "$ENDPOINT" --table-name "rosa-authz-accounts" \
  --item '{"accountId": {"S": "000000000000"}, "privileged": {"BOOL": true}, ...}'
```

**Risk:** If `$ENDPOINT` is empty or misconfigured, the script writes a privileged entry for account `000000000000` to production DynamoDB. This account ID is publicly visible in source code — any internal actor who knows it could exploit it against a misconfigured environment.

**What to mitigate:** Abort if `$ENDPOINT` does not contain `localhost`/`127.0.0.1`. Use a randomly generated account ID for tests.

---

### HIGH-4 — Authorization Middleware Fails Open on DynamoDB Errors **(carry-over from #97, unresolved)**

**Files:** `pkg/middleware/account_check.go` lines ~44–49, `pkg/middleware/privileged.go` lines ~40–50

**Risk:** When DynamoDB is unavailable, both middleware functions log an error and call `next.ServeHTTP`, allowing requests to proceed with unverified authorization state. This is fail-open behavior.

**What to mitigate:** Return `503 Service Unavailable` when the authorization backend is unreachable. Do not allow requests to proceed with unverified privilege or account status.

---

### HIGH-5 — Unpinned Container Images in Development Deployment Manifest **(NEW)**

**File:** `deployment/manifests/api.yaml` lines ~116, ~149

```yaml
image: quay.io/cdoan0/rosa-regional-platform-api:latest   # personal account, mutable tag
image: envoyproxy/envoy:v1.31-latest                       # moving branch tag
```

**Risk:**
1. `quay.io/cdoan0/rosa-regional-platform-api:latest` — a personal quay.io account with a mutable `:latest` tag. If this manifest is applied to any environment, a compromised push to this personal repository updates the deployed API. With `imagePullPolicy: Always`, every pod restart pulls the latest image.
2. `envoyproxy/envoy:v1.31-latest` — the Envoy sidecar that handles identity extraction is pinned to a moving branch tag. A security-relevant regression in a new v1.31.x patch would be silently deployed.

**What to mitigate:** Move the API image to `quay.io/rrp-dev-ci/` or the `openshift-online` organization. Pin both images to SHA256 digests. Update `imagePullPolicy` to `IfNotPresent` for digest-pinned images.

---

### HIGH-6 — Prometheus Metrics Endpoint Exposed Cluster-Wide Without Authentication **(carry-over from #85, partially mitigated)**

**File:** `deployment/manifests/api.yaml` (port 9090 on Service), `pkg/server/server.go`

**Risk:** The metrics endpoint is bound to `0.0.0.0:9090` and exposed via the Kubernetes Service on port 9090 with no authentication. Any pod in the cluster can scrape it and gain operational intelligence: request rates, error patterns, endpoint call volumes, and any custom metrics that may include account or cluster identifiers.

**What to mitigate:** Restrict the metrics bind address to `127.0.0.1` or add Prometheus bearer token authentication. Remove port 9090 from the Kubernetes Service. Use a NetworkPolicy to restrict metrics scraping to the Prometheus namespace only.

---

## MEDIUM Findings

### MED-1 — Wildcard CORS Policy **(carry-over from #85, unresolved)**

**File:** `pkg/server/server.go` lines ~174–179

```go
handlers.AllowedOrigins([]string{"*"})
```

**Risk:** All origins are allowed. The `Authorization` header is in `AllowedHeaders`. If any browser-initiated auth is added in the future, this immediately enables CSRF from any origin.

**What to mitigate:** Restrict `AllowedOrigins` to specific frontend domains, or remove CORS headers entirely if the API is purely machine-to-machine.

---

### MED-2 — Identity Headers Not Validated for Format **(carry-over from #85, unresolved)**

**File:** `pkg/middleware/identity.go`

**Risk:** `X-Amz-Account-Id` accepts arbitrary strings without format validation. AWS account IDs are always 12-digit numbers. Accepting malformed values enables log injection, downstream key injection in DynamoDB queries, and error message injection.

**What to mitigate:** Validate `X-Amz-Account-Id` matches `/^\d{12}$/` and `X-Amz-Caller-Arn` matches the expected ARN format. Return `400 Bad Request` for malformed values.

---

### MED-3 — Build Stage Dockerfile Uses Unpinned `golang:1.25-alpine` **(NEW)**

**File:** `Dockerfile` line 2

```dockerfile
FROM golang:1.25-alpine AS builder
```

**Risk:** `golang:1.25-alpine` is a mutable tag. A compromise of this base image affects the compiled binary output. The build stage compiles the API server — any backdoor introduced here is deployed to production.

**What to mitigate:** Pin to a SHA256 digest: `FROM golang:1.25-alpine@sha256:<hash>`.

---

### MED-4 — Runtime Dockerfile Uses Unpinned `distroless/static-debian12:nonroot` **(NEW)**

**File:** `Dockerfile` lines ~20–22

```dockerfile
FROM gcr.io/distroless/static-debian12:nonroot
```

**Risk:** The `nonroot` tag is mutable. While distroless images are hardened, a regressed or compromised `nonroot` tag could introduce vulnerabilities into the API runtime environment.

**What to mitigate:** Pin to a SHA256 digest: `FROM gcr.io/distroless/static-debian12@sha256:<hash>`.
