# Roadmap: MageLift v1.0.0

## Overview

This is a brownfield hardening-and-capability milestone on a codebase that already deploys production Magento on AWS ECS Fargate. The journey has one binding constraint — self-funded cloud credits — so the roadmap is ordered to spend as little as possible and to spend it late, after offline verification has bought everything mocks can buy.

The shape is: clear the debt and make the product honest about its own limits (Phase 1), make `v1.0.0-rc.1` taggable (Phase 2), then build the acceptance harness that makes every later paid pass cheap (Phase 3). With that in place, the two independent capability tracks run: the brownfield onramp that lets a real store move onto MageLift (Phases 4-5, fully offline), and the shared Kubernetes day-2 layer that makes GCP certification affordable and lifts EKS/MKS/Kapsule with it (Phases 6-7, one paid GCP pass). The hardest and narrowest capability, adopting infrastructure MageLift did not create, comes last alongside the gate-board audit (Phase 8).

Phases 4-8 are deliberately droppable in reverse order. A maintainer who wants to tag `v1.0.0-rc.1` after Phase 3 can do so without leaving anything half-wired: Phases 1-3 deliver a repository that lints, tells the truth about every capability tier, and has a closed packaging gate.

## Phases

**Phase Numbering:**

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Publishable Baseline & Honest Fallbacks** - Clear the debt that worsens with every later phase, and make every target state its own tier
- [x] **Phase 2: Tag-Ready Release Surface** - Make `v1.0.0-rc.1` taggable: version story, contract statement, packaging smoke, contributor path
- [x] **Phase 3: Credit-Efficient Acceptance Harness & Evidence Tiering** - One long-lived stack, resumable runs, automatic evidence, and a matrix that never over-claims
- [x] **Phase 4: Brownfield Onramp — PaaS Import & ece-tools Parity** - A store on Adobe Commerce Cloud or Upsun gets a reviewable `magelift.yaml` and a build system it can trust (completed 2026-07-29)
- [x] **Phase 5: Data Migration & Cutover** - `seedDump` stops being a status string; a documented cutover moves a live store over (completed 2026-07-29)
- [x] **Phase 6: Shared Kubernetes Day-2** - One `Observe` and one `deploy.Steps` in `internal/cloud/kube`, inherited by all four Kubernetes targets (completed 2026-07-30)
- [ ] **Phase 7: GCP Certification** - GKE Autopilot reaches certified tier on real-account evidence, making multi-cloud truthful
- [ ] **Phase 8: Brownfield Attach & Tag Day** - Adopt existing VPC and database safely, then close or defer every gate-board row

## Cloud Spend Map

Decision-relevant for a self-funded solo maintainer. The plan is **three paid passes total**.

| Phase | Cloud spend | What pays for it |
|-------|-------------|------------------|
| 1 | **None** | `make verify`, `make test -race`, `make floci-test` |
| 2 | **None** | `make release-smoke` (local, serial, single-target, outside Cursor) |
| 3 | **PAID — pass 1 of 3** | One AWS free-tier `preview` stack proving the harness itself (single stack, resume, evidence, `assert_clean`) |
| 4 | **None** | Fixture ACC/Upsun repositories, `make test`, `make php-test` |
| 5 | **Mostly none** | `magelift dev` MySQL + Floci for media; one real dump-import cell rides Phase 7's pass |
| 6 | **None** | Fake clientset / envtest, Pulumi mocks, local S3-compatible endpoint for OVH/Scaleway state |
| 7 | **PAID — pass 2 of 3** | One batched GCP pass (~19m21s create + PSA soak + force-clean); do not enter until 3, 5, 6 are green offline |
| 8 | **PAID — pass 3 of 3, small** | Floci for import mechanics; one free-tier AWS confirmation of a real VPC + RDS adoption |

Never depend on: Aurora `CreateDBCluster` (free-tier API block), `amazon-mq` × `preview` (2-AZ vs 3-AZ, incompatible by design), or the live OpenSearch SigV4 data plane (deferred to post-tag paid acceptance).

## Architectural Guardrails

Every phase respects these; they are not phase work, they are constraints on phase work.

