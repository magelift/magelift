---
status: done
slug: provider-taxonomy-docs
intent: intent.md
---

# Spec: provider taxonomy and docs

## Requirements

### Requirement: Provider-root rule documented in three places

The tree SHALL state one provider-root rule, with identical meaning in `docs/adding-a-provider.md`, `docs/adr/0003-portable-contracts-vs-topology.md`, and the `AGENTS.md` Map section.

#### Scenario: Rule text present in all three docs

- **WHEN** the change is applied
- **THEN** `grep -F "internal/cloud/<provider>" docs/adding-a-provider.md` exits 0
- **AND** `grep -F "internal/external/" docs/adding-a-provider.md` exits 0
- **AND** `grep -F "internal/cloud/<provider>" docs/adr/0003-portable-contracts-vs-topology.md` exits 0
- **AND** `grep -F "internal/external/" docs/adr/0003-portable-contracts-vs-topology.md` exits 0
- **AND** `grep -F "internal/cloud/<p>" AGENTS.md` exits 0
- **AND** `grep -F "internal/external/" AGENTS.md` exits 0.

#### Scenario: Docs build stays strict-green

- **WHEN** the docs edits are applied
- **THEN** `make docs` exits 0.

### Requirement: Shared recovery ports moved out of `internal/cloud/`

The change SHALL move `internal/cloud/recovery`, `internal/cloud/resilience`, and `internal/cloud/statearchive` to the decided shared root with package APIs unchanged (import-path-only rename plus doc comments that name the new path).

#### Scenario: Old import paths gone from Go sources

- **WHEN** the rename is applied
- **THEN** `grep -r "internal/cloud/recovery" --include='*.go' .` returns empty
- **AND** `grep -r "internal/cloud/resilience" --include='*.go' .` returns empty
- **AND** `grep -r "internal/cloud/statearchive" --include='*.go' .` returns empty.

#### Scenario: New shared root exists and builds

- **WHEN** the rename is applied
- **THEN** `ls internal/shared/recovery internal/shared/resilience internal/shared/statearchive` exits 0
- **AND** `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/shared/recovery/ ./internal/shared/resilience/ ./internal/shared/statearchive/ -count=1` exits 0.

### Requirement: SaaS adapter homes labeled

`docs/adding-a-provider.md` SHALL state the SaaS homes: Fastly under `internal/external/fastly`, New Relic under `internal/external/newrelic`, edge/observability composition under `internal/external/edge` and `internal/external/observability`, and the Magento-safe WAF contract under `internal/edge/waf`.

#### Scenario: SaaS homes greppable in the provider guide

- **WHEN** the docs edits are applied
- **THEN** `grep -F "internal/external/fastly" docs/adding-a-provider.md` exits 0
- **AND** `grep -F "internal/external/newrelic" docs/adding-a-provider.md` exits 0
- **AND** `grep -F "internal/external/observability" docs/adding-a-provider.md` exits 0
- **AND** `grep -F "internal/edge/waf" docs/adding-a-provider.md` exits 0.

### Requirement: Adapter-less email and DNS explicitly marked

The provider guide SHALL explicitly mark SES as config-only (adapter-less) and Cloudflare as shell-only DNS (adapter-less), each with a one-line reason, and SHALL NOT create a Go adapter for either.

#### Scenario: Adapter-less labels with reasons present

- **WHEN** the docs edits are applied
- **THEN** `grep -Fw "SES" docs/adding-a-provider.md | grep -i "adapter-less"` exits 0
- **AND** `grep -F "Cloudflare" docs/adding-a-provider.md | grep -i "adapter-less"` exits 0
- **AND** `grep -F "email.mode" docs/adding-a-provider.md` exits 0
- **AND** `grep -F "scripts/acceptance/lib-cloudflare-dns.sh" docs/adding-a-provider.md` exits 0.

#### Scenario: No Go adapter exists for email vendors or Cloudflare

- **WHEN** the change is applied
- **THEN** `ls internal/external/ses internal/external/cloudflare internal/cloudflare 2>&1` reports no such directories
- **AND** `grep -rl "package cloudflare" --include='*.go' internal/ cmd/` returns empty.

