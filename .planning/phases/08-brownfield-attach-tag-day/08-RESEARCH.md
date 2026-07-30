# Phase 8: Brownfield Attach & Tag Day - Research

**Researched:** 2026-07-30
**Domain:** AWS brownfield adopt (VPC + managed DB), Pulumi resource ownership honesty, release-readiness tag board
**Confidence:** HIGH (codebase seams); MEDIUM (Pulumi option semantics / Floci RDS fidelity)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Config references existing VPC / DB by ID/ARN (or provider-native identifiers); `preview` reports import/adopt not create; apply adopts without replace. — **Reversibility:** one-way once operators learn the config shape
- **D-02:** Adopted resources MageLift does not own: any replace/destroy attempt fails before mutate with a message naming the resource (test-asserted). — **Reversibility:** one-way honesty contract
- **D-03:** Document attach limits + detach path; verify detach leaves cloud resource intact. — **Reversibility:** reversible docs
- **D-04:** Floci (or Pulumi mocks) prove import mechanics offline first. One free-tier AWS VPC+RDS adopt confirmation when credentials available (spend map pass 3 of 3). — **Reversibility:** N/A
- **D-05:** Tag day: every `docs/release-readiness.md` row Closed with evidence or Deferred with reason; every REQUIREMENTS row Complete or recorded deferral. Phase 1 hosted CI = Deferred (Act-only until GH minutes). Phase 7 GCP rows remain Pending until 07-06/07 evidence — do not fake-certify. — **Reversibility:** costly — tag board is the public honesty surface

### Claude's Discretion
- Exact YAML field names for existing VPC/DB refs (align schema)
- Whether detach is CLI command vs documented manual un-adopt

### Deferred Ideas (OUT OF SCOPE)
- Fake-certifying GCP before live pass
- Multi-cloud attach beyond AWS free-tier confirmation this milestone
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ATTACH-01 | Adopt existing VPC into MageLift stack | Extend proven `target.aws.existing.network` reference-without-own path; add preview adopt messaging + refuse-before-mutate |
| ATTACH-02 | Adopt existing managed DB (RDS/Cloud SQL) | Wire `ExistingDatabase` SDK kind into AWS config/spec/database component; AWS RDS only this milestone (Cloud SQL deferred) |
| ATTACH-03 | Preview import-vs-create + refuse destroy/mutate | App-level guards naming the resource; Pulumi mocks assert zero VPC/RDS creates when existing set |
| ATTACH-04 | Document limits + detach without losing resource | Docs + ADR 0010 supersede; detach = remove config refs (documented manual); offline detach proof |
| RELEASE-05 | Gate board Closed/Deferred; REQUIREMENTS settled | Audit `docs/release-readiness.md` + REQUIREMENTS; GCP Pending→07; hosted CI Deferred Act-only |
</phase_requirements>

## Summary

Phase 8 is mostly a **completion and honesty** phase, not greenfield attach. AWS VPC attach already exists as `target.aws.existing.network`: the network component registers imported VPC/subnet IDs as outputs and creates **zero** `aws:ec2/vpc:Vpc` / subnet / NAT / endpoint children. That pattern is the correct ownership model for “MageLift does not own this resource.” What is missing for ATTACH-01/03 is operator-visible **preview adopt reporting** and **test-asserted refuse-before-mutate** when config would replace/destroy adopted IDs.

Managed-database attach is the real capability gap. `sdk/v1` already defines `ExistingDatabase`, but config, stack `ExistingResources`, and `internal/cloud/aws/database` have no Existing path — stacks always call `database.New` and create RDS/Aurora. ADR 0010 still says brownfield attach DB is out of scope; REQUIREMENTS supersede that. Scope this milestone to **AWS RDS MySQL** (free-tier confirmation cell). Cloud SQL and other clouds stay deferred per CONTEXT.

Tag day (RELEASE-05) can close or defer every gate-board / REQUIREMENTS row without waiting on Phase 7 live, **except** GCP certify rows must stay Pending→07 (or Deferred with reason) — never fake-certify. Hosted CI stays Deferred (Act-only). Paid AWS adopt confirmation is HUMAN_GATE until `aws login` / ADC works (session expired at research time).