- **ADR 0002/0004** — cloud adapter internals must not leak into `internal/cli` or `internal/deploy`; only `sdk/v1` types and `internal/platform` interfaces cross the boundary. The shared Kubernetes layer in Phase 6 lives in `internal/cloud/kube` and is reached through `platform`, not by the CLI importing it.
- **ADR 0007/0008** — no fifth provider in this milestone; the multi-provider gate opens only once a second target is certified (Phase 7).
- **Split by port nature, not by provider** — k8s-shaped ports (`Observe`, `deploy.Steps`) shared once; cloud-shaped ports (`Bootstrap`, `State`, `Secrets`, `CostEstimator`) stay per-provider.
- **Nothing claims a tier above its recorded evidence** — enforced in the product from Phase 1 and in the matrix from Phase 3.

## Phase Details

### Phase 1: Publishable Baseline & Honest Fallbacks

**Goal**: The repository survives a stranger's first read — CI actually runs and passes the verification suite for the first time, the highest-risk file is no longer a single 991-line construction path, every recently fixed bug has a regression guard, and no target can silently pretend to support something it does not.
**Depends on**: Nothing (first phase)
**Cloud spend**: None — fully offline
**Requirements**: QUALITY-01, QUALITY-02, QUALITY-03, QUALITY-04, QUALITY-05, QUALITY-06, QUALITY-07, QUALITY-08, TRUST-01, TRUST-02

**Criteria revised 2026-07-27** after Phase 1 research contradicted five claims in `.planning/codebase/CONCERNS.md` (all verified against code and git history). See `01-RESEARCH.md`. Corrections applied:

- The `go` CI job has **never passed** — failure ×11, cancelled ×4, skipped ×10, success ×0 across the last 25 runs. Runs showing green had `go` *skipped* by the change-detection gate. Because `lint` kills the runner, every later `make verify` target (`test`, `license-check`, `php-test`, `generate-check`, `cli-docs-check`, `docs`, `workflow-check`) has also never executed in CI. Criterion 1 is therefore "make the job green for the first time," not "remove a workaround." No branch protection exists (private repo, no Pro), so splitting the job breaks nothing.
- QUALITY-02's 6KiB guard **already exists** at `internal/cloud/aws/bootstrap/identity_test.go:242` (`if len(boundary) > 6144`) — scoped to verify-and-document, not new work.
- QUALITY-01's deleted `cost_test.go` tested functions that no longer exist (legitimate refactor, not a regression) — scoped to writing *new* tests for the current `internal/cli/cost.go`.
- QUALITY-08's OVH bugs live in `internal/cloud/ovh/runtime/runtime.go` (`37081b7`) and `internal/cloud/ovh/network/network.go` (`780a969`), **not** `stack/ops.go`. The subnet-cap fix already has a test; the node-pool fix does not, and `internal/cloud/ovh/runtime/` has **zero test files**.
- QUALITY-05: only OVH has a subnet index cap. AWS carves `len(AZs)*3` with no cap (`internal/cloud/aws/network/network.go:391`), GCP has none past index 8, and Scaleway carves no subnets at all (`internal/cloud/scaleway/network/network.go:79`, comment at `:87`). Decision: add the missing caps, then test them.
- TRUST-02 as originally written was satisfiable while leaving worse silent-success paths intact. Extended to cover them.