### Requirement: Kube shared helper documented as the single exception

ADR 0003 SHALL document `internal/cloud/kube` as the single blessed shared Pulumi helper (Magento-shaped K8s wiring for GKE/MKS/Kapsule, no topology switch), frozen in scope, with no new shared Pulumi helpers without an ADR.

#### Scenario: Carve-out wording present in ADR 0003

- **WHEN** the docs edits are applied
- **THEN** `grep -F "internal/cloud/kube" docs/adr/0003-portable-contracts-vs-topology.md` exits 0
- **AND** `grep -Fi "carve-out" docs/adr/0003-portable-contracts-vs-topology.md` exits 0
- **AND** `grep -F "pulumi-kubernetes" docs/adr/0003-portable-contracts-vs-topology.md` exits 0.

### Requirement: Slim per-cloud mains deleted

The change SHALL delete `cmd/magelift-aws`, `cmd/magelift-gcp`, `cmd/magelift-ovh`, and `cmd/magelift-scaleway`, and SHALL leave no dangling references in scripts, docs, Makefile, CI, or Go sources.

#### Scenario: Slim mains gone from the tree

- **WHEN** the deletion is applied
- **THEN** `ls cmd/ | grep "magelift-aws"` returns empty
- **AND** `ls cmd/ | grep "magelift-gcp"` returns empty
- **AND** `ls cmd/ | grep "magelift-ovh"` returns empty
- **AND** `ls cmd/ | grep "magelift-scaleway"` returns empty.

#### Scenario: No dangling slim-main references

- **WHEN** the deletion plus reference updates are applied
- **THEN** `grep -r "cmd/magelift-gcp" --include='*.sh' --include='*.md' --include='Makefile' --include='*.yml' --include='*.go' .` returns empty
- **AND** `grep -r "cmd/magelift-aws" --include='*.sh' --include='*.md' --include='*.yml' --include='*.go' .` returns empty
- **AND** `grep -r "cmd/magelift-ovh" --include='*.sh' --include='*.md' --include='*.yml' --include='*.go' .` returns empty
- **AND** `grep -r "cmd/magelift-scaleway" --include='*.sh' --include='*.md' --include='*.yml' --include='*.go' .` returns empty.

#### Scenario: Acceptance script builds the shipped CLI

- **WHEN** the script update is applied
- **THEN** `grep -F "./cmd/magelift" scripts/gcp-acceptance-local.sh` exits 0
- **AND** `grep -F "cmd/magelift-gcp" docs/gcp-acceptance.md` returns empty.

### Requirement: No runtime behavior change

The change SHALL NOT alter runtime behavior; it is an import-path rename, file deletions, and docs-only edits, with generated artifacts refreshed via `make generate` only.

#### Scenario: Focused unit suites stay green

- **WHEN** the change is applied
- **THEN** `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/shared/recovery/ ./internal/shared/resilience/ ./internal/shared/statearchive/ -count=1` exits 0
- **AND** `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/config/ -count=1` exits 0.

#### Scenario: Generated drift gates stay green

- **WHEN** the change is applied
- **THEN** `make generate-check` exits 0
- **AND** `go run ./cmd/gendocs --check` exits 0.

### Requirement: No certification tier change

The change SHALL NOT change any certified/experimental/unavailable cell in `docs/capability-matrix.md` or `docs/evidence/README.md`; folder moves and wording clarifications recertify nothing.

#### Scenario: Certified rows unchanged

- **WHEN** the change is applied
- **THEN** `grep -F "certified for the evidenced preview tuple" docs/capability-matrix.md` exits 0
- **AND** `grep -F "certified for evidenced runtime cells" docs/capability-matrix.md` exits 0
- **AND** `git diff -- docs/capability-matrix.md | grep -E "^[-+].*\bcertified\b"` returns empty unless the matching `+`/`-` pair is a context-neutral reword with identical tier meaning recorded in the PR description.

## Design

### Exact rule wording (to land verbatim, humanizer-polished, in `docs/adding-a-provider.md` § Provider roots and ADR 0003 § Decision)