**Primary recommendation:** Extend the existing **reference-without-own** pattern to RDS (do not Pulumi-`Import` into state for resources MageLift refuses to destroy). Add schema `target.aws.existing.database`, database component Existing branch, stack wiring for writer endpoint + secret ARN, app-level refuse-before-mutate, docs/ADR update, offline Pulumi-mock (+ optional Floci) proof, then tag-board audit. Recommend **6 plans**.

**Recommended plan count:** 6

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Existing VPC/DB config surface | API / Backend (config schema + PlanFromConfig) | CDN / Static (docs) | YAML → Spec validation owns IDs before Pulumi |
| Adopt without create | API / Backend (network/database components) | — | Components skip child custom resources; outputs from refs |
| Preview import-vs-create report | API / Backend (CLI preview / plan summary) | — | Operators need explicit adopt lines, not silent omission |
| Refuse destroy/replace of adopted | API / Backend (pre-mutate gate) | — | Must fail before provider call; Pulumi Protect alone is insufficient for D-02 |
| Detach without cloud delete | API / Backend + docs | — | Removing config refs leaves cloud intact (never in state) |
| Floci / mock offline proof | API / Backend (tests) | — | D-04: mocks first; Floci optional smoke |
| Free-tier AWS confirm | External cloud | HUMAN_GATE | Spend map pass 3/3 when AWS ADC available |
| Tag board / REQUIREMENTS honesty | CDN / Static (docs) + planning | — | RELEASE-05 public honesty surface |

## Project Constraints (from .cursor/rules/)

From `.cursor/rules/serial-builds-only.mdc` and `AGENTS.md`:

- Never parallel `go build` / goreleaser / docker buildx locally (16 GB Mac; kernel panic risk).
- Packaging smoke only via `./scripts/release-smoke-local.sh` or `make release-smoke` (`--single-target --parallelism=1`, `GOMAXPROCS=1`).
- Ad-hoc builds: `GOMAXPROCS=1 GOFLAGS=-p=1`; at most one `go build` at a time.
- Prefer serial Floci/`go test` under Cursor; abort if swap climbs.
- Full multi-platform release matrices belong on CI, not this Mac.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/pulumi/pulumi/sdk/v3` | v3.253.0 (`go.mod`) | Automation / resource options | Existing MageLift IaC runtime [VERIFIED: go.mod] |
| `github.com/pulumi/pulumi-aws/sdk/v7` | v7.37.0 | VPC / RDS custom resources | Existing AWS provider [VERIFIED: go.mod] |
| MageLift `sdk/v1` `ExistingResourceRef` | in-repo | Network/database kind contracts | Already validates `ExistingDatabase` [VERIFIED: sdk/v1/types.go] |
| Pulumi mocks (`pulumi.WithMocks`) | in-repo tests | Offline adopt graphs | Pattern used by `network_test.go` / `database_test.go` [VERIFIED: codebase] |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Floci (`floci/floci:1.5.33` in CI) | 1.5.33 | AWS emulator | Optional offline smoke after mocks; not AWS certification [VERIFIED: .github/workflows/ci.yml] |
| AWS CLI / free-tier account | local | Pass 3/3 live confirm | Only when ADC/`aws login` available [VERIFIED: research probe — session expired] |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Reference-without-own (recommended) | Pulumi `Import` + `Protect` + `IgnoreChanges` | Import takes ownership; Protect can be unset; replace still risky. RetainOnDelete helps detach but still puts resource in state [CITED: pulumi.com/docs/.../import/, .../retainondelete/] |
| Reference-without-own | `RetainOnDelete(true)` on created then detached | Wrong direction for brownfield (resource not MageLift-created) |
| Documented manual detach | New `magelift detach` CLI | Extra surface; discretionary — docs sufficient for tag day |

**Installation:** None — no new Go modules. Reuse existing Pulumi AWS SDK.

**Version verification:** `go.mod` lines for pulumi/sdk/v3 and pulumi-aws/sdk/v7 read 2026-07-30. [VERIFIED: go.mod]

## Package Legitimacy Audit

No new external packages for this phase.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| — | — | — | — | — | — | N/A — no installs |

**Packages removed due to [SLOP] verdict:** none  
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```text
magelift.yaml
  target.aws.existing.network (+ subnet IDs)
  target.aws.existing.database (+ secretArn / endpoint fields)
        │
        ▼