**Success Criteria** (what must be TRUE):

  1. The `go` job passes on `main` — genuinely, not by being skipped. Lint runs as partitioned jobs (per provider plus `core` and `cmd/magelift`, which the import graph forces) with `run.concurrency: 1` and the 30-minute timeout removed from `.golangci.yml`, AND every subsequent `make verify` target executes and passes in CI: `generate-check`, `cli-docs-check`, `fmt-check`, `test` (`go test -race ./...`), `license-check`, `php-test`, `docs`, `workflow-check`. Partitioning must not produce `unused` false positives across partition boundaries — verified, not assumed.
  2. No **non-test** file under `internal/cloud/aws/runtime/` exceeds 400 lines, and the pre-existing `runtime_test.go` suite (including the combination tests added under criterion 3) passes unchanged after the split — proving the refactor moved code without changing behaviour. `TestRuntimeAddsSigV4ProxyForMagentoOpenSearch` must not regress.
  3. `go test -race ./...` includes new guards that fail on regression: an AOSS OCU value AWS rejects; `queueMode` × `searchMode` × `webRuntime` in combination, restricted to combinations valid per `docs/capability-matrix.md` (never asserting on `amazon-mq` × `preview`, incompatible by design); `internal/cli/cost.go` flag parsing / error paths / `ErrNotSupported` on non-AWS targets; OVH MKS node-pool dependency ordering in a newly created `internal/cloud/ovh/runtime/` test file; and subnet index at min, cap, and cap+1 for OVH, AWS, and GCP — with explicit index caps **added** to AWS and GCP carving where none exists today, failing with a clear error. Scaleway's non-carving is documented as N/A. The existing 6KiB boundary guard is confirmed still present and its coverage recorded.
  4. Any mutating command against a target whose certification tier is `platform.TierExperimental` prints a tier-naming warning before Pulumi is invoked — keyed on **tier, not a provider allowlist**, so `aws/eks-autopilot` is covered and no future experimental target can slip through. Asserted by a CLI test, not by reading docs.
  5. No experimental target can silently appear to succeed. Specifically: every unimplemented day-2 command exits non-zero naming the capability and its tier, with a test enumerating the `ErrNotSupported` sites (15 each in `internal/cloud/ovh/stack/ops.go` and `internal/cloud/scaleway/stack/ops.go`) and asserting none returns a nil-success path; `magelift deploy` no longer silently degrades to infrastructure-only when `Ops` returns `ErrNotSupported` (`internal/cli/lifecycle.go:176-180`) but announces it loudly or refuses without an explicit flag; and no-op `AcquireLock` implementations warn that no lock was taken rather than returning a release function that implies one was.

**Plans**: 10/10 plans executed

- [x] 01-01-PLAN.md
- [x] 01-02-PLAN.md
- [x] 01-03-PLAN.md
- [x] 01-04-PLAN.md
- [x] 01-05-PLAN.md
- [x] 01-06-PLAN.md
- [x] 01-07-PLAN.md
- [x] 01-08-PLAN.md
- [x] 01-09-PLAN.md
- [x] 01-10-PLAN.md — gap closure: QUALITY-07 runtime split + TRUST-02 AcquireLock warn

### Phase 2: Tag-Ready Release Surface

**Goal**: `v1.0.0-rc.1` becomes taggable — the version story is consistent everywhere, the contract carries an explicit RC stability statement, the packaging gate is closed, and a stranger can go from clone to green.
**Depends on**: Phase 1 (CONTRIBUTING.md promises a green `make verify`, which Phase 1 makes true)
**Cloud spend**: None
**Requirements**: RELEASE-01, RELEASE-02, RELEASE-03, RELEASE-04, RELEASE-06
**Success Criteria** (what must be TRUE):

  1. `README.md`, `docs/versioning.md`, and `docs/release-readiness.md` each name `v1.0.0-rc.1` as the first public tag, and no remaining sentence in them describes the project as pre-alpha or `v0.x` (CHANGELOG history excepted)
  2. `docs/versioning.md` carries a stability statement for `sdk/v1` and `platform.StackModule` that names what may change during the RC series — explicitly reserving the shared-Kubernetes port changes planned for Phase 6
  3. `make release-smoke` completes on the maintainer's machine in a plain Terminal (serial, single-target), and the packaging-smoke row moves from Partial to Closed with the run date and output recorded
  4. A fresh clone, following only the steps written in `CONTRIBUTING.md`, reaches a green `make verify` — run in a clean checkout, not from the working tree
  5. `examples/custom-cli` builds and registers an out-of-tree provider following `docs/adding-a-provider.md` alone, verified from a clean module cache with no core source consulted

**Plans:** 4/4 plans executed

Plans:

- [x] 02-01-PLAN.md — Version story + RC contract (`v1.0.0-rc.1`, sdk/v1 + StackModule stability, Phase 6 reservation)
- [x] 02-02-PLAN.md — CONTRIBUTING verify honesty + fresh-clone `make verify` proof
- [x] 02-03-PLAN.md — Custom-cli clean-GOMODCACHE proof + community stub registration
- [x] 02-04-PLAN.md — HUMAN_GATE plain Terminal `make release-smoke` → Packaging smoke Closed

