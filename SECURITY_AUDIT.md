# Security Audit — rosa-regional-platform-api

> **Audit Date:** 2026-05-01  
> **Auditor:** Automated adversarial security agent  
> **Scope:** Full repository static analysis — Go source, Kubernetes manifests, Helm charts, CI/CD

---

## Summary

This repository implements the ROSA Regional Platform API server in Go, using Cedar/AWS Verified Permissions for authorization. The audit identified **0 CRITICAL**, **1 HIGH**, **11 MEDIUM**, and **3 LOW** findings. The most important issues are an unauthenticated Prometheus metrics endpoint, untrusted AWS identity headers accepted without source validation, and a fail-open privileged access check that proceeds on authorization errors.

---

## Findings

### HIGH

---

**[HIGH] Prometheus Metrics Endpoint Exposed Without Authentication**

- **File:** `pkg/server/server.go` (lines 227–228, 245–249)
- **Category:** Application — Missing Access Controls
- **Issue:** The `/metrics` Prometheus endpoint is served on a separate port (9090) with no authentication middleware. The Envoy deployment manifest routes this port through the load balancer, making it potentially reachable to any client that can hit the ALB.

- **Attack Vector:** An attacker can query `/metrics` to enumerate: active operation counts, error rates by endpoint, database query latency (inferring query patterns), authorization check timings (usable for side-channel analysis of Cedar policy evaluation), and internal label cardinality. This data significantly accelerates targeted attacks by revealing system internals.

- **Impact:** Information disclosure of operational internals, potential side-channel amplification for authorization bypass research, compliance violation if metrics labels contain account IDs or resource identifiers.

- **Recommendation:** Remove `/metrics` from the external Envoy routing. Serve metrics only on a loopback or cluster-internal interface. If external scraping is required, add authentication middleware (Bearer token or mTLS) and restrict by NetworkPolicy.

---

### MEDIUM

---

**[MEDIUM] AWS Identity Headers Trusted Without Source Validation**

- **File:** `pkg/middleware/identity.go` (lines 33–59)
- **Category:** Application — Authentication
- **Issue:** The `Identity` middleware extracts `X-Amz-Account-Id`, `X-Amz-Caller-Arn`, and related headers directly from the HTTP request without validating that the request came from a trusted upstream (API Gateway or ALB with IAM auth). Any client that can reach the API server directly can spoof these headers and impersonate any AWS principal.

- **Attack Vector:** An attacker who can reach the API server directly (bypassing API Gateway — e.g., via network misconfiguration, a VPN, or an internal SSRF) sends requests with arbitrary `X-Amz-Caller-Arn: arn:aws:iam::123456789012:role/AdminRole`. The identity middleware trusts this header. Authorization checks then evaluate against the spoofed identity.

- **Impact:** Full authentication bypass and identity spoofing. An attacker can claim to be any account or role, including privileged ones. Severity is HIGH if the API is reachable without going through API Gateway; MEDIUM if API Gateway is strictly enforced at the network layer.

- **Recommendation:** Document and enforce at the infrastructure layer that this API must only be reachable via API Gateway with IAM authorization enabled. Consider adding a secondary validation: require a request signature header that API Gateway adds and the application verifies, or use mTLS between ALB and the application to ensure only trusted proxies can set identity headers.

---

**[MEDIUM] CORS Configured to Allow All Origins (`*`)**

- **File:** `pkg/server/server.go` (lines 215–219)
- **Category:** Application — API Security
- **Issue:**
  ```go
  handlers.AllowedOrigins([]string{"*"})
  ```
  This disables CORS same-origin protection, allowing any web page to make cross-origin requests to this API on behalf of a victim's browser session.

- **Attack Vector:** An attacker hosts a malicious webpage. When a victim (authenticated user) visits it, their browser makes cross-origin requests to the platform API using the victim's credentials (cookies, tokens). The API responds, and the attacker's page reads the response due to the wildcard CORS policy.

- **Impact:** Cross-origin data exfiltration, unauthorized actions performed using victim credentials.

- **Recommendation:** Replace `"*"` with the explicit origins that legitimately consume this API (e.g., the console domain). Use environment variables to configure this per deployment.

---

**[MEDIUM] Privileged Access Check Fails Open on Authorization Error**