PlanFromConfig → Spec.Existing.{Network,Database}
        │
        ├─ validate IDs / kinds / completeness
        │
        ▼
stack.New
        │
        ├─ network.New(Existing≠nil)
        │     └─ NO ec2.Vpc/Subnet/NAT children
        │     └─ outputs: vpcId, subnet IDs
        │
        ├─ database.New(Existing≠nil)   ← NEW
        │     └─ NO rds.Instance/Cluster children
        │     └─ outputs: writerEndpoint, masterSecretArn from refs
        │
        └─ runtime/security still create MageLift-owned resources
              (SGs, ECS, ALB, …) inside adopted VPC

preview path
        │
        ├─ report: ADOPT network vpc-… / database db-…
        └─ report: CREATE security/runtime/…

pre-mutate / destroy gate (ATTACH-03)
        │
        └─ if plan would replace/destroy adopted ID → error naming resource
```

### Recommended Project Structure

```
internal/config/model.go          # AWSExistingResources.Database (+ fields)
internal/cloud/aws/stack/spec.go  # ExistingResources.Database
internal/cloud/aws/stack/config.go
internal/cloud/aws/database/database.go  # Existing branch (mirror network)
internal/cloud/aws/network/network.go    # already Existing; honesty tests
docs/configuration.md / migrating-from-paas.md / brownfield-attach.md (new)
docs/adr/0010-database-dump-seed.md      # supersede attach-out-of-scope
docs/release-readiness.md                # RELEASE-05 audit
tests/... or package *_test.go           # refuse-before-mutate + adopt mocks
```

### Pattern 1: Reference-without-own (canonical)

**What:** When `Existing` is set, register the component, set outputs from external IDs, create **no** provider custom resources for that capability.  
**When to use:** Any resource MageLift must never destroy (VPC, adopted RDS).  
**Example:** Existing network early-return in `internal/cloud/aws/network/network.go` (lines ~102–116). [VERIFIED: codebase]

### Pattern 2: Config mirror for database

**What:** Add `target.aws.existing.database` as `AWSExistingResource` (provider/kind/externalId) plus connection refs MageLift cannot invent:

| Field | Role |
|-------|------|
| `existing.database.provider` | `aws` |
| `existing.database.kind` | `database` |
| `existing.database.externalId` | RDS instance ID or ARN (validate format) |
| `existing.databaseSecretArn` **or** nested `secretArn` | Secrets Manager master user secret (required for ECS injection) |
| `existing.databaseEndpoint` **or** nested `endpoint` | Writer hostname (required if not looked up at apply) |

**Recommendation (discretion):** Prefer nested fields under `existing.database` for secretArn/endpoint to keep `AWSExistingResource` stable, **or** sibling keys `databaseSecretArn` / `databaseEndpoint` parallel to subnet ID siblings — pick one shape and document in schema prose. Prefer:

```yaml
target:
  aws:
    existing:
      network:
        provider: aws
        kind: network
        externalId: vpc-0123…
      publicSubnetIds: […]
      privateSubnetIds: […]
      dataSubnetIds: […]
      database:
        provider: aws
        kind: database
        externalId: db-magento-prod   # or full ARN
        secretArn: arn:aws:secretsmanager:…:secret:…
        endpoint: magento.xxxxx.eu-north-1.rds.amazonaws.com