### Phase 3: Credit-Efficient Acceptance Harness & Evidence Tiering

**Goal**: Every later paid pass costs one stack and buys only what mocks cannot prove — and no cell in the capability matrix claims more than the harness actually recorded.
**Depends on**: Phase 1 (the tier labels TRUST-01/02 introduce are what the matrix records per cell)
**Cloud spend**: **PAID** — one AWS free-tier `preview` pass to prove the harness. The GCP harness path (ACCEPT-05) is written here and first exercised for real in Phase 7.
**Requirements**: ACCEPT-01, ACCEPT-02, ACCEPT-03, ACCEPT-04, ACCEPT-05, ACCEPT-06, TRUST-03, TRUST-04
**Success Criteria** (what must be TRUE):

  1. An acceptance run iterates at least three catalog cells against a single stack — the run log shows one Pulumi create followed by updates, never a re-create between cells — and killing the run mid-matrix then re-invoking it resumes at the first uncompleted cell, skipping recorded ones
  2. After a run, `.magelift/matrix-results.md` has one appended row per cell carrying cell, result, duration, provider, account, and date, written by the harness — the git diff shows no hand-typed evidence
  3. Teardown destroys everything it created and `assert_clean` exits non-zero when a resource is deliberately left behind, zero on a clean account — both outcomes demonstrated, not assumed
  4. The GCP harness path runs the same shape end to end including PSA soak and force-clean teardown, verified at least as far as a dry run / preview pass before Phase 7 spends credits
  5. `docs/capability-matrix.md` records an evidence tier per cell plus an explicit unverifiable reason for Aurora `CreateDBCluster`, `amazon-mq` × `preview`, and the OpenSearch SigV4 data plane; a checked-in port-coverage table maps every day-2 port to mocks / Floci / paid-only, and `make floci-test` covers every port marked mockable

**Plans:** 6/6 plans executed

Plans:

- [x] 03-01-PLAN.md — Offline harness core: cell catalog, checkpoint/resume, evidence append, dry-run
- [x] 03-02-PLAN.md — Shared assert_clean + offline dual-outcome stub tests
- [x] 03-03-PLAN.md — Capability-matrix evidence tiers, unverifiable reasons, port-coverage table
- [x] 03-04-PLAN.md — Floci/unit-fake coverage for every mockable day-2 port
- [x] 03-05-PLAN.md — GCP harness same shape via preview/dry-run (no live up)
- [x] 03-06-PLAN.md — HUMAN_GATE paid AWS free-tier create-once proof (≥3 cells, resume, evidence, assert_clean)

### Phase 4: Brownfield Onramp — PaaS Import & ece-tools Parity

**Goal**: A store already running on Adobe Commerce Cloud or Upsun can generate a reviewable `magelift.yaml` from its existing config and trust that MageLift's PHP build system does what `ece-tools` did for it.
**Depends on**: Phase 1 (the loud-failure mechanism unmappable keys use), Phase 2 (the schema and version story the generated file targets). Independent of all cloud work — may run in parallel with Phases 6-7.
**Cloud spend**: None — fixture repositories, `make test`, `make php-test`
**Requirements**: IMPORT-01, IMPORT-02, IMPORT-03, IMPORT-04, IMPORT-05, IMPORT-06, ECE-01, ECE-02, ECE-03, ECE-04
**Success Criteria** (what must be TRUE):

  1. `magelift init --from-acc` in an Adobe Commerce Cloud fixture repository and `magelift init --from-upsun` in an Upsun fixture repository each write a `magelift.yaml` that passes `magelift config validate` and validates against `schema/magelift.schema.json` with zero hand edits
  2. A fixture carrying unmappable keys produces a report naming every source key that could not be translated and refuses to claim success, with tests asserting key-by-key coverage of application config, services, routes, and cron definitions
  3. Import output is always a file the operator diffs first: `magelift config validate` and `magelift deploy` reject `.magento.app.yaml` / `.platform.app.yaml` as direct input (test asserts), and the importer's clean-room provenance is recorded in `docs/knowledge/` with a check confirming no reference-repository source was vendored
  4. `docs/ece-parity.md` enumerates every `ece-tools` build, deploy, and post-deploy hook plus every env-var-driven Magento setting in the ACC/Upsun shape, each marked closed or intentionally-gapped with a reason — no row blank
  5. `make php-test` covers `magento-cloud-patches`-style patch application and static-content-deploy settings (locales, themes, strategy, thread count) for the supported configurations

