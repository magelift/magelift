# Codebase Concerns

**Analysis Date:** 2026-07-27

## Tech Debt

**Multi-cloud providers are infra-graph stubs, not operable targets:**
- Issue: GCP, OVH, and Scaleway targets provision infrastructure via Pulumi but almost every Magento day-2 operation (logs, exec, secrets, state, deploy orchestration) returns `ErrNotSupported`. Only AWS ECS Fargate is certified.
- Files: `internal/cloud/ovh/stack/ops.go`, `internal/cloud/scaleway/stack/ops.go`, `internal/cloud/gcp/ops/day2.go`, `internal/cloud/aws/eksops/ops.go` (56 total `ErrNotSupported` call sites across `internal/`)
- Impact: Anyone selecting a non-AWS-Fargate target gets a working `preview`/`apply` but a mostly non-functional operations surface. Docs (`docs/ovh-experimental.md`, `docs/gcp-experimental.md`, `docs/scaleway-experimental.md`, `docs/capability-matrix.md`) label this "experimental," so it is documented debt rather than hidden, but the volume of stubbed day-2 code is a maintenance surface that will grow with every new provider.
- Fix approach: Do not add more providers until GCP reaches certified status (per `docs/post-beta-roadmap.md`); track ADR 0007/0008 for the multi-provider commitment gate.

**Cost estimation only implemented for AWS:**
- Issue: `internal/cloud/aws/cost/estimate.go` is the only `CostEstimator` implementation. `magelift cost` for GCP/OVH/Scaleway/EKS silently has no adapter.
- Files: `internal/cli/cost.go`, `internal/platform/cost.go`, `internal/cloud/aws/cost/estimate.go`
- Impact: Cost visibility gap for experimental providers; listed explicitly as post-beta work in `docs/post-beta-roadmap.md` ("Provider cost adapters").
- Fix approach: Implement per-provider adapters when each provider is promoted toward certification.

**OpenSearch SigV4 data-plane unverified against a live MageLift stack:**
- Issue: `docs/release-readiness.md` documents that the OpenSearch/SigV4 proxy wiring is only validated via Pulumi mocks and prior (non-MageLift) Chantelle Terraform ops, not a real MageLift-provisioned stack.
- Files: search wiring in `internal/cloud/aws/search/search.go`, referenced test `TestRuntimeAddsSigV4ProxyForMagentoOpenSearch` in `internal/cloud/aws/runtime/runtime_test.go`
- Impact: Index creation, storefront search queries, reconnect-after-recycle, and least-privilege IAM for the live SigV4 path are unverified. Explicitly gated as "do not claim live Magento search on MageLift acceptance is green until a paid pass."
- Fix approach: Paid AWS acceptance checklist already defined in `docs/release-readiness.md`; execute before claiming this capability production-ready.

**`ecs-artemis` and `ecs-rabbitmq` queue modes run single-task, no HA:**
- Issue: `docs/capability-matrix.md` and `docs/post-beta-roadmap.md` both note the ECS RabbitMQ path is single-node today; the 3-node quorum HA ladder (modeled on a prior Chantelle Terraform pattern) is deferred.
- Files: `internal/cloud/aws/queue/component.go`
- Impact: Any deployment using `queueMode: ecs-rabbitmq` or `ecs-artemis` has no broker redundancy; a task recycle is a full queue outage.
- Fix approach: Post-beta roadmap item "ECS RabbitMQ HA ladder" — do not flip 1↔3 node count in place; needs EFS+AMQPS single-node → 3-node quorum migration path designed first.

## Known Bugs (recently fixed — watch for regressions)

**OVH MKS node pool dependency ordering:**
- Symptoms: Node pool was not wired into k8s workload dependencies, risking apply-order races.
- Files: `internal/cloud/ovh/stack/ops.go` (commit `37081b7`)
- Trigger: Applying an OVH MKS stack before this fix could deploy workloads before the node pool was ready.
- Status: Fixed in `37081b7 fix(ovh): wire MKS node pool into k8s workload dependencies` — no regression test noted beyond the fix commit itself; verify test coverage exists for dependency ordering before extending OVH further.

**OVH subnet carve validation off the /24 index cap:**
- Symptoms: Subnet carving accepted values beyond the actual /24 index cap.
- Files: `internal/cloud/ovh/stack/ops.go` (commit `780a969`)
- Trigger: Any OVH network config near the subnet index boundary.
- Status: Fixed; confirm the fix has an explicit boundary test (index == cap, index == cap+1).

**Deleted `cost_test.go` with no replacement:**
- Symptoms: `internal/cli/cost_test.go` (107 lines) was deleted in the working tree with no new test file added for `internal/cli/cost.go` in this change set.
- Files: `internal/cli/cost.go` (66 lines, currently uncovered by any visible test in the working tree)
- Impact: Regression risk for CLI cost command behavior (flag parsing, error paths, `ErrNotSupported` handling for non-AWS targets) with no test safety net.
- Fix approach: Confirm whether coverage moved elsewhere (e.g., into `internal/platform/cost.go` tests) before merging; if not, this is a coverage regression that should block the change.

## Security Considerations