> | Root | Holds | Examples |
> | --- | --- | --- |
> | `internal/cloud/<provider>/` | IaaS topology only: one cloud's network, database, cache, search, queue, runtime, native edge/observability, and stack. Each cloud owns its Pulumi graph; no `if provider ==` switches. | `internal/cloud/aws`, `internal/cloud/gcp`, `internal/cloud/ovh`, `internal/cloud/scaleway` |
> | `internal/external/<vendor>/` | SaaS edge/observability adapters behind typed SDK intents. | `internal/external/fastly`, `internal/external/newrelic`, `internal/external/edge` (composition), `internal/external/observability` (composition) |
> | `internal/shared/<port>/` | Provider-neutral durable engines and ports. Stdlib plus `sdk/v1` plus `internal/provider` only; no cloud SDK, no Pulumi. | `internal/shared/recovery`, `internal/shared/resilience`, `internal/shared/statearchive` |
> | `internal/edge/waf/` | Provider-neutral Magento-safe WAF contract every edge adapter translates. | `internal/edge/waf` (`waf/magento-safe`) |
> | Adapter-less (not a provider root) | Config strings or shell helpers with no Go adapter package. | `email.mode: ses` in `internal/config` (SMTP settings plus secret references; validation only, delivery uncertified); Cloudflare DNS shell helpers (`scripts/acceptance/lib-cloudflare-dns.sh`, `scripts/cutover-dns-cloudflare.sh`, `tests/acceptance/cloudflare_dns_helper_test.sh`; DNS cutover only, no CDN/WAF claim) |
>
> Single exception: `internal/cloud/kube` stays where it is (see ADR 0003 carve-out). Nothing else shared lives under `internal/cloud/`.

AGENTS.md Map gains three rows (same meaning, terse):

> | `internal/cloud/<p>/` | IaaS adapter + Pulumi (aws/gcp/ovh/scaleway only) |
> | `internal/external/<v>/` | SaaS edge/observability adapters (fastly/newrelic/edge/observability) |
> | `internal/shared/<port>/` | Provider-neutral ports (recovery/resilience/statearchive; no SDKs) |

### Rename destination: `internal/shared/` (proposed, default)

Move:

- `internal/cloud/recovery` → `internal/shared/recovery`
- `internal/cloud/resilience` → `internal/shared/resilience`
- `internal/cloud/statearchive` → `internal/shared/statearchive`

Rationale:

- Verified provider-neutral: `s3_object_store.go` imports only `context/errors/fmt/strings/time` and defines the `S3ObjectAPI` interface with the comment "Provider packages translate their official SDK request and response models to this port"; `object_archive.go`/`object_engine.go` import stdlib only; `operation.go`/`projection.go` add only `sdk/v1`; `resilience/*.go` (non-test) import stdlib plus `sdk/v1` plus `internal/provider` only; `statearchive/archive.go` imports stdlib only. No `pulumi`, `aws-sdk-go`, `cloud.google.com/go`, OVH, or Scaleway imports in any non-test file.
- `internal/platform/` rejected: it is the CLI-facing port surface (`ModuleRegistry`, `EnvBinding`, `RequiredOutputKeys`, flat `*.go` files, no subpackages today). Nesting durable archive engines under it widens `platform`'s meaning, invites import cycles (providers already import both `platform` and the recovery ports), and confuses the "cross-provider ports" Map row. `internal/provider/` rejected: it is the small lifecycle-client seam (`ClientRequest`, credentials, factory), not an engine home.
- `internal/shared/` is a new, grep-able root with one rule ("no cloud SDK, no Pulumi"), keeps the validated keep-`cloud`+`external` decision intact, and matches the intent's stated option.

### Kube carve-out wording for ADR 0003 (to land verbatim, humanizer-polished)

> Carve-out: `internal/cloud/kube` is the single shared Pulumi helper. It holds Magento-shaped Kubernetes wiring (`SkipAwaitAnnotations`, `ToStringArray`, `EnvVars`, `BuildStaticTokenKubeconfig`, shared Observe/Steps/projection) consumed by the GKE, MKS, and Kapsule runtime adapters, pinned to one `pulumi-kubernetes` SDK version. It owns no network, database, cache, search, or queue topology and contains no `if provider ==` switch. Scope is frozen: no new shared Pulumi helpers without an ADR amending this section.