**Plans:** 6/6 plans complete

Plans:

- [x] 04-01-PLAN.md — Tracer: ACC fixture → init --from-acc → Load-valid YAML + foreign-schema reject
- [x] 04-02-PLAN.md — Init CLI contract: --from-* mutual exclusion, refuse/--yes, --config-out (D-03/D-04 gates)
- [x] 04-03-PLAN.md — Shared ACC/Upsun mapper, D-07 allowlist, unmapped sidecar (D-05), cron schema, soak skip
- [x] 04-04-PLAN.md — SCD strategy/threads Go→PHP vertical slice (ECE-03)
- [x] 04-05-PLAN.md — Clean-room m2-hotfixes PatchApplier + php-test (ECE-02)
- [x] 04-06-PLAN.md — docs/ece-parity.md + D-07 docs + migration/provenance + clean-room check

### Phase 5: Data Migration & Cutover

**Goal**: A live store's data actually lands in MageLift — `seedDump` stops being a recorded status string, media follows, and a documented runbook moves a real store over with a way back.
**Depends on**: Phase 3 (the dump-import cell rides the harness rather than buying its own pass), Phase 4 (the imported config is what a migrating team deploys)
**Cloud spend**: Mostly none — verified against the `magelift dev` MySQL and Floci for media; one real dump-import cell batches into Phase 7's GCP pass
**Requirements**: MIGRATE-01, MIGRATE-02, MIGRATE-03, MIGRATE-04, MIGRATE-05
**Success Criteria** (what must be TRUE):

  1. `magelift env create --dump <file>` leaves the dumped tables present and queryable in the target database after the first successful deploy — verified against local dev MySQL and once on a managed instance; `internal/cli/env.go:144` no longer writes a placeholder status in place of doing the work
  2. `magelift env status` shows `seedDumpStatus` moving recorded → importing → imported on a good dump, and `failed` with a readable reason on a deliberately corrupt one
  3. Re-running an import against a non-empty database exits non-zero without an explicit confirmation flag, and an interrupted import re-run converges to the same database state (both asserted by tests)
  4. A media-sync run copies a fixture media tree into the target's object storage and a listing diff against the source is empty
  5. `docs/migrating-from-paas.md` carries a cutover runbook (DNS, maintenance mode, reindex, verification, rollback) that has been followed end to end at least once against a non-production target, with the run recorded

**Plans:** 6/6 plans complete

Plans:

- [x] 05-01-PLAN.md — Config SeedDump + schema generate + create journal recorded (Wave 0 KnownFields)
- [x] 05-02-PLAN.md — seeddump journal state machine + magelift env status merge
- [x] 05-03-PLAN.md — dumpimport local MySQL + nonempty/--yes schema-replace (D-04 gate)
- [x] 05-04-PLAN.md — env import-dump + post-deploy auto-import once-from-recorded (D-01 gate)
- [x] 05-05-PLAN.md — env media-sync --source + Floci/unit listing-diff
- [x] 05-06-PLAN.md — cutover runbook + local scratch proof + Phase 7 HUMAN_GATE split

### Phase 6: Shared Kubernetes Day-2