```

Extend `AWSExistingResource` with optional `SecretARN` / `Endpoint` only when kind=database, **or** introduce `AWSExistingDatabase` struct. Prefer a dedicated struct to avoid polluting network refs. [ASSUMED: best schema ergonomics — discretion]

### Pattern 3: Refuse-before-mutate (D-02)

**What:** Before apply/destroy, detect plans that would create VPC/RDS when existing was set, change adopted external IDs, or delete Magento-owned resources in a way that implies mutating the adopted DB/VPC. Fail with `adopted resource <name> (<externalId>): MageLift does not own this resource`.  
**When to use:** Always when Existing.* is present.  
**Do not rely on:** Pulumi `Protect` alone (operator can clear protect). [CITED: pulumi.com/docs/.../options/]

### Pattern 4: Detach (discretion → docs)

**What:** Documented manual un-adopt: remove `existing.network` / `existing.database` from config for a **new** stack, or destroy the MageLift stack while adopted resources were never in Pulumi state. Cloud VPC/RDS remain.  
**Do not ship** a dedicated CLI for tag day unless preview/destroy messaging is unclear. Verification: offline mock destroy leaves no Delete calls for adopted IDs; live confirm uses `aws rds describe-db-instances` / `aws ec2 describe-vpcs` after stack destroy. [ASSUMED: detach CLI unnecessary for SC4]

### Anti-Patterns to Avoid

- **Pulumi-Import then manage adopted DB:** Contradicts D-02; any property drift becomes replace risk.
- **Fake GCP certify on tag board:** Violates D-05 / CONTEXT deferred.
- **Claiming Floci = AWS attach certification:** Floci is emulator evidence only (same honesty as Phase 3).
- **Creating RDS subnet group while “adopting” instance:** Implies ownership / possible destroy of MageLift-created group only — OK; do not create a second DB instance.
- **Cloud SQL in this phase:** Deferred multi-cloud attach.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Bring foreign VPC under management | Custom import state editor | Existing `network.Existing` pattern | Already proven; zero child resources |
| Prevent accidental cloud delete | Homegrown provider wrapper | Never put adopted resource in state + app refuse gate | Protect/RetainOnDelete still allow operator mistakes if imported |
| Offline graph proof | Live AWS for every PR | `pulumi.WithMocks` | Existing network/database tests |
| Emulator AWS API | Custom LocalStack fork | Pinned Floci in CI | Already in workflow |
| Secret injection for Magento | Custom credential store | Existing Secrets Manager ARN → ECS secrets | Same as managed DB path |

**Key insight:** For “does not own,” the safest IaC is **not managing**. Import is for taking ownership; MageLift’s honesty contract is the opposite.

## Runtime State Inventory

| Category | Items Found | Action Required |
|----------|-------------|-----------------|
| Stored data | Pulumi stack state for greenfield envs may contain MageLift-created VPC/RDS | Attach applies to **new** stacks with existing refs; do not rewrite historical state mid-flight without HUMAN_GATE |
| Live service config | Free-tier AWS account (eu-north-1 prior evidence); GCP project for Phase 7 | Attach paid cell uses AWS only; no GCP attach |
| OS-registered state | None for attach strings | None — verified by absence of launchd/systemd Magelift attach units |
| Secrets/env vars | AWS ADC / `aws login` (expired at research); Cloudflare token for Phase 7 DNS | HUMAN_GATE for pass 3/3; not Phase 8 code |
| Build artifacts | None named for attach | None |

## Common Pitfalls

### Pitfall 1: Silent omission ≠ “reports import”
**What goes wrong:** Preview shows no VPC create but never says “adopt.” SC1 fails audit.  
**Why:** Existing network early-return omits resources without messaging.  
**How to avoid:** Explicit preview/plan summary lines for each adopted ref.  
**Warning signs:** Tests only assert zero creates.

### Pitfall 2: Import + Protect false safety
**What goes wrong:** Operator unprotects or property mismatch triggers replace.  
**Why:** Pulumi Protect is advisory relative to D-02’s hard fail.  
**How to avoid:** Reference-without-own + refuse-before-mutate naming resource.  
**Warning signs:** Adopted IDs appear in `pulumi stack --show-urns` as custom resources.

### Pitfall 3: Existing network without egress
**What goes wrong:** ECS tasks fail pulls/logs because MageLift skips NAT/endpoints in existing mode.  
**Why:** Documented already in configuration.md.  
**How to avoid:** ATTACH-04 limits must restate egress/VPC endpoint ownership.  
**Warning signs:** Runtime ImagePull / Secrets Manager timeouts.

### Pitfall 4: DB adopt missing secret/endpoint
**What goes wrong:** Component “adopts” ID but Magento cannot connect.  
**Why:** Managed path gets secret from `ManageMasterUserPassword`; existing DB must supply ARN + endpoint.  
**How to avoid:** Validate required secretArn+endpoint at PlanFromConfig.  
**Warning signs:** Empty `databaseWriter` / execution role policy gaps.

### Pitfall 5: Tag board fake-close GCP
**What goes wrong:** RELEASE-05 lies; ADR 0007 multi-cloud claim poisoned.  
**Why:** Pressure to “finish” Phase 8 while 07-06 blocked.  
**How to avoid:** Leave GCP Magento deploy Ops / GCP-06 as Pending→07 or Deferred with reason.  
**Warning signs:** Board says Closed without `.magelift/gcp-matrix` live evidence.

### Pitfall 6: Parallel local builds during Floci/AWS
**What goes wrong:** Kernel panic on 16 GB Mac.  
**Why:** Serial-builds-only rule.  
**How to avoid:** `GOMAXPROCS=1`; one test/build process.  
**Warning signs:** Swap climb / thrash under Cursor.

## Code Examples

### Existing network early-return (canonical)

```go
// Source: internal/cloud/aws/network/network.go (existing codebase)
if args.Existing != nil {
    if err := validateExistingNetwork(args, args.Existing); err != nil {
        return nil, err
    }
    component.VpcID = pulumi.ID(args.Existing.VPCID).ToIDOutput()
    // … subnet ID outputs …
    return component, nil // no ec2.NewVpc
}
```

### Recommended database Existing branch (sketch)

```go
// Source: pattern mirrored from network; implement in database.go
type ExistingDatabase struct {
    Identifier string // externalId
    Endpoint   string
    SecretARN  string
}