Assessment: bless-with-freeze is correct. `internal/cloud/kube/kube.go` header already claims "Magento-shaped Kubernetes helpers shared by the GKE, MKS, and Kapsule runtime adapters (ADR 0008). Cloud topology stays per-provider; only the provider-agnostic Magento wiring lives here." Constraining it out (duplicating K8s wiring per provider, or banning the import) is a behavior-risk refactor far beyond a taxonomy intent, and `docs/versioning.md:47-54` explicitly reserves shared-kube consolidation during RC. A stricter no-Pulumi rule is out of scope.

### Slim-main deletion scope

Delete (unreleased, same `go.mod`, ship nowhere):

- `cmd/magelift-aws/main.go` (registers `awsops.Module` only)
- `cmd/magelift-gcp/main.go` (registers `gcpops.Module` Autopilot + Standard)
- `cmd/magelift-ovh/main.go` (registers `ovhstack.Module` only)
- `cmd/magelift-scaleway/main.go` (registers `scwstack.Module` only)

Verified absence from `.goreleaser.yaml`: only `magelift` (`./cmd/magelift`) and `magelift-provider-gcp` (`./cmd/magelift-provider-gcp`) builds exist. Verified no Go importers (all four are `package main`; repo-wide grep for `magelift-aws|magelift-gcp|magelift-ovh|magelift-scaleway` hits only tmp-file names in acceptance scripts plus the two live references below). Verified no Makefile or `.github/workflows` references.

Update the two live references:

1. `scripts/gcp-acceptance-local.sh:699` — `(cd "$ROOT" && ... go build ... -o "$BIN" ./cmd/magelift-gcp)` → build `./cmd/magelift`. Keep the serial `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB` flags and the `MAGELIFT_GCP_ACCEPTANCE_BIN` resume override. Accept the heavier link (full CLI links all adapters) as the cost of the validated delete decision; the script already documents `MAGELIFT_GCP_ACCEPTANCE_BIN` for resume without rebuild.
2. `docs/gcp-acceptance.md:326-328` — "The live script builds `cmd/magelift-gcp`, which links only the GCP adapter and keeps local certification memory bounded." → "The live script builds `cmd/magelift` (the shipped CLI). … Set `MAGELIFT_GCP_ACCEPTANCE_BIN` to an already built binary to avoid rebuilding during a resume." Drop the memory-bounded slim-link claim.

### Docs update list (minimal; ADR + human page together)

1. `docs/adding-a-provider.md` — add § Provider roots with the rule table above; extend the opening paragraph ("VPC, DB, and runtime stay under `internal/cloud/<provider>/`") to name `internal/external/`, `internal/shared/`, `internal/edge/waf`, and the two adapter-less cases; add the kube-exception pointer to ADR 0003; add the SaaS-homes and adapter-less subsections under the existing step 6 (Fastly/New Relic) so the edge/observability boundary and the taxonomy rule agree.
2. `docs/adr/0003-portable-contracts-vs-topology.md` — extend § Decision with the `internal/external/` + `internal/shared/` + adapter-less rule and the kube carve-out paragraph; extend § Consequences ("moving `recovery`/`resilience`/`statearchive` out of `internal/cloud/` recertifies nothing").
3. `AGENTS.md` Map — replace the single `internal/cloud/<p>/` row meaning with the three-row version above; leave the Never rule ("Topology stays in `internal/cloud/<provider>/`") intact and let it point at ADR 0003.
4. `docs/gcp-acceptance.md:326-328` — slim-main wording swap (above).
5. `docs/capability-matrix.md` Cross-cutting edge and observability table — add home/adapter-less annotations only (e.g. "Fastly (`internal/external/fastly`)", "Cloudflare DNS cutover only — shell helpers, no Go adapter", "SES — `email.mode` validation only, no adapter"). No tier-word changes.
6. Explicitly NOT touched: `docs/versioning.md:47-54` (kube path unchanged, reservation still accurate), `docs/evidence/README.md` (no tier change), `docs/architecture.md:40-41` (generic rule already correct; optional one-line pointer to the provider-roots table, only if the humanizer pass finds it ambiguous), `website/` (verified: `grep -r "internal/cloud" website/` returns empty).

