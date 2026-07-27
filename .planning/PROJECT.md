# MageLift

## What This Is

MageLift is a Go CLI that lets any Magento 2 Open Source (or Adobe Commerce) development team deploy, manage, and monitor their store on their own public cloud account — AWS, GCP, OVH, or Scaleway — without renting a PaaS. It uses Pulumi's Go Automation API under the hood and a minimal, clean PHP build system that mimics `ece-tools`, so the YAML and CLI feel familiar to anyone who has used Magento Cloud, Adobe Commerce Cloud, or Platform.sh/Upsun. It manages both ephemeral environments (PR previews, throwaway test envs) and static ones (prod, staging, UAT), and exposes a capability matrix so each team can tune architecture to its own budget and HA needs.

This milestone takes MageLift from "advanced private project" to a public open-source release the community can adopt and contribute to.

## Core Value

**A Magento team with no dedicated devops resource can deploy and operate production Magento on their own cloud account, at a cost they control, using a CLI and YAML that already feel familiar.**

If everything else fails, that must work — for at least one certified provider, honestly documented.

## Business Context

- **Customer**: E-commerce agencies and SMEs running Magento 2 Open Source who are either overpaying for Adobe Commerce Cloud / Platform.sh, or stuck with bare-metal hosts where every deploy or investigation is a support ticket.
- **Revenue model**: None in v1. Open source (Apache-2.0), solo maintainer. Future optional commercial surfaces (hosted state/ESC for agencies, FinOps SaaS) are explicitly deferred and must never weaken the CLI.
- **Success metric**: Time-to-first-successful-deploy for a new team on their own cloud account. Secondary: adoption by agencies who migrate at least one real store off a PaaS.
- **Strategy notes**: `docs/capability-matrix.md` (what may be claimed), `docs/release-readiness.md` (public-tag gate board), `docs/post-beta-roadmap.md` (explicit deferrals).

## Requirements

### Validated

<!-- Inferred from the existing codebase (see .planning/codebase/) — shipped and relied upon. -->

