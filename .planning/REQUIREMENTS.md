# Requirements: MageLift v1.0.0

**Defined:** 2026-07-27
**Core Value:** A Magento team with no dedicated devops resource can deploy and operate production Magento on their own cloud account, at a cost they control, using a CLI and YAML that already feel familiar.

**Milestone goal:** Take MageLift from advanced private project to a public open-source release — first tag `v1.0.0-rc.1`, RCs until gates close, then `v1.0.0`.

> "Operator" below means the person running the CLI (agency developer, SME dev, platform engineer). "Maintainer" means the project maintainer running acceptance passes against paid accounts.

## v1 Requirements

### Verification & Acceptance Efficiency

<!-- Self-funded cloud credits are the binding constraint on this milestone. This category comes first because it reduces the cost of every category after it. -->

- [x] **ACCEPT-01**: Maintainer can run an acceptance pass that creates one long-lived stack and iterates catalog cells on it without recreating infrastructure between cells
- [x] **ACCEPT-02**: Maintainer can resume an interrupted acceptance run from the last completed cell instead of restarting the matrix
- [x] **ACCEPT-03**: Acceptance runs record evidence automatically (cell, result, duration, provider, account, date) into a matrix results file, with no hand transcription
- [x] **ACCEPT-04**: Acceptance runs destroy every created resource on exit and assert the account is clean, failing loudly if anything remains
- [ ] **ACCEPT-05**: Maintainer can run the same acceptance harness shape against GCP as against AWS, including GCP's PSA soak and force-clean teardown path
- [x] **ACCEPT-06**: Every day-2 port that can be exercised without a paid account is covered by Floci or Pulumi mocks, so a paid pass only buys what mocks cannot prove

### Honest Capability Claims

<!-- Some cells are unverifiable on the maintainer's accounts at any price. The product must never claim more than its evidence supports. -->

- [x] **TRUST-01**: Operator selecting an experimental provider or runtime sees an unmistakable warning from the CLI itself before any mutating command, not only in documentation
- [x] **TRUST-02**: Operator invoking an unimplemented day-2 command on an experimental target gets an actionable error naming the capability and its certification tier — never a silent no-op or a false success
- [x] **TRUST-03**: Capability matrix records an evidence tier per cell (Pulumi mocks / Floci / real-account acceptance) and no cell claims a tier above its recorded evidence
- [x] **TRUST-04**: Cells that cannot be verified on the maintainer's accounts are marked unverifiable with the specific reason (free-tier API block, AZ requirement, cost), so a reader can distinguish "untested" from "broken"

### GCP Certification

<!-- ADR 0007 blocks any multi-cloud claim until a second first-party target is certified. -->

- [ ] **GCP-01**: Operator can bootstrap GCP with Workload Identity Federation so CI authenticates without long-lived service-account keys
- [ ] **GCP-02**: Operator can store and retrieve Composer credentials from GCP Secret Manager during build and deploy, with no unimplemented path returning success
- [ ] **GCP-03**: Operator can run the full day-2 command set (logs, exec, secrets, state, health) against a GKE Autopilot target
- [ ] **GCP-04**: Operator can deploy Magento to GKE Autopilot through the standard candidate-deploy sequence (migrate → cutover → health → record)
- [ ] **GCP-05**: Operator can run `magelift cost` against a GCP target and get a per-cell estimate
- [ ] **GCP-06**: GCP GKE Autopilot is recorded as certified tier, backed by a real-account acceptance pass, making the multi-cloud claim truthful

### Shared Kubernetes Day-2

<!-- Split by port nature: k8s-shaped ports shared once, cloud-shaped ports per-provider (ADR 0008). -->