### Skill updates

- `contrib/skills/magelift-provider/SKILL.md` Boundary § — mirror the rule: keep "VPC, DB, runtime, and Pulumi components stay under `internal/cloud/<provider>/`", add "SaaS adapters go under `internal/external/<vendor>/`; provider-neutral ports go under `internal/shared/<port>/`; `internal/cloud/kube` is the single ADR-blessed shared K8s helper; SES and Cloudflare are adapter-less by decision". Update the "Leave behind" line to the same effect. No checklist-step renumbering.
- `contrib/skills/magelift-contribute` — verified: contains zero `internal/cloud|internal/external|internal/edge` references, so no change.
- Shipped user skills under `agents/skills/` — no change (taxonomy is contributor-facing; no YAML contract changes).

## Gotchas / policy flags

- No recertification by move. AWS ECS Fargate and GCP GKE Autopilot stay the only certified cells per `docs/capability-matrix.md` plus `docs/evidence/README.md`; EKS, GKE Standard, OVH MKS, and Scaleway Kapsule stay experimental (`eu-providers-experimental` scope respected). The PR description must state "no tier change" and the verify scenario greps the tier words.
- Humanizer + remove-ai-marks required. `docs/adding-a-provider.md`, ADR 0003, the capability-matrix annotations, `docs/gcp-acceptance.md`, and the AGENTS.md Map rows are human pages: run the `humanizer` skill then `remove-ai-marks` on each touched page per repo rules before claiming done.
- Import-path churn risk: measured blast radius is 80 Go files matching `cloud/recovery|cloud/resilience|cloud/statearchive|cloud/kube` (the three-path rename touches the large majority; kube stays). The implementer SHALL perform the rename with `git mv` plus `goimports -w` (or `gofmt -r` for the import rewrite), then run `make generate` (not hand-edits) for any drift in `schema/`, CLI reference, certification coverage, or `agents/manifest.json`, then `make generate-check`, `go run ./cmd/gendocs --check`, and the serial test form `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/<pkg>/ -count=1` per package — never unbounded `go test -race ./...` or parallel heavy builds from an IDE session (`magelift-serial-builds`).
- Website copy out of scope. Verified `grep -r "internal/cloud" website/` returns empty, so no `website/` page names a moved path; if the implementer's re-grep finds otherwise, that page enters scope with humanizer + remove-ai-marks, otherwise it stays untouched.

## Open questions carried forward

- Target root — DECIDED (validated decision 1): keep `internal/cloud/` for IaaS plus `internal/external/` for SaaS with the explicit documented rule above; no `internal/providers/` migration. Owner: intent acceptor. Spec records the rule verbatim.
- Shared-ports destination — DEFAULT: `internal/shared/{recovery,resilience,statearchive}` per the rationale above; fallback `internal/platform/{recovery,resilience,statearchive}` rejected (widens CLI-port surface, cycle risk, flat-package mismatch). Owner: implementer to confirm during plan; if the plan review prefers `internal/platform/`, the spec's grep scenarios swap `internal/shared/` → `internal/platform/` one-for-one.
- Slim mains — DECIDED (validated decision 2): delete all four `cmd/magelift-{aws,gcp,ovh,scaleway}` mains; update `scripts/gcp-acceptance-local.sh` to build `./cmd/magelift` and reword `docs/gcp-acceptance.md:326-328`. No GoReleaser slim-CLI wiring. Owner: intent acceptor.
- Kube carve-out — DEFAULT: bless `internal/cloud/kube` in place with the frozen-scope ADR carve-out above; do not move, split, or further constrain it in this change. A stricter no-shared-Pulumi rule is a separate intent. Owner: implementer to confirm during plan; default stands if unchallenged.
