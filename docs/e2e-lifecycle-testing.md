# E2E Lifecycle Testing

Design document for end-to-end testing across the full HCP lifecycle on a management cluster.

## TL;DR

If you have come to this document from a CI failure telling you that a test is missing an `hcp:` lifecycle label, you most likely just need to add `Label("hcp:available")` to the `Describe` or `Context` block that contains the test. See [Adding New Tests](#adding-new-tests) for an example.

If you're reviewing this document for the first time, then the general idea is that we have a set of phases that define the lifecycle of a test, and each phase has a corresponding `hcp:` lifecycle label. For example, `hcp:pre-setup` is the phase that runs before the test setup, and `hcp:setup` is the phase that runs during the test setup.

## Problem

As the platform grows, feature developers need a predictable way to hook tests into the HCP lifecycle without understanding the entire test suite. Today, tests are ordered implicitly by file position within a single `Ordered` Ginkgo suite. Labels exist (`setup`, `create`, `monitor`, `cleanup`) but aren't formalized — there's no contract about what state is available at each phase, no way to slot new tests between phases, and no enforcement that reserved labels aren't misused.

## Design

### Lifecycle Phases

The HCP lifecycle is divided into phases. The main phases are `setup`, `create`, `monitor`, `available`, and `cleanup`. In general, with the exception of `hcp:available`, you probably do not want to add tests to these phases as there is no guarantee that they will run before or after the actual commands to manipulate the HCP itself. While there is no current plan on restricting which tests can run in which phases, it is best to avoid adding tests to these phases unless absolutely necessary.

Further, each main phase has three sub-phases: `pre-`, the phase itself, and `post-`, and they run in that order. This allows for granular control over when tests are run relative to the actual HCP commands, and allows more tests to be run in parallel. There is no guarantee that feature A will not have a conflict with Feature B if both are manipulating the same resources during the same phase, or that tests between features will run in any specific or consistent order. We do guarantee that all tests for a given phase will run before moving to the next (sub)phase.

```
hcp:pre-setup     →  hcp:setup     →  hcp:post-setup
hcp:pre-create    →  hcp:create    →  hcp:post-create
hcp:pre-monitor   →  hcp:monitor   →  hcp:post-monitor
hcp:pre-available →  hcp:available →  hcp:post-available
hcp:pre-cleanup   →  hcp:cleanup   →  hcp:post-cleanup
```

The `hcp:available` phase is distinct from `hcp:monitor` — it represents the window where the HCP is fully operational (API server reachable, nodepools ready). `hcp:monitor` covers status polling and readiness checks that happen before availability is confirmed.

### Phase Definitions

#### `hcp:pre-setup`

**State**: Nothing provisioned. AWS credentials available.
**Purpose**: Validate preconditions before infrastructure provisioning begins.
**Example uses**: AWS credential checks, region availability checks, quota validation.

#### `hcp:setup`

**State**: Infrastructure provisioning in progress.
**Purpose**: Create the cloud infrastructure required for an HCP.
**Current tests**:
| Test | Label | Description |
|------|-------|-------------|
| Login to BASE_URL | `login` | Authenticate rosactl against the platform API |
| Create cluster-vpc | `vpc-create` | Provision VPC via rosactl |
| List cluster-vpc | `vpc-list` | Verify VPC appears in listing |
| Create cluster-iam | `iam-create` | Provision IAM roles via rosactl |
| List cluster-iam | `iam-list` | Verify IAM roles appear in listing |
| Add customer account | `account-add` | Register customer account with the platform API |

#### `hcp:post-setup`

**State**: VPC, IAM, and account registration complete.
**Purpose**: Validate infrastructure is correctly provisioned before cluster creation.
**Example uses**: Verify subnet tags, validate IAM trust policies, check account permissions.

#### `hcp:pre-create`

**State**: Infrastructure ready, no HCP exists.
**Purpose**: Pre-creation validation.
**Example uses**: Verify no stale clusters exist, validate cluster name availability.

#### `hcp:create`

**State**: HCP creation in progress.
**Purpose**: Create the HCP and supporting resources (OIDC).
**Current tests**:
| Test | Label | Description |
|------|-------|-------------|
| Create HCP cluster | `hcp-create` | Create cluster via rosactl, capture cluster ID |
| Create cluster-oidc | `oidc-create` | Provision OIDC provider for the cluster |
| List cluster-oidc | `oidc-list` | Verify OIDC provider appears in listing |

#### `hcp:post-create`

**State**: HCP create request accepted, cluster provisioning.
**Purpose**: Validate creation was accepted correctly.
**Example uses**: Verify cluster appears in API listings, check initial status, validate resource bundles created.

#### `hcp:pre-monitor`

**State**: Cluster provisioning, not yet ready.
**Purpose**: Setup for monitoring/polling phase.
**Example uses**: Record start time for installation duration metrics.

#### `hcp:monitor`

**State**: Cluster transitioning to ready.
**Purpose**: Poll for readiness.
**Current tests**:
| Test | Label | Description |
|------|-------|-------------|
| Wait for cluster ready | `cluster-status` | Poll `/clusters/{id}/statuses` until all controller conditions are True (20min timeout) |
| Wait for nodepools | `nodepools-wait` | Wait 5min for nodepools to deploy |

#### `hcp:post-monitor`

**State**: Controller conditions True, nodepools deploying.
**Purpose**: Validate monitoring results.
**Example uses**: Assert installation completed within SLA, log time-to-ready.

#### `hcp:pre-available`

**State**: Cluster reported ready, final convergence in progress.
**Purpose**: Last checks before declaring availability.
**Example uses**: Verify API server DNS resolves, check certificate issuance.

#### `hcp:available`

**State**: HCP is fully operational. API server reachable, nodepools ready, metrics flowing.
**Purpose**: Run functional tests against a live HCP. This is the primary phase for feature verification.
**Current tests**:
| Test | Label | Description |
|------|-------|-------------|
| HCP availability metric | `hcp-metrics` | Query Thanos for `hcp:hostedcluster_available` recording rule metric |

**Example uses**: Run workloads on the cluster, verify ingress, test HCP API server responsiveness, validate SLA recording rules are producing data, run customer-facing feature smoke tests.

#### `hcp:post-available`

**State**: Functional testing complete, cluster still running.
**Purpose**: Collect results, take snapshots.
**Example uses**: Capture must-gather, record final metric values, export audit logs.

#### `hcp:pre-cleanup`

**State**: Cluster running, about to be torn down.
**Purpose**: Pre-deletion validation or data collection.
**Example uses**: Verify no orphaned resources before deletion, snapshot final cluster state.

#### `hcp:cleanup`

**State**: Teardown in progress.
**Purpose**: Delete the HCP and all associated infrastructure.
**Current tests**:
| Test | Label | Description |
|------|-------|-------------|
| Delete HCP cluster | `hcp-delete` | DELETE `/clusters/{id}`, expect 202 |
| Poll until deleted | `cluster-query` | Poll GET `/clusters/{id}` until 404/410 (10min timeout) |
| Delete resource bundles | `bundles-delete` | Delete all resource bundles matching cluster ID |
| Delete cluster-oidc | `oidc-delete` | Remove OIDC provider via rosactl |
| Delete cluster-vpc | `vpc-delete` | Remove VPC via rosactl (3 retries, 5min backoff) |
| Delete cluster-iam | `iam-delete` | Remove IAM roles via rosactl |

#### `hcp:post-cleanup`

**State**: All resources deleted.
**Purpose**: Verify clean teardown.
**Example uses**: Verify no orphaned VPCs/IAM roles, check CloudFormation stacks deleted, validate no dangling DNS records.

### Execution Order

When run as a full suite (`ginkgo ./test/e2e-cli`), the `Ordered` container guarantees specs execute top-to-bottom. The lifecycle label filter runs phases in this order:

```
ginkgo --label-filter="hcp:pre-setup"     ./test/e2e-cli
ginkgo --label-filter="hcp:setup"         ./test/e2e-cli
ginkgo --label-filter="hcp:post-setup"    ./test/e2e-cli
ginkgo --label-filter="hcp:pre-create"    ./test/e2e-cli
ginkgo --label-filter="hcp:create"        ./test/e2e-cli
ginkgo --label-filter="hcp:post-create"   ./test/e2e-cli
ginkgo --label-filter="hcp:pre-monitor"   ./test/e2e-cli
ginkgo --label-filter="hcp:monitor"       ./test/e2e-cli
ginkgo --label-filter="hcp:post-monitor"  ./test/e2e-cli
ginkgo --label-filter="hcp:pre-available" ./test/e2e-cli
ginkgo --label-filter="hcp:available"     ./test/e2e-cli
ginkgo --label-filter="hcp:post-available"./test/e2e-cli
ginkgo --label-filter="hcp:pre-cleanup"   ./test/e2e-cli
ginkgo --label-filter="hcp:cleanup"       ./test/e2e-cli
ginkgo --label-filter="hcp:post-cleanup"  ./test/e2e-cli
```

Individual phases can be run in isolation when the required state already exists (e.g., a long-lived dev cluster).

### Adding New Tests

To add a test that runs during a specific lifecycle phase, label the `Describe` or `Context` block with the appropriate lifecycle label. All child specs inherit the label automatically — there's no need to repeat it on individual `It` blocks:

```go
// In a new file: test/e2e-cli/my_feature_test.go
var _ = Describe("My Feature", func() {
    Context("validates feature X after HCP is available", Label("hcp:available"), func() {
        It("should verify feature X works", func() {
            // test code — HCP is fully operational at this point
        })

        It("should verify feature Y works", func() {
            // also inherits hcp:available from the Context
        })
    })

    Context("cleans up feature X resources", Label("hcp:pre-cleanup"), func() {
        It("should remove feature X test data", func() {
            // cleanup before HCP deletion begins
        })
    })
})
```

If a file only contains tests for a single phase, label the top-level `Describe`. This is most likely the case for almost all functional tests for various features that require the HCP to be ready for use.

```go
var _ = Describe("HCP Metrics", Label("hcp:available"), func() {
    It("should have availability metric", func() { ... })
    It("should have install metric", func() { ... })
})
```

Every spec must have at least one `hcp:` lifecycle label, whether inherited from a parent container or applied directly. Unlabeled specs will be caught by the enforcement check described below.

### Reserved Labels

The following bare labels are **reserved** and must not be used on new tests. They exist only for backward compatibility with the original `cluster_test.go` specs:

- `setup`, `create`, `monitor`, `cleanup`

New tests must use the `hcp:` prefixed labels. A future migration will update existing tests to use `hcp:` labels and retire the bare names.

### Fine-Grained Labels

Tests may carry both a lifecycle label and a feature-specific label:

```go
Context("HCP metrics", Label("hcp:available", "hcp-metrics"), func() {
    It("should have hcp:hostedcluster_available metric", func() { ... })
})
```

This allows running all tests for a specific feature across lifecycle phases:

```bash
ginkgo --label-filter="hcp-metrics" ./test/e2e-cli
```

### Enforcement

Every spec in `test/e2e-cli` must carry an `hcp:` lifecycle label (directly or inherited from a parent `Describe`/`Context`). This is enforced programmatically using Ginkgo's `PreviewSpecs()` API, which resolves the full label set for each spec — including labels inherited from parent containers.

Add the following to `test/e2e-cli/suite_test.go`:

```go
func TestE2ECLI(t *testing.T) {
    RegisterFailHandler(Fail)

    report := PreviewSpecs("ROSA Regional Platform API CLI E2E Suite")
    for _, spec := range report.SpecReports {
        if spec.State == types.SpecStatePending {
            continue
        }
        labels := spec.Labels()
        hasLifecycleLabel := false
        for _, l := range labels {
            if strings.HasPrefix(l, "hcp:") {
                hasLifecycleLabel = true
                break
            }
        }
        if !hasLifecycleLabel {
            t.Errorf("spec %q has no hcp: lifecycle label — see docs/e2e-lifecycle-testing.md", spec.FullText())
        }
    }
    if t.Failed() {
        t.Fatal("all e2e-cli specs must have an 'hcp:' lifecycle label. See docs/e2e-lifecycle-testing.md for more information.")
    }

    RunSpecs(t, "ROSA Regional Platform API CLI E2E Suite")
}
```

This runs before any specs execute. If a developer adds a test without a lifecycle label, the suite fails immediately with a clear error pointing to this doc. No CI scripts or grep hacks needed — the enforcement lives in the test suite itself.

**Note**: During the migration period, specs in `cluster_test.go` that only carry bare labels (`setup`, `create`, etc.) will fail this check. The migration to `hcp:` labels should be completed before enabling enforcement.