- [ ] **KUBE-01**: Operator can tail Magento logs on any Kubernetes target (GKE, EKS Autopilot, OVH MKS, Scaleway Kapsule) through a single shared `Observe.TailLogs` implementation
- [ ] **KUBE-02**: Operator can check Magento runtime health on any Kubernetes target through a single shared `Observe.CheckRuntime` implementation
- [ ] **KUBE-03**: Operator can `magelift exec` into a Magento pod on any Kubernetes target through a single shared `Observe.PrepareExec` implementation
- [ ] **KUBE-04**: Operator can deploy Magento on any Kubernetes target through a single shared `deploy.Steps` implementation in `internal/cloud/kube`
- [ ] **KUBE-05**: Operator gets state lock, backup, and restore on OVH and Scaleway by reusing the S3-compatible state manager with an endpoint override, rather than three separate implementations
- [ ] **KUBE-06**: `Bootstrap` and `Secrets` stay per-provider, and every path not yet implemented fails loudly with its tier rather than returning nil success
- [ ] **KUBE-07**: The `unsupported{}` stub shells in `internal/cloud/ovh/stack/ops.go` and `internal/cloud/scaleway/stack/ops.go` are replaced by real implementations where the shared layer provides them, with remaining gaps tier-gated and matrix-recorded

### PaaS Config Import

<!-- The strongest ICP demo, and entirely offline/unit-testable with zero cloud spend. -->

- [ ] **IMPORT-01**: Operator can run `magelift init --from-acc` inside an Adobe Commerce Cloud repository and get a valid `magelift.yaml`
- [ ] **IMPORT-02**: Operator can run `magelift init --from-upsun` inside a Platform.sh or Upsun repository and get a valid `magelift.yaml`
- [ ] **IMPORT-03**: Import translates application config, services, routes, and cron definitions, and reports every source key it could not map instead of dropping it silently
- [ ] **IMPORT-04**: The generated `magelift.yaml` validates against `schema/magelift.schema.json` and passes `magelift config validate` with no hand editing for supported source shapes
- [ ] **IMPORT-05**: Import output is a reviewable file the operator can diff and edit; a foreign schema is never accepted directly as deploy input
- [ ] **IMPORT-06**: Importer implementation observes the clean-room policy recorded in `docs/knowledge/`

### Data Migration

<!-- ADR 0010 built the seam; the import runner is the missing piece. Serves both PaaS and bare-metal origins. -->

- [ ] **MIGRATE-01**: Operator can create an environment with `--dump` and have the dump actually imported into the managed MySQL/Aurora instance after the first successful deploy — `seedDump` is no longer inert
- [ ] **MIGRATE-02**: Operator sees an accurate `seedDumpStatus` progressing through recorded → importing → imported → failed, with the failure reason available
- [ ] **MIGRATE-03**: Operator can sync Magento media from a source location into the target's object storage
- [ ] **MIGRATE-04**: Operator can follow a documented cutover runbook (DNS, maintenance mode, reindex, verification, rollback) to move a live store onto MageLift
- [ ] **MIGRATE-05**: Dump import is safe to retry and refuses to overwrite a non-empty production database without explicit confirmation

### ece-tools Parity

<!-- Gaps here silently break migrated projects, which is the worst possible first impression for the ICP. -->

- [ ] **ECE-01**: A documented parity matrix compares MageLift's PHP build system against `ece-tools` build, deploy, and post-deploy hook behaviour, with every gap either closed or explicitly recorded as intentional
- [ ] **ECE-02**: Operator can apply `magento-cloud-patches`-style patches through MageLift's build system
- [ ] **ECE-03**: Static content deploy honours the settings real projects depend on (locales, themes, strategy, thread count) for supported configurations
- [ ] **ECE-04**: Environment-variable-driven Magento configuration in the ACC/Upsun shape is either honoured or explicitly mapped to its `magelift.yaml` equivalent, with the mapping documented

### Brownfield Attach

<!-- Supersedes ADR 0010's "attach existing DB remains out of scope". -->

- [ ] **ATTACH-01**: Operator can adopt an existing VPC into a MageLift stack instead of having one created
- [ ] **ATTACH-02**: Operator can adopt an existing managed database instance (RDS/Cloud SQL) into a MageLift stack
- [ ] **ATTACH-03**: Adoption runs through `preview` first and shows exactly what will be imported versus created, and refuses to mutate or destroy adopted resources it does not own
- [ ] **ATTACH-04**: Documented limits of adoption — what can be attached, what cannot, and how to detach without losing the resource

### Internal Quality

<!-- The debt that would embarrass a public repository, plus the guards that stop known bugs recurring. -->