if args.Existing != nil {
    if err := validateExistingDatabase(args.Existing); err != nil {
        return nil, err
    }
    component.ClusterARN = pulumi.String(args.Existing.Identifier).ToStringOutput() // or ARN field
    component.WriterEndpoint = pulumi.String(args.Existing.Endpoint).ToStringOutput()
    component.ReaderEndpoint = component.WriterEndpoint
    component.MasterSecretARN = pulumi.String(args.Existing.SecretARN).ToStringPtrOutput()
    // Register outputs; return — no rds.NewInstance / NewCluster
    return component, nil
}
```

### Pulumi RetainOnDelete (only if ever imported — not primary path)

```go
// Source: https://www.pulumi.com/docs/iac/concepts/resources/options/retainondelete/
db, _ := NewDatabase(ctx, "db", &DatabaseArgs{}, pulumi.RetainOnDelete(true))
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| ADR 0010: attach existing DB out of scope | REQUIREMENTS ATTACH-02 in Phase 8 | This milestone | Must supersede ADR consequence |
| Greenfield-only VPC | `existing.network` reference-without-own | ~2026-07-18 lesson | ATTACH-01 mostly complete |
| Pulumi Import for brownfield | Prefer reference-without-own for non-owned | Research 2026-07-30 | Safer for D-02 |
| Post-beta “brownfield attach” wishlist | Phase 8 on critical path | ROADMAP | Tag day couples attach + gate audit |

**Deprecated/outdated:**
- ADR 0010 bullet “Brownfield attach existing DB remains out of scope” — supersede in Phase 8 docs plan.
- `docs/post-beta-roadmap.md` listing attach as post-beta only — update cross-links to Phase 8.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Nested `existing.database.{secretArn,endpoint}` is preferred YAML shape | Pattern 2 | Planner must confirm with maintainer if sibling keys preferred |
| A2 | Documented manual un-adopt satisfies ATTACH-04 without new CLI | Pattern 4 | If SC4 interpreted as needing CLI, add thin command |
| A3 | Floci RDS/VPC is good enough for optional smoke, not required for ATTACH offline close | Standard Stack | If Floci EC2 VPC Describe is incomplete, rely solely on Pulumi mocks |
| A4 | Free-tier confirmation uses pre-existing VPC+RDS (not create-then-adopt in same pass) | Environment | Spend/shape differs if operator must create fixtures first |

