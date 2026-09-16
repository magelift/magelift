---
status: done
slug: magento-deployment-safety
spec: spec.md
---

# Plan: Magento deployment safety (health that means Magento, one lifecycle)

## Files that change

Exact paths. New vs edit. One line each on what changes.

- `internal/cloud/kube/steps.go` (edit): split Health from Stabilize (rollout-identity assertions plus Magento probe); extend ServiceHealth with generation, updated counts, digest.
- `internal/cloud/kube/steps_test.go` (edit): rollout-identity tests (stale-replica fail, intended pass) plus probe-outcome tests with fake clients.
- `internal/cloud/kube/candidate.go` (edit): one-shot Magento probe workload support reusing the candidate Job machinery.
- `internal/cloud/kube/candidate_test.go` (edit): probe workload tests; migration sequence stays valid.
- `internal/cloud/aws/deployment/steps.go` (edit): same Health split for ECS (revision plus digest assertions plus probe).
- `internal/cloud/aws/deployment/steps_test.go` (edit): ECS rollout-identity plus probe tests with fakes.
- `internal/cloud/aws/operations/runtime.go` (edit): extend ServiceHealth with revision and digest fields from DescribeServices data.
- `internal/cloud/aws/operations/deployment.go` (edit): one-shot Magento probe task support reusing the candidate task machinery.
- `internal/cloud/aws/operations/deployment_test.go` (edit): probe task tests.
- `internal/platform/workloads.go` (edit): add the fixed Magento probe shell; drop deploy-time `setup:static-content:deploy` from the migration shell.
- `internal/platform/workloads_test.go` (edit): pin migration shell (PHP-matching, no deploy-time SCD) plus probe shell shapes; existing queue tests kept.
- `internal/cloud/gcp/operations/candidate_test.go` (edit): update the old-sequence assertion to the PHP-matching sequence.
- `internal/cloud/gcp/operations/jobs_k8s.go` (verify-only, zero edits expected): inherits the fixed shell; confirm no local SCD logic.
- `internal/cloud/aws/runtime/env.go` (verify-only, zero edits expected): inherits the fixed shell; confirm no local SCD logic.
- `internal/build/plan/plan.go` (edit): require `build.staticContent` locales plus themes with a clear error.
- `internal/build/plan/plan_test.go` (edit): missing-staticContent failure test plus existing passing cases.
- `examples/sample-shop/magelift.yaml` (edit): add the required staticContent keys.
- `internal/cli/lifecycle.go` (edit): `--ack-maintenance-drain` flag threaded into deploy options.
- `internal/deploy/orchestrator.go` (edit): production-class gate requiring the attestation before mutate.
- `internal/cli/lifecycle_test.go` (edit): flag threading plus refusal tests.
- `internal/deploy/orchestrator_test.go` (edit): production-gate tests.
- `internal/cloud/aws/runtime/containers.go` (edit): comment-only update (writable root resolved deliberate, no behavior change).
- `docs/operations.md` (edit): incompatible-migration runbook plus Health-semantics section.
- `docs/architecture.md` (edit): lifecycle-authority plus runtime-storage rationale plus readiness/liveness split.
- `agents/skills/magelift-operate/SKILL.md` (edit): production-deploy rule with runbook pointer plus Health meaning.
- `agents/manifest.json` (regenerate via generator only): digest refresh for the skill edit.
- `build/` PHP tree (verify-only, zero edits expected): LifecyclePlan already correct; suite runs as regression.

## Order of work

Build and verify order, not a task dump. Group by area, number within the group.
Each box carries the check that closes it.

- [x] 1.1 Split kube Health with rollout-identity assertions and extended ServiceHealth (probe hookup lands in 1.3) — verify: `go test ./internal/cloud/kube/ -run 'TestHealth|TestStabilize' -count=1` passes including new stale-replica-fail and intended-pass cases
- [x] 1.2 Split ECS Health with rollout-identity assertions and extended AWS ServiceHealth (probe hookup lands in 1.3) — verify: `go test ./internal/cloud/aws/deployment/ ./internal/cloud/aws/operations/ -count=1` pass including new stale-reject and intended-accept cases
- [x] 1.3 Add the one-shot Magento probe workloads both platforms (db:status plus search checks when enabled, explicit timeout) — verify: probe success/failure/timeout cases pass offline with fakes on both sides
- [x] 2.1 Drop deploy-time SCD from the migration shell; update the gcp sequence test — verify: no `setup:static-content:deploy` in `workloads.go`, the three callers inherit it (grep), and their suites pass
- [x] 2.2 Require `build.staticContent` locales plus themes; update the sample shop — verify: missing-keys build fails with the documented error, the sample builds-plan clean
- [x] 2.3 Assert candidate/serving digest identity in Health — verify: a digest-mismatch case fails Health naming the mismatch on both platforms
- [x] 3.1 Add the production `--ack-maintenance-drain` gate (flag plus orchestrator refusal before mutate) — verify: production deploy without the flag refuses naming flag plus runbook; with it proceeds (fake-gated)
- [x] 4.1 Write the runbook, storage rationale, and skill alignment; flip the writable-root comment to deliberate — verify: runbook sequence plus old-writer rule plus no-rollback statement present; storage section pins each path; skill contradicts nothing; `internal/cloud/aws/runtime` diff is comment-only
- [x] 4.2 Run humanizer, then remove-ai-marks, on touched human pages — verify: both passes completed (or recorded unperformed) and boxes 1.1–4.1 verifies still pass
- [x] 5.1 Prove the five failure classes plus no collateral damage — verify: one named offline test per class passes; `go test -tags synthetic ./tests/synthetic/`, the PHP LifecyclePlan suite, and `make docs` all pass

## Risks

What could break, and the check for each.

- Rollout-identity fields unavailable from a checker backend (ECS revision data, kube digest): box 1.1/1.2 fail on missing data; extend the checker structs (already in the file list) rather than weakening asserts.
- Probe workload seams differ per platform (Job vs RunTask specifics): box 1.3 pins one command with per-platform runners; timeout plus output-attach behavior asserted on fakes.
- staticContent requirement breaks in-tree example or test YAMLs beyond the sample: box 2.2 fails naming them; update in the same change (plan file list moves only by append).
- Ack flag threading misses a production entry path (promote vs deploy): box 3.1 tests every production-mutate path the flag must cover; strays fail the box.
- Steady-state probe temptation (switching readiness to exec): out of scope by design; any readinessProbe/livenessProbe diff fails review.
- PHP suite environment (libargon2 precedent): box 5.1 runs the LifecyclePlan suite; environmental failures are recorded as such with the pre-existing signature, never fixed by editing PHP behavior here.

## Proof

The end-to-end evidence that the whole spec is met, not the per-step verifies
above. Tests, commands, or screenshots.

- Affected Go suites green: kube steps/candidate, aws deployment/operations, platform callers, build plan, cli lifecycle, deploy orchestrator.
- Synthetic suite still green (no offline-behavior collateral).
- PHP LifecyclePlan suite green (or pre-existing environmental signature recorded).
- `make docs` exits 0.
- No cloud resources created, changed, or destroyed (fakes only).