**Goal**: One Kubernetes day-2 implementation serves GKE Autopilot, EKS Autopilot, OVH MKS, and Scaleway Kapsule — so certifying GCP in Phase 7 lifts three more targets without three more implementations.
**Depends on**: Phase 1 (QUALITY-06 lint partitioning must land before this adds provider code, and TRUST-02 supplies the loud-failure mechanism KUBE-06 relies on), Phase 3 (the mock/Floci coverage targets this work must hit before any paid pass)
**Cloud spend**: None — fake clientset / envtest for the Kubernetes API, Pulumi mocks for graphs, a local S3-compatible endpoint for OVH/Scaleway state. Live exercise happens in Phase 7.
**Requirements**: KUBE-01, KUBE-02, KUBE-03, KUBE-04, KUBE-05, KUBE-06, KUBE-07
**Success Criteria** (what must be TRUE):

  1. `Observe.TailLogs`, `Observe.CheckRuntime`, and `Observe.PrepareExec` exist once in `internal/cloud/kube` and all four Kubernetes modules resolve to that single implementation — a test asserts each module returns the shared type and no provider package defines its own copy
  2. The Kubernetes `deploy.Steps` implementation lives once in `internal/cloud/kube` and drives the full sequence (Validate → RegisterCandidate → RunMigrations → CleanupCandidate → UpdateServices → Stabilize → Health → Record) against a fake cluster, exercised through all four module registrations
  3. State lock, backup, and restore succeed for OVH and Scaleway through the existing S3-compatible manager with nothing but an endpoint override — verified against a local S3-compatible endpoint, with no third state implementation added
  4. `internal/cloud/ovh/stack/ops.go` and `internal/cloud/scaleway/stack/ops.go` hold no `unsupported{}` stub for any capability the shared layer now provides (a grep gate over the 15 `ErrNotSupported` sites each carries today), and every remaining gap returns a tier-named error
  5. `docs/capability-matrix.md` rows for `eks-autopilot`, `mks`, and `kapsule` state the day-2 surface that now works with its evidence tier, while `Bootstrap` and `Secrets` remain per-provider with a test asserting no nil-success path in either

**Plans:** 6/6 plans complete

Plans:

- [x] 06-01-PLAN.md — SSE ObjectEncryption + OVH/SCW State via NewAWSWithEndpoint (KUBE-05)
- [x] 06-02-PLAN.md — OutputKubeconfig + kube client factory + four stack exports
- [x] 06-03-PLAN.md — Shared kube.Observe + collapse GCP + four RuntimeObserve (KUBE-01..03)
- [x] 06-04-PLAN.md — Shared kube.Steps + four NewDeploySteps + fake sequence (KUBE-04)
- [x] 06-05-PLAN.md — Unsupported honesty, AcquireLock flip, matrix/docs (KUBE-06..07)
- [x] 06-06-PLAN.md — Offline integration gates + Phase 7 Cloudflare DNS handoff

### Phase 7: GCP Certification

**Goal**: GCP GKE Autopilot reaches certified tier on real-account evidence, making the multi-cloud claim truthful and opening the ADR 0007/0008 provider gate.
**Depends on**: Phase 6 (the shared `Observe` and `deploy.Steps` must exist before certification exercises them — GCP-03 and GCP-04 are unreachable otherwise), Phase 3 (the harness is what makes this one pass instead of several exploratory ones), Phase 5 (the dump-import cell rides this pass), Phase 1 (GCP-02 needs the loud-failure mechanism so no secrets path returns a false success)
**Cloud spend**: **PAID** — the single efficient GCP pass (~19m21s create, plus PSA soak and force-clean teardown). Do not enter this phase until Phases 3, 5, and 6 are green offline.
**Requirements**: GCP-01, GCP-02, GCP-03, GCP-04, GCP-05, GCP-06
**Success Criteria** (what must be TRUE):

  1. `magelift` bootstrap provisions Workload Identity Federation on a real GCP project and a CI run authenticates successfully with no long-lived service-account key present in repository secrets
  2. Composer credentials are written to and read back from GCP Secret Manager during both build and deploy on that project, and a test asserts every GCP secrets path either succeeds for real or fails loudly — none returns nil success
  3. `logs`, `exec`, `secrets`, `state`, and `health` each succeed against the live GKE Autopilot target, with a harness-recorded evidence row per command in `.magelift/gcp-matrix/matrix-results.md`
  4. `magelift deploy` completes migrate → cutover → health → record on GKE Autopilot and the resulting release is readable from the releases journal
  5. `magelift cost` returns a per-cell estimate for the GCP target, and both `docs/capability-matrix.md` and the `docs/release-readiness.md` gate board record `gcp` / `gke-autopilot` as certified, citing this acceptance pass as evidence

**Plans:** 1/7 plans executed