**Resolved from CONTEXT (not open):**
- Phase 7 live may still be blocked → GCP rows Pending→07; do not fake-certify.
- Floci/mocks first; one small AWS adopt when creds available.
- Multi-cloud attach deferred.
- Hosted CI Deferred Act-only.

## Open Questions

> All discretionary / dependency questions from discuss-phase are **RESOLVED** below for the planner. No blocking unknowns remain for planning.

1. **YAML field names for DB** — RESOLVED (discretion recommendation): use `target.aws.existing.database` with `provider`/`kind`/`externalId`/`secretArn`/`endpoint`; keep network shape unchanged.
2. **Detach CLI vs docs** — RESOLVED (discretion recommendation): documented manual un-adopt only for this milestone; verify via mocks + optional live describe-after-destroy.
3. **Cloud SQL** — RESOLVED (deferred): AWS RDS only; ATTACH-02 Cloud SQL wording satisfied later or recorded deferral on REQUIREMENTS for non-AWS.
4. **Phase 7 GCP certify** — RESOLVED: RELEASE-05 leaves GCP Pending→07 / Deferred with reason until 07-06/07 evidence.
5. **AWS credentials for pass 3/3** — RESOLVED as HUMAN_GATE: offline plans proceed; paid confirmation plan is checkpoint when `aws sts get-caller-identity` works (expired at research).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Tests / build | ✓ | go1.26.5 | — |
| Docker | Floci | ✓ | 29.6.2 | Pulumi mocks only |
| Pulumi CLI | Local preview smoke | ✓ | v3.255.0 | Automation API in-process |
| AWS CLI / ADC | Paid adopt confirm | ✗ (session expired) | aws-cli/2.36.11 | HUMAN_GATE; offline mocks |
| gcloud | Phase 7 (not Phase 8 attach) | ✓ SDK installed | 576.0.0 | N/A for attach |
| Floci image | Optional smoke | ✓ (CI pin 1.5.33) | via Docker | Mocks |

**Missing dependencies with no fallback:**
- Live AWS ADC for spend-map pass 3/3 (blocks only the paid confirmation plan, not offline ATTACH work).

**Missing dependencies with fallback:**
- Floci VPC/RDS fidelity → Pulumi mocks.

## Recommended Plans (6)