- **File:** `pkg/middleware/privileged.go` (lines 31–51)
- **Category:** Application — Error Handling / Authorization
- **Issue:** When the `CheckPrivileged` middleware fails to contact DynamoDB or the authorization service (transient error, throttling, network partition), it logs the error and continues to the next handler — granting access to the privileged endpoint:

  ```go
  isPrivileged, err := p.authorizer.IsPrivileged(ctx, accountID)
  if err != nil {
      p.logger.Error("failed to check privileged status", ...)
      // falls through to next handler
  }
  ```

- **Attack Vector:** An attacker triggers or times requests to coincide with a transient DynamoDB outage or rate-limit event (e.g., caused by a concurrent load spike they induce). During the outage window, privileged endpoints become accessible to non-privileged accounts.

- **Impact:** Privilege escalation during service degradation windows. An attacker who can induce transient failures in the authorization backend effectively has a window to perform privileged operations.

- **Recommendation:** Fail closed: return `503 Service Unavailable` (not a permission-granting response) when the authorization check itself fails. Only grant access on an explicit positive result.

---

**[MEDIUM] HTTP Clients to Hyperfleet and Maestro Lack Explicit TLS Configuration**

- **File:** `pkg/clients/hyperfleet/client.go` (lines 31–39), `pkg/clients/maestro/client.go` (lines 141–143)
- **Category:** Application — TLS/Transport Security
- **Issue:** HTTP clients connecting to upstream services use default Go transport without explicitly configuring TLS verification. While Go defaults to certificate verification, there is no explicit minimum TLS version, no cipher suite restriction, and no documentation confirming this is intentional. Future code changes adding `InsecureSkipVerify: true` would not be caught by any automated check.

- **Recommendation:** Explicitly configure TLS: set `MinVersion: tls.VersionTLS12`, `InsecureSkipVerify: false`, and add a comment documenting the intent. This acts as a forcing function for code reviewers to notice any future deviation.

---

**[MEDIUM] Container Deployments Missing Kubernetes Security Contexts**

- **File:** `deployment/manifests/api.yaml`, `deployment/helm/rosa-regional-frontend/templates/deployment.yaml`
- **Category:** Infrastructure — Kubernetes Pod Security
- **Issue:** Neither the main API container nor its Envoy sidecar specify a `securityContext`. This means:
  - `allowPrivilegeEscalation` is not explicitly denied
  - `readOnlyRootFilesystem` is not enforced
  - Capability dropping is not applied
  - Running as non-root is not enforced at the Kubernetes layer (even if the Dockerfile has a non-root user, a misconfigured image override could change this)

- **Recommendation:** Add to all container specs:
  ```yaml
  securityContext:
    runAsNonRoot: true
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop: ["ALL"]
  ```

---

**[MEDIUM] Production Deployment Uses `:latest` Image Tag with `imagePullPolicy: Always`**

- **File:** `deployment/manifests/api.yaml` (line 150–151), `deployment/helm/rosa-regional-frontend/values.yaml` (lines 16–18)
- **Category:** Supply Chain — Unpinned Image Tag
- **Issue:** `quay.io/cdoan0/rosa-regional-platform-api:latest` with `imagePullPolicy: Always` means every pod restart pulls a potentially different, unaudited image. Notably, the image is hosted under a personal account (`cdoan0`), not an organizational one.

- **Attack Vector:** Compromise of the `cdoan0` Quay.io account results in all running pods being updated with a malicious image on next restart (node maintenance, OOM kill, rollout).

- **Recommendation:** Use an organizational registry, pin to a commit-SHA-tagged image, and set `imagePullPolicy: IfNotPresent`. Implement image signing and verification.

---

**[MEDIUM] Authorization Errors May Expose Internal System Details**

- **File:** `pkg/middleware/authz.go` (lines 66–80), `pkg/handlers/authz.go` (lines 157–162, 247–249)
- **Category:** Application — Error Handling / Information Disclosure
- **Issue:** Authorization failure responses include a `Reason` field whose content may vary in ways that reveal internal structure — e.g., distinguishing "account not provisioned" from "policy evaluation denied". This provides oracle information to an attacker enumerating what accounts are provisioned.

- **Recommendation:** Return a single generic `access-denied` message to clients regardless of the specific authorization failure. Log the detailed reason server-side with a correlation ID for debugging.

---