**IAM permissions boundary size constraint:**
- Risk: CI IAM permissions boundary policy must stay under AWS's 6KiB size limit (fixed previously in `8c3a4c6`), meaning future IAM policy additions can silently fail to apply or get truncated if the boundary grows without size-checking.
- Files: bootstrap/IAM code in `internal/cloud/aws/bootstrap/identity.go` (442 lines), `internal/cloud/aws/bootstrap/bootstrap.go`
- Current mitigation: Prior fix trimmed the policy; no visible automated guard against future policy growth.
- Recommendation: Add a lint/test asserting the rendered boundary policy JSON stays under the AWS size limit so this doesn't regress silently.

**AOSS (OpenSearch Serverless) collection-group OCU values previously wrong:**
- Risk: Fixed in `b8b957e fix: require AOSS collection-group OCU values AWS actually accepts` — indicates the search capacity config previously could submit invalid/rejected values to AWS.
- Files: `internal/cloud/aws/search/search.go`
- Current mitigation: Values now validated against what AWS accepts.
- Recommendation: Add table-driven tests enumerating the full valid OCU range to prevent silent drift if AWS changes accepted values again.

**Composer/GCP Secret Manager credentials incomplete:**
- Risk: `docs/gcp-experimental.md` states Secret Manager Composer credentials wiring is "incomplete — fail loudly where unimplemented." Confirm all unimplemented secret paths genuinely fail loudly rather than silently no-op.
- Files: `internal/cloud/gcp/ops/day2.go`
- Recommendation: Audit for any code path returning `nil`/success instead of `ErrNotSupported` in the GCP secrets flow.

## Performance Bottlenecks

**golangci-lint CI has repeated OOM/timeout issues:**
- Problem: Five of the last ~10 commits on `main` are CI remediation for golangci-lint running out of memory or timing out on cold multi-cloud Pulumi graphs (`b5deb3d`, `91b8ecb`, `a8173af`, `3a0ec0f`).
- Files: `.github/workflows/ci.yml`, `.golangci.yml` (currently `run.concurrency: 1`, `timeout: 30m`)
- Cause: Type-checking multiple large Pulumi provider SDK graphs (AWS + GCP + OVH + Scaleway) simultaneously exceeds available CI memory when run concurrently; lint had to be serialized (`concurrency: 1`) and given extended timeout as a workaround rather than a root-cause fix.
- Improvement path: As more providers are added, this will worsen. Consider splitting lint by module/build-tag per provider, or running provider-specific lint jobs in a matrix with lower per-job memory footprint instead of one serialized 30-minute job.

**Large single-file components in the AWS stack graph:**
- Files by size: `internal/cloud/aws/runtime/runtime.go` (991 lines), `internal/cli/root.go` (530 lines), `internal/cloud/aws/stack/spec.go` (529 lines), `internal/config/config.go` (502 lines), `internal/cloud/aws/network/network.go` (489 lines), `internal/cloud/aws/queue/component.go` (462 lines), `internal/cloud/aws/observability/component.go` (460 lines), `internal/cloud/aws/stack/component.go` (456 lines), `internal/cloud/aws/bootstrap/identity.go` (442 lines)
- Concern: `runtime.go` at 991 lines is the largest non-test file in the repo and is central to the certified AWS path (ECS Fargate task/service wiring, web runtime selection, SigV4 sidecar injection). Its size makes it the highest-risk file for review fatigue and merge conflicts.
- Improvement path: If further AWS runtime features are added (e.g., ECS Managed Instances launch type per the post-beta roadmap), split `runtime.go` by concern (task definition, service, sidecar injection) before it grows further.

## Fragile Areas

**AWS Fargate `runtime.go` — central dependency for all catalog cells:**
- Files: `internal/cloud/aws/runtime/runtime.go` (991 lines), tested by `internal/cloud/aws/runtime/runtime_test.go` (586 lines)
- Why fragile: Single file wires web runtime (`nginx-fpm` vs `frankenphp-classic`), queue mode selection, SigV4 OpenSearch sidecar, and Magento env — a change to one catalog cell (e.g., queue mode) risks unintended interaction with another (e.g., search sidecar injection) since they share the same task/service construction path.
- Safe modification: Always run the full `runtime_test.go` suite and check for cross-cell test cases (e.g., `TestRuntimeAddsSigV4ProxyForMagentoOpenSearch` combined with queue mode variants) before changing shared construction logic.
- Test coverage: Ratio of test-to-source lines (586:991) suggests reasonable coverage, but coverage of *combinations* of catalog cells (queue × search × web runtime) should be explicitly verified, not assumed.

**Amazon MQ CLUSTER_MULTI_AZ vs preview preset AZ count mismatch:**
- Files: `internal/cloud/aws/queue/component.go`, `internal/cloud/aws/queue/component_test.go`
- Why fragile: `docs/capability-matrix.md` explicitly documents that `queueMode: amazon-mq` is "incompatible by design" with the `preview` preset because Amazon MQ CLUSTER_MULTI_AZ requires 3 AZs while preview is 2-AZ. This is a known landmine for any preset/queue-mode combination logic; a future preset change (e.g., new custom presets) could silently reintroduce broken combinations.
- Safe modification: Any change to preset AZ counts or queue mode defaults must be cross-checked against this incompatibility matrix in `docs/capability-matrix.md`.