- ✓ Hexagonal architecture with cloud providers as pluggable `platform.StackModule` modules registered at startup; CLI and deploy orchestrator never import concrete cloud packages — existing
- ✓ `sdk/v1` stable contract package (`TargetDescriptor`, `Application`, `BuildArtifact`, topology, validation) shared by core and all adapters, governed by ADR 0002/0004 — existing
- ✓ Typed `magelift.yaml` config model with generated JSON Schema (`cmd/genconfig` → `schema/magelift.schema.json`), presets (`preview`, `standard`, `high-availability`), and compatibility checks that fail before mutate — existing
- ✓ AWS ECS Fargate certified target: Route 53, CloudFront, WAF, ALB, private Fargate, S3 media, Valkey cache, RDS/Aurora MySQL — existing
- ✓ AWS catalog cells: `natMode` (`nat-gateway`, `fck-nat`), `databaseEngine` (`aurora-mysql`, `rds-mysql`), `queueMode` (`db`, `amazon-mq`, `ecs-rabbitmq`, experimental `ecs-artemis`), `searchMode` (`disabled`, `serverless`, `provisioned`), `webRuntime` (`nginx-fpm`, `frankenphp-classic`) — existing
- ✓ Provider-agnostic candidate-deploy orchestrator (`internal/deploy`): Validate → Preview → RegisterCandidate → RunMigrations → CleanupCandidate → UpdateServices → Stabilize → Health → Record — existing
- ✓ Full AWS Fargate day-2 surface: logs, exec, secrets, state (DIY S3 lock/backup/restore), releases journal, health, doctor, status, cost — existing
- ✓ PHP build/lifecycle package replacing `ece-tools` (`build/`, PSR-4 `MageLift\Build\`, PHPUnit 11.5 + PHPStan 2.2 + Psalm 6.16) — existing
- ✓ Multi-stage container build pipeline (`internal/build/{pipeline,plan,runner,kit}`) with BuildKit and a subprocess runner protocol; build once, promote by digest — existing
- ✓ Runtime images published to GHCR with SBOM, SLSA provenance, and keyless Cosign signing: `php-runtime`, `frankenphp-classic`, `php-builder`, `varnish` — existing
- ✓ Local development without cloud credentials: `magelift dev init/up/status/seed` over Docker Compose, MySQL + Valkey defaults, digest-pinned images, local HTTPS via Caddy internal CA — existing
- ✓ Ephemeral environment primitives: `magelift env create` with `--expires-at` (RFC3339) and `env sweep`, `expiresAt` honoured across all four providers — existing
- ✓ Floci-based account-free AWS integration testing (`make floci-test`: bootstrap, locks, secrets, logs, media restore, ECS candidates) — existing
- ✓ Experimental infra provisioning for GCP GKE Autopilot, AWS EKS Autopilot, OVH MKS, Scaleway Kapsule: `preview`/`apply` work, day-2 mostly `ErrNotSupported`, tier-gated via `platform.TierCertified`/`TierExperimental` — existing
- ✓ Release engineering: Release Please + Conventional Commits, GoReleaser multi-platform archives, SHA-256 checksums, SBOMs, Sigstore attestation, Homebrew cask path — existing
- ✓ Community/governance scaffolding: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `GOVERNANCE.md`, `CODEOWNERS`, issue + PR templates, Dependabot, CodeQL, actions-security workflow — existing
- ✓ Documentation site (MkDocs) with 28 docs including `architecture.md`, `configuration.md` (32K), `cli-reference.md` (generated), `capability-matrix.md`, per-provider experimental pages, and 10 ADRs — existing
- ✓ OKF knowledge bundle at `docs/knowledge/` with 249 notes capturing lessons, decisions, and dead ends — existing

### Active

<!-- This milestone: what must be true to tag v1.0.0-rc.1 and then v1.0.0. -->

**Certify a second provider (GCP) so multi-cloud is a truthful claim**

- [ ] GCP GKE Autopilot reaches certified tier: Workload Identity Federation bootstrap, Secret Manager Composer credentials completed (no silent no-ops), full day-2 ops, and a real acceptance pass
- [ ] GCP `CostEstimator` adapter so `magelift cost` works on the second certified provider

**Shared Kubernetes day-2, split by port**

- [ ] `Observe` (TailLogs, CheckRuntime, PrepareExec) implemented once in `internal/cloud/kube` and inherited by GKE, EKS Autopilot, OVH MKS, and Scaleway Kapsule
- [ ] `deploy.Steps` (Magento candidate deploy on Kubernetes) implemented once in `internal/cloud/kube` and inherited by all four Kubernetes targets
- [ ] `Bootstrap`, `State`, `Secrets`, and `CostEstimator` remain per-provider; the S3-compatible `State` manager is reused for OVH and Scaleway Object Storage via endpoint override
- [ ] Replace the `unsupported{}` stub shells in `internal/cloud/ovh/stack/ops.go` and `internal/cloud/scaleway/stack/ops.go` with real implementations where the shared layer provides them; whatever remains unimplemented fails loudly and is surfaced in the capability matrix

**Brownfield: let teams actually migrate onto MageLift**

- [ ] PaaS config import: `magelift init --from-acc` / `--from-upsun` translates `.magento.app.yaml`, `.platform.app.yaml`, `services.yaml`, `routes.yaml`, and cron definitions into `magelift.yaml` — never accepting a foreign schema silently, and respecting the clean-room policy in `docs/knowledge/`
- [ ] Data migration runner: finish ADR 0010's import path so `seedDump` stops being inert — DB dump import into managed MySQL/Aurora after first deploy, media sync to object storage, and a documented cutover runbook (DNS, maintenance mode, reindex)
- [ ] `ece-tools` parity audit: verify the PHP build system covers what real projects depend on — build/deploy/post-deploy hooks, static content deploy, `magento-cloud-patches` application, env-var-driven config — and close or explicitly document every gap
- [ ] Brownfield attach: adopt an existing VPC / RDS / cluster into MageLift state via Pulumi import (supersedes ADR 0010's "attach existing DB remains out of scope")

**Credit-efficient, honest verification**

- [ ] Acceptance harness that makes paid passes cheap: one long-lived stack iterated cell-by-cell, resumable runs, automated evidence capture into `matrix-results.md`, single destroy with `assert_clean`, for both AWS and GCP
- [ ] Shift verification left into Floci and Pulumi mocks wherever a day-2 port allows, so each paid pass buys only what mocks cannot prove
- [ ] Evidence tiering surfaced in the product, not just docs: no cell may claim more than its evidence tier supports, and cells that are unverifiable on the maintainer's accounts say so explicitly
- [ ] Experimental providers warn unmistakably at CLI runtime, not only in documentation

**Close the public-tag gates**

- [ ] Reconcile the version story: `README.md`, `docs/versioning.md`, and the `docs/release-readiness.md` gate board all say `v0.x` / pre-alpha, but the first public tag is now `v1.0.0-rc.1`
- [ ] Freeze `sdk/v1` and the `platform.StackModule` surface as a real compatibility contract, with provider-authoring guidance that a stranger can follow (`docs/adding-a-provider.md`, `examples/custom-cli`)
- [ ] Finish the packaging smoke that was aborted on 2026-07-22 (`make release-smoke` in a plain Terminal, serial only)
- [ ] Close the internal quality debt that would embarrass a public repo: restore `internal/cli/cost.go` test coverage, add the IAM permissions-boundary 6KiB size guard, add AOSS OCU table-driven tests, add cross-catalog-cell (queue × search × webRuntime) combination tests, add subnet-carve boundary tests per provider
- [ ] Fix the golangci-lint CI OOM/timeout root cause (partition lint by provider rather than serializing one 30-minute job)
- [ ] Split `internal/cloud/aws/runtime/runtime.go` (991 lines) by concern before adding further AWS runtime features

### Out of Scope

- **Signed remote plugins / auto-download plugin loader** — no Sigstore allowlist story yet; out-of-tree providers register through a custom binary (`examples/custom-cli`, ADR 0007)
- **In-core storefront frameworks (Next.js, PWA Studio, OpenNext)** — Magento's own storefront *is* fully supported; `application.mode: headless` means MageLift exposes REST and GraphQL APIs and the team deploys its own frontend with its own tooling. SST was cited only as a shape reference, not a dependency.
- **FinOps SaaS** (multi-project cost anomaly detection, rightsizing, drift as a hosted service) — a business model, not CLI code; the CLI keeps full cost power
- **Pulumi Cloud / ESC as a requirement** — optional hosted state and OIDC may be supported later for agencies, but DIY S3/GCS remains the OSS default
- **MageLift as a hosting service** — it deploys into the user's own cloud account; there is no MageLift-operated control plane
- **Adding a fifth cloud provider** — the multi-provider commitment gate (ADR 0007/0008) says no new providers until a second one is certified
- **ECS Managed Instances + 3-node RabbitMQ quorum HA ladder** — *deferred, not excluded*. This is the strongest cost lever (Chantelle proved MI + RI/Savings Plans for web and RabbitMQ density) and should be revisited immediately post-v1. Fargate + single-node broker stays the certified path; never flip 1↔3 nodes in place.

## Context

**Origin.** Built from direct experience deploying Magento 2 Open Source on AWS with Terraform and Docker in a previous role: a greenfield AWS project (`../backend-m2-b2b`) and a brownfield migration toolkit for stores leaving Adobe Commerce Cloud or Platform.sh (`../magento-aws-stack`). Conversations with agencies and SMEs confirmed the two failure modes MageLift targets: overpaying a PaaS, or being hostage to a bare-metal host for every deploy and investigation.

**Design references.** The Magento Cloud / Platform.sh CLI (`../cli`), `ece-tools` (`../ece-tools`), and `magento-cloud-patches` (`../magento-cloud-patches`) define the ergonomics Magento developers already expect — readable, editable YAML and a CLI that does the obvious thing. MageLift deliberately mimics that surface. A clean-room policy governs how these references may inform implementation; it is recorded in `docs/knowledge/`.

**Current state.** The project is well advanced and unusually well documented for its stage. `.planning/codebase/` holds a full map (analysed 2026-07-27). `docs/release-readiness.md` maintains a gate board where trademark clearance, contract freeze, the OpenSearch public-tag substitute, the queue matrix, time-to-preview (~10m31s), `/health` on runtime images, and NOTICE/license review are all **Closed**; GCP Magento deploy Ops and the live OpenSearch SigV4 data plane are **Deferred**; packaging smoke is **Partial**.

**What "community-ready" already means here.** Governance, security policy, code of conduct, CODEOWNERS, issue and PR templates, Dependabot, CodeQL, and an actions-security workflow all exist. Provider-authoring docs and a working out-of-tree extension example exist. This milestone polishes that rather than building it.

**Verification reality.** Acceptance runs are funded personally on a free-tier-limited AWS account and a GCP project. Some AWS cells are unverifiable on that account at any price — Aurora `CreateDBCluster` is blocked, and Amazon MQ's `CLUSTER_MULTI_AZ` needs 3 AZs while the `preview` preset is 2-AZ (incompatible by design). Each pass is also slow (~10m31s create on AWS, ~19m21s on GCP, plus destroy soak). Floci and Pulumi mocks are therefore the default automated verification, and paid passes must be batched and efficient.

**Known fragile ground.** `internal/cloud/aws/runtime/runtime.go` (991 lines) is the single highest-risk file — it wires web runtime, queue mode, the SigV4 OpenSearch sidecar, and Magento env through one construction path, so catalog cells can interact unintentionally. CIDR carving is reimplemented per provider with a history of boundary bugs. `go.yaml.in/yaml/v4` is pinned to a release candidate because upstream froze v3.

## Constraints

- **Team**: Solo maintainer — no parallel human review, so automated gates and honest self-checks carry the quality load
- **Timeline**: No deadline, but the goal is public as soon as the bar is met; sequencing should front-load what unblocks a public tag
- **Budget**: Cloud acceptance is self-funded on both AWS and GCP — acceptance must run only when needed, batch efficiently, and destroy reliably
- **Verifiability**: Some AWS cells are blocked by free-tier API limits regardless of budget — the roadmap must never depend on verifying them
- **Local machine**: Full multi-platform `goreleaser release` cannot run locally (parallel cross-compiles of this Pulumi-linked binary have caused kernel panics); serial single-target only, outside Cursor. Full matrices belong on CI.
- **CI**: golangci-lint already OOMs/times out on cold multi-cloud Pulumi graphs; it is serialized at `concurrency: 1` with a 30-minute timeout. Any new provider code makes this worse.
- **Legal/trademark**: Independent of Adobe. Magento and Adobe Commerce are Adobe trademarks used descriptively only; non-affiliation language must stay accurate everywhere
- **Licensing**: Apache-2.0; `go-licenses` cannot classify `github.com/ovh/pulumi-ovh` nested packages, so its license is recorded manually in `NOTICE` and must be re-verified by hand
- **Architecture**: ADR 0002/0004 — cloud adapter internals must not leak into the CLI or `internal/deploy`; only `sdk/v1` types and `platform` interfaces cross the boundary
- **Provider gate**: ADR 0007/0008 — no fifth provider until a second is certified

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Ship broad scope for v1 rather than a narrow AWS-only launch | Open source tolerates rough edges; the community can iron things out through issues and PRs, and a GCP project is available to certify against | — Pending |
| First public tag is `v1.0.0-rc.1`, RCs until gates close, then `v1.0.0` | Community can use it while the contract is not yet frozen; clear path to stable. Requires reconciling `README.md`, `docs/versioning.md`, and the gate board, which all still say `v0.x` | — Pending |
| Certify GCP GKE Autopilot as the second provider in this milestone | ADR 0007 blocks any multi-cloud claim until two first-party targets are certified, and GCP is the only one with an available account | — Pending |
| Split day-2 by *port nature*, not by provider: k8s-shaped ports shared, cloud-shaped ports per-provider | `Observe` and `deploy.Steps` are identical given a kubeconfig, so one implementation serves GKE/EKS/MKS/Kapsule; `Bootstrap`/`State`/`Secrets`/`Cost` share nothing across clouds. Follows ADR 0008 and the existing `internal/cloud/kube` package rather than adding a new layer. GCP certification then lifts three more targets. | — Pending |
| Reuse the S3-compatible `State` manager for OVH and Scaleway Object Storage | Both are S3-compatible; only GCS is genuinely separate. Avoids three parallel state implementations. | — Pending |
| All four brownfield tracks in v1, including brownfield attach | The ICP is teams *already* on a PaaS or bare metal; a greenfield-only launch would land flat. Supersedes ADR 0010's "attach existing DB remains out of scope". | — Pending |
| Treat acceptance efficiency as an early deliverable, not hygiene | Self-funded credits are the binding constraint; a resumable single-stack harness with automated evidence capture pays for itself across every later phase | — Pending |
| Keep OVH and Scaleway in-tree rather than extracting to examples | The shared k8s day-2 layer makes parity affordable, so the multi-provider story stays visible without lying about it | — Pending |
| ECS Managed Instances and RabbitMQ HA deferred but explicitly not excluded | Strongest cost lever and already proven at Chantelle; revisit immediately post-v1 rather than burying it in out-of-scope | — Pending |
| Signed remote plugins stay out; extensions ship as compiled custom binaries | No Sigstore allowlist story yet; `examples/custom-cli` already proves the boundary works | ✓ Good |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Business Context check — customer, revenue model, success metric still accurate?
4. Audit Out of Scope — reasons still valid?
5. Update Context with current state

---
*Last updated: 2026-07-27 after initialization*