**[MEDIUM] `dummy` Credentials Hardcoded for Local DynamoDB**

- **File:** `pkg/authz/client/dynamodb.go` (lines 23–26)
- **Category:** Application — Credential Management
- **Issue:** The code hardcodes `"dummy"` credentials when a custom DynamoDB endpoint is configured:
  ```go
  aws.String("dummy"), // access key ID
  aws.String("dummy"), // secret access key
  ```
  If `DYNAMODB_ENDPOINT` is ever mistakenly set in a production environment, authentication to DynamoDB is bypassed using these dummy credentials.

- **Recommendation:** Add an explicit production guard: reject configuration where `DYNAMODB_ENDPOINT` is set to a non-local value, or require an environment variable `ALLOW_LOCAL_DYNAMODB=true` to be explicitly set.

---

**[MEDIUM] Envoy Sidecar Uses `v1.31-latest` Floating Image Tag**

- **File:** `deployment/manifests/api.yaml` (line 201), `deployment/helm/rosa-regional-frontend/values.yaml` (line 44)
- **Category:** Supply Chain — Unpinned Image Tag
- **Issue:** `envoyproxy/envoy:v1.31-latest` is a floating tag — any patch push to the `v1.31-latest` tag by the upstream project changes what image runs after the next pod restart.

- **Recommendation:** Pin to a specific patch version (e.g., `envoyproxy/envoy:v1.31.5`) or a SHA256 digest.

---

**[MEDIUM] No Rate Limiting on Any API Endpoint**

- **File:** `pkg/server/server.go`
- **Category:** Application — DoS Protection
- **Issue:** The API has no rate limiting middleware. An attacker can send unlimited requests, exhausting DynamoDB read/write capacity, triggering AVP API throttling, and causing cascading failures in Maestro and Hyperfleet.

- **Recommendation:** Add per-IP and per-account rate limiting middleware. Return `429 Too Many Requests` with a `Retry-After` header when limits are exceeded.

---

**[MEDIUM] Missing HTTP Security Response Headers**

- **File:** `pkg/server/server.go`
- **Category:** Application — HTTP Security
- **Issue:** API responses do not include: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Strict-Transport-Security`, or `Content-Security-Policy`. While this is a JSON API and browser-based attacks are less likely, these headers are a standard defense-in-depth control.

- **Recommendation:** Add a global middleware that sets security headers on all responses.

---

**[MEDIUM] AWS Credentials Passed as Command-Line Environment Variables in E2E Tests**

- **File:** `Makefile` (lines 122–128)
- **Category:** CI/CD — Credential Handling
- **Issue:** E2E test targets accept AWS credentials (`CUSTOMER_AWS_ACCESS_KEY_ID`, `CUSTOMER_AWS_SECRET_ACCESS_KEY`) as environment variables passed on the command line, which can appear in process listings (`ps aux`) and CI log output.

- **Recommendation:** Use IAM roles for CI runners. If static credentials are unavoidable for test accounts, pass them via file or secrets manager rather than environment variables sourced at the command line.

---

### LOW

---

**[LOW] No Validation of AWS Account ID Format in Identity Headers**

- **File:** `pkg/middleware/identity.go`
- **Category:** Application — Input Validation
- **Issue:** The `X-Amz-Account-Id` header value is stored in context without validating it matches the expected 12-digit format. Invalid values propagate into authorization checks.

- **Recommendation:** Validate with `regexp.MustCompile("^[0-9]{12}$")` and reject malformed values.

---

**[LOW] No HTTP Request Body Size Limit**

- **File:** `pkg/server/server.go`
- **Category:** Application — DoS Protection
- **Issue:** No `http.MaxBytesReader` is applied, allowing arbitrarily large request bodies to be read into memory.

- **Recommendation:** Apply `http.MaxBytesReader(w, r.Body, 10*1024*1024)` in a global middleware.

---

**[LOW] E2E Test Images Use `:latest` Tags**

- **File:** `hack/podman-compose.e2e-authz.yaml`
- **Category:** Supply Chain — Unpinned Image Tag
- **Issue:** DynamoDB Local and Cedar Agent images in E2E test infrastructure use `:latest` tags, leading to non-deterministic test environments.

- **Recommendation:** Pin test images to specific versions to ensure reproducible test results and prevent supply chain surprises in the test environment.