Plans:
- [x] 07-01-PLAN.md — Offline WIF bootstrap + Composer Secret Manager Get (GCP-01, GCP-02)
- [ ] 07-02-PLAN.md — GCP account-free CostEstimator (GCP-05)
- [ ] 07-03-PLAN.md — Harness cell catalog + live_cell_loop dry-run (GCP-03..05 wiring)
- [ ] 07-04-PLAN.md — Kube-adjacent managed dumpimport runner (MIGRATE-04 dump)
- [ ] 07-05-PLAN.md — Cloudflare DNS cutover script offline (MIGRATE-04 DNS, D-04)
- [ ] 07-06-PLAN.md — Live paid create-once pass + force_clean/PSA + DNS cleanup
- [ ] 07-07-PLAN.md — Certify capability-matrix + release-readiness (GCP-06, MIGRATE-04 close)

### Phase 8: Brownfield Attach & Tag Day

**Goal**: The hardest brownfield case works safely — adopting infrastructure MageLift did not create, without ever destroying it — and the gate board tells the truth on tag day.
**Depends on**: Phase 7 (the board cannot be closed before the GCP row is settled) and, transitively, every earlier phase, since RELEASE-05 audits all of them. Attach itself is independently droppable: deferring ATTACH-01..04 to a later RC leaves nothing half-wired, provided the deferral is recorded on the board.
**Cloud spend**: **PAID but small** — Floci for the import mechanics, one free-tier AWS confirmation adopting a real VPC and RDS instance
**Requirements**: ATTACH-01, ATTACH-02, ATTACH-03, ATTACH-04, RELEASE-05
**Success Criteria** (what must be TRUE):

  1. `magelift preview` against a config referencing an existing VPC reports it as an import rather than a create, and the subsequent apply adopts it without replacing it
  2. An existing managed database instance (RDS or Cloud SQL) is adopted into a MageLift stack, and a following `preview` reports no destructive change against it
  3. Any attempt to replace or destroy an adopted resource MageLift does not own fails before mutate with a message naming the resource — asserted by a test, not by convention
  4. Documented adoption limits state what can be attached, what cannot, and a detach path — verified by detaching an adopted resource and confirming it still exists in the account afterwards
  5. Every row in `docs/release-readiness.md` is Closed with named evidence or Deferred with a reason, and every unchecked requirement in `.planning/REQUIREMENTS.md` has a recorded deferral — no row and no requirement left in an undetermined state on tag day

**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8

Phases 4-5 (brownfield onramp, fully offline) and Phases 6-7 (shared Kubernetes plus GCP certification) are independent tracks after Phase 3. Either pair may be pulled ahead of the other; `parallelization: true` in config permits interleaving their plans. Phase 8 requires both tracks to be settled.

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Publishable Baseline & Honest Fallbacks | 10/10 | In Progress|  |
| 2. Tag-Ready Release Surface | 4/4 | In Progress|  |
| 3. Credit-Efficient Acceptance Harness & Evidence Tiering | 5/6 | In Progress|  |
| 4. Brownfield Onramp — PaaS Import & ece-tools Parity | 6/6 | Complete    | 2026-07-29 |
| 5. Data Migration & Cutover | 6/6 | Complete    | 2026-07-29 |
| 6. Shared Kubernetes Day-2 | 6/6 | Complete    | 2026-07-30 |
| 7. GCP Certification | 0/7 | Planned | - |
| 8. Brownfield Attach & Tag Day | 0/TBD | Not started | - |

## Requirement Coverage

56 of 56 v1 requirements mapped, each to exactly one phase.

| Phase | Requirements | Count |
|-------|--------------|-------|
| 1 | QUALITY-01..08, TRUST-01, TRUST-02 | 10 |
| 2 | RELEASE-01, RELEASE-02, RELEASE-03, RELEASE-04, RELEASE-06 | 5 |
| 3 | ACCEPT-01..06, TRUST-03, TRUST-04 | 8 |
| 4 | IMPORT-01..06, ECE-01..04 | 10 |
| 5 | MIGRATE-01..05 | 5 |
| 6 | KUBE-01..07 | 7 |
| 7 | GCP-01..06 | 6 |
| 8 | ATTACH-01..04, RELEASE-05 | 5 |

---
*Roadmap created: 2026-07-27*
