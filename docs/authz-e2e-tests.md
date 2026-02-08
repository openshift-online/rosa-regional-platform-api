# Authorization Integration E2E Test Plan

This document describes the integration E2E test plan for the ROSA authorization service. Unlike the existing E2E tests (which use local containers with DynamoDB Local and cedar-agent/MockAVPClient), these tests exercise the full authorization lifecycle against real AWS services (AVP + DynamoDB).

## Test Entities

| Entity             | ID                                           | Role                                        |
| ------------------ | -------------------------------------------- | ------------------------------------------- |
| Privileged Account | `000000000000`                               | Pre-seeded in DynamoDB, bypasses all checks |
| Test Account 1     | `111111111111`                               | First customer account                      |
| Test Account 2     | `222222222222`                               | Second customer account (isolation tests)   |
| Admin1 (Acct 1)    | `arn:aws:iam::111111111111:user/admin1`      | First admin, bootstrapped by privileged     |
| Admin2 (Acct 1)    | `arn:aws:iam::111111111111:user/admin2`      | Promoted to admin by Admin1                 |
| User3 (Acct 1)     | `arn:aws:iam::111111111111:user/user3`       | Regular user, subject of Cedar policies     |
| Admin (Acct 2)     | `arn:aws:iam::222222222222:user/admin-acct2` | Admin in second account                     |

## Test Phases

### Phase 1 — Unenabled Account Isolation

Before Account 1 is enabled, any ARN from that account should be unable to do anything:

- Enable accounts → 403 (not privileged)
- Manage policies/groups/attachments/admins → 403 (account not provisioned)

### Phase 2 — Account Enablement & Admin Bootstrap

1. The privileged account enables Account 1 (non-privileged).
2. Verify the account is created with a policy store.
3. The privileged account adds Admin1 as admin for Account 1 via the API.

### Phase 3 — Non-Admin Restriction (All Management Operations)

User3 (not an admin) attempts every management operation and gets 403 for each:

- Create/list policies
- Create/list groups
- Create/list attachments
- Add/list admins

### Phase 4 — Admin Can Manage Resources

Admin1 performs full CRUD on policies, groups, attachments, and group membership. Verify success for each operation.

### Phase 5 — Admin Delegation

1. Admin1 adds Admin2 as admin.
2. Admin2 successfully creates and deletes a policy.
3. Admin2 successfully creates and deletes a group.
4. This proves admin status was granted.

### Phase 6 — Admin Removal

1. Admin1 removes Admin2.
2. Admin2 attempts to create a policy → 403 (not admin).
3. Confirm the removal took effect.

### Phase 7 — Cedar Policy Authorization

Admin1 sets up the test infrastructure for User3. For each policy test file in `pkg/authz/testdata/policies/`:

1. Create the policy.
2. Create a group.
3. Add User3 to the group.
4. Attach the policy to the group.
5. Run each test case via `/api/v0/authz/check` and verify the decision matches the expected result.
6. Clean up after each policy file.

### Phase 8 — Cross-Account Isolation

1. The privileged account enables Account 2 and adds Admin-Acct2 as its admin.
2. Admin-Acct2 tries to manage Account 1's resources (create policy, list groups, add admin) → 403 for each.
3. Admin1 tries to manage Account 2's resources → 403 for each.
4. Policies and groups from one account must not be visible or accessible from the other.

### Phase 9 — Account Deletion

1. The privileged account disables Account 1.
2. Admin1 attempts to create a policy on Account 1 → 403 (account not provisioned).
3. User3 attempts any operation → 403.

### Phase 10 — Cleanup

1. The privileged account disables Account 2.
2. Any remaining test resources are cleaned up.

## Infrastructure

These tests use real AWS services: AVP and DynamoDB. The service runs locally and connects to AWS.

### Prerequisites

- AWS credentials with permissions for:
  - AVP: `CreatePolicyStore`, `DeletePolicyStore`, `CreatePolicyTemplate`, `DeletePolicyTemplate`, `CreatePolicy`, `DeletePolicy`, `IsAuthorized`, etc.
  - DynamoDB: CRUD on authz tables
- DynamoDB tables created (same schema as `scripts/e2e-init-dynamodb.sh`)
- Privileged account (`000000000000`) seeded in the accounts table
- Service started **without** `CEDAR_AGENT_ENDPOINT` (uses real AVP) and **without** `DYNAMODB_ENDPOINT` override (uses real DynamoDB)