| Plan | Focus | Requirements |
|------|--------|--------------|
| **08-01** | VPC attach honesty: preview adopt messaging + refuse-before-mutate for existing network; regression tests on current Early-return | ATTACH-01, ATTACH-03 (network) |
| **08-02** | Config/schema/Spec: `existing.database` + validation + PlanFromConfig mapping | ATTACH-02 (surface) |
| **08-03** | `database.Existing` component path + stack wiring (skip RDS create; wire secret/endpoint/outputs/IAM) + mock graph tests | ATTACH-02 (apply) |
| **08-04** | Unified ATTACH-03: preview import-vs-create report for network+DB; destroy/replace refusal tests naming resources | ATTACH-03 |
| **08-05** | Docs + ADR 0010 supersede + attach limits/detach runbook (`docs/brownfield-attach.md` or section); offline detach proof | ATTACH-04 |
| **08-06** | D-04 offline Floci-or-mock evidence record; HUMAN_GATE free-tier AWS confirm when ADC up; RELEASE-05 gate board + REQUIREMENTS audit (GCP Pending→07; CI Deferred) | D-04, D-05, RELEASE-05 |

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` + race (`make test` → `go test -race ./...`) |
| Config file | none — standard Go |
| Quick run command | `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/aws/network ./internal/cloud/aws/database ./internal/cloud/aws/stack -count=1` |
| Full suite command | `make test` (prefer serial under Cursor if memory pressure) |
| Floci | `make floci-test` (`./scripts/floci-test.sh`) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ATTACH-01 | Existing VPC → zero VPC/subnet creates | unit/mock | `go test ./internal/cloud/aws/network -run ExistingNetwork -count=1` | ✅ `network_test.go` |
| ATTACH-01 | Preview reports adopt | unit | new test on preview/plan summary | ❌ Wave 0 |
| ATTACH-02 | Existing DB → zero RDS creates; outputs from refs | unit/mock | `go test ./internal/cloud/aws/database -run Existing -count=1` | ❌ Wave 0 |
| ATTACH-02 | Config maps database existing | unit | `go test ./internal/cloud/aws/stack -run ExistingDatabase -count=1` | ❌ Wave 0 |
| ATTACH-03 | Refuse replace/destroy naming resource | unit | `go test ./internal/cloud/aws/stack -run AdoptedRefuse -count=1` | ❌ Wave 0 |
| ATTACH-04 | Docs + detach leaves resource (mock: no Delete) | docs + unit | mock destroy assertion + doc presence | ❌ Wave 0 |
| RELEASE-05 | Board rows Closed/Deferred; REQUIREMENTS settled | manual audit | checklist in 08-06 | ❌ Wave 0 (process) |

### Sampling Rate

- **Per task commit:** package-scoped `go test -race` under `GOMAXPROCS=1`
- **Per wave merge:** `make test` (abort if swap climbs)
- **Phase gate:** ATTACH unit/mocks green; RELEASE-05 board audited; paid AWS optional HUMAN_GATE

### Wave 0 Gaps

- [ ] Database Existing branch tests (`database_test.go`)
- [ ] Stack PlanFromConfig existing database tests
- [ ] Preview adopt messaging tests
- [ ] Refuse-before-mutate tests with resource name in error
- [ ] Detach / destroy-no-delete mock assertion
- [ ] Optional Floci smoke only after mocks green

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | — |
| V3 Session Management | no | — |
| V4 Access Control | yes | IAM least-privilege to adopted secret ARN only; no broaden-to-* |
| V5 Input Validation | yes | Regex/format validation for vpc-/subnet-/db IDs and ARNs (mirror network) |
| V6 Cryptography | no new | Reuse existing KMS/Secrets Manager patterns; do not hand-roll |

### Known Threat Patterns for attach

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Accidental destroy of customer VPC/RDS | Elevation / Tampering | Reference-without-own + refuse-before-mutate |
| Adopt wrong DB via typo ID | Tampering | Strict ID/ARN validation; preview shows external IDs |
| Secret ARN confused with plaintext | Information disclosure | Secrets Manager ARN only; never inline password in YAML |
| Privilege expansion via execution role | Elevation | Policy scoped to single secret ARN + KMS key |

## Sources

### Primary (HIGH confidence)

- `internal/cloud/aws/network/network.go` — Existing network early-return
- `internal/cloud/aws/network/network_test.go` — `TestExistingNetworkUsesImportedSubnetsWithoutCreatingVPCResources`
- `internal/config/model.go` / `schema.go` / `docs/configuration.md` — existing.network surface
- `sdk/v1/types.go` — `ExistingDatabase` kind present, unwired
- `internal/cloud/aws/database/database.go` — always creates RDS/Aurora today
- `docs/adr/0010-database-dump-seed.md` — attach out of scope (to supersede)
- `docs/release-readiness.md` — gate board state
- `.planning/ROADMAP.md` Phase 8 SC1–SC5
- `.planning/phases/08-brownfield-attach-tag-day/08-CONTEXT.md`
- `go.mod` — Pulumi versions
- `.cursor/rules/serial-builds-only.mdc` / `AGENTS.md`

### Secondary (MEDIUM confidence)

- [Pulumi Import option](https://www.pulumi.com/docs/iac/concepts/resources/options/import/) — custom-resource import semantics [CITED]
- [Pulumi RetainOnDelete](https://www.pulumi.com/docs/iac/concepts/resources/options/retainondelete/) — detach-from-state without provider Delete [CITED]
- [Floci README / floci.io](https://github.com/floci-io/floci) — claims EC2 VPC + RDS Docker MySQL [CITED]

### Tertiary (LOW confidence)

- Floci fidelity for MageLift Pulumi AWS provider VPC/RDS adopt path — not exercised in `tests/floci/` today [ASSUMED]

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — versions and patterns from repo
- Architecture: HIGH — extend proven network pattern to DB
- Pitfalls: HIGH — validated against SC wording + existing docs gaps
- Floci live attach: LOW — optional only

**Research date:** 2026-07-30  
**Valid until:** 2026-08-29 (30 days; Pulumi options stable; Floci pin may move)

**Plan count:** 6