- [x] **QUALITY-01**: `internal/cli/cost.go` has test coverage restored — flag parsing, error paths, and `ErrNotSupported` handling for non-AWS targets
- [x] **QUALITY-02**: A test asserts the rendered IAM permissions-boundary policy stays under AWS's 6KiB limit, so future policy growth fails CI instead of failing silently in AWS
- [x] **QUALITY-03**: Table-driven tests enumerate the AOSS collection-group OCU values AWS accepts, preventing silent drift
- [x] **QUALITY-04**: Combination tests cover `queueMode` × `searchMode` × `webRuntime` together, not just each catalog cell in isolation
- [x] **QUALITY-05**: Every provider's subnet/CIDR carving has explicit boundary tests (min index, max index, max index + 1)
- [x] **QUALITY-06**: `golangci-lint` completes in CI without OOM or timeout by partitioning work per provider, rather than by serializing one 30-minute job
- [x] **QUALITY-07**: `internal/cloud/aws/runtime/runtime.go` is split by concern (task definition, service, sidecar injection) so no single 991-line file owns all AWS runtime wiring
- [x] **QUALITY-08**: The two recently fixed OVH bugs — MKS node-pool dependency ordering and subnet index cap validation — have explicit regression tests

### Public Release

- [x] **RELEASE-01**: The version story is consistent across `README.md`, `docs/versioning.md`, and the release-readiness gate board: the first public tag is `v1.0.0-rc.1`, and nothing still describes the project as pre-alpha `v0.x`
- [x] **RELEASE-02**: `sdk/v1` and `platform.StackModule` are documented as a compatibility contract with an explicit stability statement covering what may change during the RC series
- [x] **RELEASE-03**: A third party can follow `docs/adding-a-provider.md` plus `examples/custom-cli` to register an out-of-tree provider without reading core source
- [x] **RELEASE-04**: `make release-smoke` completes on the maintainer's machine (serial, single-target, outside Cursor), closing the Partial packaging gate
- [ ] **RELEASE-05**: Every release-readiness gate board row is Closed or explicitly Deferred with a reason, and the board reflects reality on tag day
- [x] **RELEASE-06**: A new contributor can go from `git clone` to a green `make verify` following `CONTRIBUTING.md` alone

## v2 Requirements

Acknowledged and deferred. Not in this roadmap.

### Cost Optimization

- **MI-01**: ECS Managed Instances as an optional capacity provider beside Fargate (`launchType: fargate|managed-instances`), gated behind a certified acceptance cell
- **MI-02**: RabbitMQ HA ladder — EFS + AMQPS single-node → 3-node quorum on Managed Instances, with a designed migration path (never a 1↔3 in-place flip)
- **COST-01**: `CostEstimator` adapters for OVH, Scaleway, and EKS
- **COST-02**: Managed Instances launch-type pricing cells

### Verification

- **SEARCH-01**: Live OpenSearch SigV4 data-plane acceptance on a real MageLift stack — index creation, storefront queries, reconnect after task recycle, least-privilege task role (requires paid credits)

### Operations

- **STATE-01**: Optional Pulumi Cloud / ESC hosted state and OIDC for agencies; DIY S3/GCS remains the OSS default
- **ORG-01**: Move the repository from a personal namespace to a `magelift` GitHub organization
- **STORE-01**: Documentation-only storefront recipes — wiring Next.js or PWA Studio to Magento outputs

## Out of Scope

| Feature | Reason |
|---------|--------|
| Signed remote plugins / auto-download plugin loader | No Sigstore allowlist story yet; out-of-tree providers register through a custom binary (`examples/custom-cli`, ADR 0007) |
| In-core storefront frameworks (Next.js, PWA Studio, OpenNext) | Magento's own storefront is fully supported. `application.mode: headless` means MageLift exposes REST and GraphQL; the team deploys its own frontend with its own tooling |
| FinOps SaaS (multi-project cost anomaly, rightsizing, drift) | A business model, not CLI code. The CLI keeps full cost power |
| MageLift as a hosting service | Deploys into the user's own cloud account; there is no MageLift-operated control plane |
| A fifth cloud provider | ADR 0007/0008 multi-provider gate — no new providers until a second one is certified |
| Amazon MQ on the `preview` preset | Incompatible by design: `CLUSTER_MULTI_AZ` needs 3 AZs, `preview` is 2-AZ. Guard stays, combination is never supported |

## Traceability

Populated during roadmap creation (2026-07-27). Every v1 requirement maps to exactly one phase.