**Subnet/network carving math (per-provider):**
- Files: `internal/cloud/aws/network/network.go`, `internal/cloud/ovh/stack/ops.go`, `internal/cloud/scaleway/stack/ops.go`
- Why fragile: Multiple recent fix commits touch subnet index/carve boundary bugs (`780a969` OVH). CIDR carving logic tends to have off-by-one risk at range boundaries across providers, each with separate implementations rather than a shared, well-tested utility.
- Safe modification: When adding a new provider's network carving, write explicit boundary tests (min index, max index, max index + 1) mirroring the OVH fix pattern rather than reusing ad hoc logic per provider.

## Scaling Limits

**IAM permissions boundary policy size (AWS):**
- Current capacity: Must stay under AWS's 6KiB inline policy limit (documented fix in `8c3a4c6`).
- Limit: Adding more fine-grained permissions to the CI service role's boundary risks exceeding this hard AWS limit again.
- Scaling path: Move to policy composition via managed policies/policy sets if the boundary approaches the limit again, rather than continuing to hand-trim a single JSON document.

**Serialized golangci-lint CI job:**
- Current capacity: `concurrency: 1` in `.golangci.yml` serializes all lint checking into one 30-minute job.
- Limit: Each new provider package (GCP, OVH, Scaleway, future providers) adds to the type-checking graph size that this single serialized job must process; the timeout has already been raised once (`a8173af`).
- Scaling path: Partition lint by provider directory/build tag into parallel low-memory jobs instead of continuing to raise the timeout on one large serialized job.

## Dependencies at Risk

**YAML v4 pinned to a release candidate:**
- Risk: `docs/dependency-policy.md` states MageLift uses a pinned YAML v4 release candidate because "upstream has frozen v3... until v4 reaches a final release, its pinned release candidate must pass all strict-decoding, merge, and provenance tests before each update."
- Impact: Config parsing (`internal/config/model.go`, `internal/config/config.go`, `schema/magelift.schema.json`) depends on pre-release library behavior; an upstream v4 breaking change between RCs could break config decoding silently if not caught by the "strict-decoding, merge, and provenance tests" gate.
- Migration plan: Track upstream go-yaml v4 stabilization; re-verify the full decode/merge/provenance test suite on every RC bump (already policy, but worth flagging as an ongoing risk rather than a one-time decision).

**`go-licenses` ignores `github.com/ovh/pulumi-ovh` due to LICENSE-file misdetection:**
- Risk: `docs/dependency-policy.md` notes this exception is because the Apache-2.0 LICENSE sits at module root while nested Go packages aren't classified by the tool; license is manually recorded in `NOTICE` instead.
- Impact: Any future nested-package license drift in `pulumi-ovh` would not be caught automatically by the license-check CI gate.
- Migration plan: Periodically manually re-verify `pulumi-ovh`'s nested package licenses match the recorded `NOTICE` entry, since the automated gate is blind to this dependency.

## Missing Critical Features

**Multi-cloud claim blocked on GCP certification:**
- Problem: Per `docs/capability-matrix.md`, "Multi-cloud is not claimed until two first-party targets are certified" — only AWS ECS Fargate is certified today; GCP GKE Autopilot is the intended second certified provider but is not yet complete (bootstrap WIF deferred, Secret Manager Composer incomplete).
- Blocks: Any public-facing multi-cloud marketing/positioning claim until GCP reaches parity.

**PaaS importers, brownfield attach, signed remote plugins:** all explicitly deferred to post-beta per `docs/post-beta-roadmap.md` — not gaps to fix now, but scope that should not be assumed present when planning near-term phases.

## Test Coverage Gaps

**`internal/cli/cost.go` test coverage status unclear:**
- What's not tested: `internal/cli/cost_test.go` was deleted (107 lines) in the current working tree with no visible replacement test file for `internal/cli/cost.go`.
- Files: `internal/cli/cost.go`
- Risk: CLI cost command regressions (argument validation, non-AWS `ErrNotSupported` handling, output formatting) would go undetected until manual testing or production use.
- Priority: High — verify before this change is merged whether coverage moved to `internal/platform/cost.go` tests or was genuinely dropped.

**Cross-catalog-cell interaction coverage (AWS runtime):**
- What's not tested (unverified without deeper inspection): Whether `internal/cloud/aws/runtime/runtime_test.go` covers all meaningful combinations of `queueMode` × `searchMode` × `webRuntime`, not just each cell in isolation.
- Files: `internal/cloud/aws/runtime/runtime.go`, `internal/cloud/aws/runtime/runtime_test.go`
- Risk: A change to one catalog cell's wiring could silently break another cell's injected resources (e.g., SigV4 sidecar + FrankenPHP worker mode) without a combinatorial test catching it.
- Priority: Medium — recommend an explicit test matrix review before adding the next catalog cell (e.g., ECS Managed Instances launch type).

---

*Concerns audit: 2026-07-27*