| Requirement | Phase | Status |
|-------------|-------|--------|
| ACCEPT-01 | Phase 3 | Complete |
| ACCEPT-02 | Phase 3 | Complete |
| ACCEPT-03 | Phase 3 | Complete |
| ACCEPT-04 | Phase 3 | Complete |
| ACCEPT-05 | Phase 3 | Pending |
| ACCEPT-06 | Phase 3 | Complete |
| TRUST-01 | Phase 1 | Complete |
| TRUST-02 | Phase 1 | Complete |
| TRUST-03 | Phase 3 | Complete |
| TRUST-04 | Phase 3 | Complete |
| GCP-01 | Phase 7 | Pending |
| GCP-02 | Phase 7 | Pending |
| GCP-03 | Phase 7 | Pending |
| GCP-04 | Phase 7 | Pending |
| GCP-05 | Phase 7 | Pending |
| GCP-06 | Phase 7 | Pending |
| KUBE-01 | Phase 6 | Pending |
| KUBE-02 | Phase 6 | Pending |
| KUBE-03 | Phase 6 | Pending |
| KUBE-04 | Phase 6 | Pending |
| KUBE-05 | Phase 6 | Pending |
| KUBE-06 | Phase 6 | Pending |
| KUBE-07 | Phase 6 | Pending |
| IMPORT-01 | Phase 4 | Pending |
| IMPORT-02 | Phase 4 | Pending |
| IMPORT-03 | Phase 4 | Pending |
| IMPORT-04 | Phase 4 | Pending |
| IMPORT-05 | Phase 4 | Pending |
| IMPORT-06 | Phase 4 | Pending |
| MIGRATE-01 | Phase 5 | Pending |
| MIGRATE-02 | Phase 5 | Pending |
| MIGRATE-03 | Phase 5 | Pending |
| MIGRATE-04 | Phase 5 | Pending |
| MIGRATE-05 | Phase 5 | Pending |
| ECE-01 | Phase 4 | Pending |
| ECE-02 | Phase 4 | Pending |
| ECE-03 | Phase 4 | Pending |
| ECE-04 | Phase 4 | Pending |
| ATTACH-01 | Phase 8 | Pending |
| ATTACH-02 | Phase 8 | Pending |
| ATTACH-03 | Phase 8 | Pending |
| ATTACH-04 | Phase 8 | Pending |
| QUALITY-01 | Phase 1 | Complete |
| QUALITY-02 | Phase 1 | Complete |
| QUALITY-03 | Phase 1 | Complete |
| QUALITY-04 | Phase 1 | Complete |
| QUALITY-05 | Phase 1 | Complete |
| QUALITY-06 | Phase 1 | Complete |
| QUALITY-07 | Phase 1 | Complete |
| QUALITY-08 | Phase 1 | Complete |
| RELEASE-01 | Phase 2 | Complete |
| RELEASE-02 | Phase 2 | Complete |
| RELEASE-03 | Phase 2 | Complete |
| RELEASE-04 | Phase 2 | Complete |
| RELEASE-05 | Phase 8 | Pending |
| RELEASE-06 | Phase 2 | Complete |

**Coverage:**

- v1 requirements: 56 total
- Mapped to phases: 56
- Unmapped: 0 ✓

**By phase:**

| Phase | Name | Requirements | Cloud spend |
|-------|------|--------------|-------------|
| 1 | Publishable Baseline & Honest Fallbacks | 10 | None |
| 2 | Tag-Ready Release Surface | 5 | None |
| 3 | Credit-Efficient Acceptance Harness & Evidence Tiering | 8 | PAID (AWS free-tier, pass 1 of 3) |
| 4 | Brownfield Onramp — PaaS Import & ece-tools Parity | 10 | None |
| 5 | Data Migration & Cutover | 5 | Mostly none (one cell rides Phase 7) |
| 6 | Shared Kubernetes Day-2 | 7 | None |
| 7 | GCP Certification | 6 | PAID (GCP, pass 2 of 3) |
| 8 | Brownfield Attach & Tag Day | 5 | PAID small (AWS free-tier, pass 3 of 3) |

---
*Requirements defined: 2026-07-27*
*Last updated: 2026-07-27 after roadmap creation — traceability populated*
