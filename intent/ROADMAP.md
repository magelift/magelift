# Roadmap to first release: narrow alpha, then earn stability

Goal: one tagged, installable alpha (`v0.1.0-alpha.1`, exact naming subject to
the release decision) that lets a pilot agency team install from verified
binaries and run one documented GCP Autopilot recipe — deploy, release,
failed-release recovery, post-expiry operation, backup/restore, preview
expiry — without DevOps-owned infrastructure work. No tag is authorized by
this file; `reference-store-acceptance` ends with a go/no-go plus the exact
tag command, and the maintainer makes the release call.

Authority: `intent/audit.md` (2026-09-16). It assigns every finding F01–F15 to
an intent below. This file only orders the queue and states the gates. Archived
intents are history, not the queue; historical evidence is linked where it
still counts and never mistaken for current release proof.

Old roadmap (24 orders to `v1.0.0-rc.1`) is superseded in full. Recoverable
from git history. The stable cut is deferred past alpha, pilots, and AWS
parity — see `v1-stable-cut` (deferred).

## The queue (8 to alpha, 3 deferred)

Work one intent at a time: accept, spec, plan, implement, verify, archive.
No code before the plan is approved. Local contract, mock, schema, and docs
tracks may proceed in parallel while they provision nothing paid.

| Order | Intent | Cloud | Exit | Status |
| --- | --- | --- | --- | --- |
| 1 | `release-trust-baseline` | none | Evidence gate green (sealed-run mismatch reconciled with provenance, derived files regenerated); matrix plus evidence name exactly what each cell proves; website/docs cost and security language matches code (`Enforced: false` honored, no hard-cap promise); writable-path claims aligned. | [archived](archive/2026-09-16-release-trust-baseline/) |
| 2 | `provider-plugin-contract` | none | Approved spec plus ADR plus human-docs update define core vs provider, the versioned operations protocol, and independent-release mechanics. No implementation; followers prove it. | [archived](archive/2026-09-16-provider-plugin-contract/) (ADR 0013 reconciled by corrections) |
| 3 | `mocked-shop-scenarios` | none | Original synthetic fixtures with example-only identities; three honest layers (offline protocol/config, local app where useful, live owned elsewhere); gaps fail loudly and file back. Proves routing, contracts, failures, validation — never store proof. | [archived](archive/2026-09-16-mocked-shop-scenarios/) |
| 4 | `magento-deployment-safety` | local; live only if the spec needs it | Deploy success means intended rollout plus bounded Magento readiness (generation, replicas, digest); one lifecycle authority (PHP owns, Go orchestrates); assets proved delivered; conservative incompatible-migration policy; the five failure classes tested. | [archived](archive/2026-09-16-magento-deployment-safety/) + [corrections](alpha-review-corrections/) R04/R08 |
| 5 | `gcp-autonomous-provider` | GCP live (packed, destroy on exit) | One autonomous GCP Autopilot provider built outside the root module on the public SDK: provisions plus all seven day-2 ops over the versioned protocol, explicit negotiation, fail-closed integrity/compat, fresh-credential operations with expiry/refresh/restart/CI-identity tested. Core builds without provider SDKs. | [archived](archive/2026-09-16-gcp-autonomous-provider/) + [corrections](alpha-review-corrections/) R02/R05/R10 |
| 6 | `full-deployment-coverage` | none beyond what order 5 proved | One documented path from clean workstation to working shop on the alpha recipe; human-action prerequisites named pre-deploy; one validated SMTP path; classified deploy failures; skills match the built CLI. | [archived](archive/2026-09-16-full-deployment-coverage/) + [corrections](alpha-review-corrections/) R03/R12 |
| 7 | `verified-provider-distribution` | none (local registries/proxies prove the sequence) | Clean-module SDK consumer plus provider resolve with `GOWORK=off`; YAML-driven provider download with checksum/signature/digest/compat enforcement; one fail-closed trust policy for installer/updater/core/plugins; install plus CI use verified release binaries. | [archived](archive/2026-09-17-verified-provider-distribution/) + [corrections](alpha-review-corrections/) R01/R06/R07/R11 |
| 7.5 | `alpha-review-corrections` | GCE clean-machine proof only (destroyed after) | Every R01–R12 finding resolved or explicitly dispositioned; orders 4–7 reports amended; full gates green; then the order-8 re-run. | in progress |
| 8 | `reference-store-acceptance` | GCP live (release candidate + artifacts) | Shipped-path loop green on pinned recipe: initial deploy, release, failed release with CLI-diagnosed recovery, post-expiry ops, backup/restore (encryption key + media), autonomous preview expiry with residual-cost report. Ends with go/no-go plus tag command. Then pilot intake begins. | paused for corrections; re-runs from rc.2 |

Deferred (start only after the alpha tag; never gate it):

| Intent | Status | Resume trigger |
| --- | --- | --- |
| `aws-provider-parity` (draft) | post-alpha | Alpha tagged + pilot feedback started. Second autonomous provider on the same protocol; AWS gaps closed or honestly documented; search earns app-level proof or stays infra-only. |
| `v1-stable-cut` (deferred) | post-alpha + AWS parity + pilots | Stability review with user evidence (diagnose from CLI output, recover without maintainer commands, understand cost/responsibilities). Then define the freeze surface from proved contracts. |
| `eu-providers-experimental` (deferred) | post-alpha | Maintainer call, default after AWS parity unless a pilot pays for EU first. Scaleway GREEN kept, OVH retry when pools converge. |

## Finding-to-intent map

| Finding | Owner |
| --- | --- |
| F01 provider independence not achieved | `provider-plugin-contract`, then `gcp-autonomous-provider` |
| F02 subprocess proof, not product | `gcp-autonomous-provider`, `verified-provider-distribution` |
| F03 SDK boundary too porous | `provider-plugin-contract` |
| F04 GCP ops on expiring saved token | `gcp-autonomous-provider`, verified in `reference-store-acceptance` |
| F05 health can mean scheduler health | `magento-deployment-safety` |
| F06 two Magento lifecycle authorities | `magento-deployment-safety` |
| F07 workspace masks distribution | `verified-provider-distribution` |
| F08 claims exceed cell evidence | `release-trust-baseline`, `reference-store-acceptance` |
| F09 evidence integrity gate red | `release-trust-baseline` |
| F10 budgets are not enforced caps | `release-trust-baseline`, `full-deployment-coverage`, acceptance |
| F11 first-install verification skippable | `verified-provider-distribution` |
| F12 security docs vs runtime disagree | claims: `release-trust-baseline`; runtime: `magento-deployment-safety`, `full-deployment-coverage` |
| F13 onboarding confuses automation/ownership | `full-deployment-coverage`, `release-trust-baseline`, `aws-provider-parity` |
| F14 synthetic scenarios must not fake stores | `mocked-shop-scenarios`, acceptance |
| F15 clearer ownership, not more repos | boundaries: `provider-plugin-contract`; queue: this file |

## Disposition of the previous open work

| Previous intent | Disposition | Why |
| --- | --- | --- |
| `full-deployment-coverage` | Rewritten as draft reference onboarding | Three email vendors before one working shop delayed the primary workflow; old spec/plan superseded, `audit.md` kept as inventory with F13 corrections. |
| `mocked-shop-scenarios` | Rewritten as draft synthetic foundation | Useful test ideas kept; mocks separated from store proof; private-shop dependence removed; old spec/plan superseded. |
| `v1-stable-cut` | Deferred past alpha | Report PENDING, ref stale, architecture not freezable; old spec/plan/report kept historical. |
| `eu-providers-experimental` | Deferred with evidence kept | Valuable experimental work, unnecessary for the first supported release; Scaleway GREEN and OVH PARKED retained. |

None of the four was archived as complete; none had a passing verification
record for its old scope. Prior files carry explicit HISTORICAL banners.

## Alpha boundary (what the tag promises)

- One GCP Autopilot recipe with pinned compatible versions (Magento, PHP,
  MySQL, Valkey, search, RabbitMQ, image digests), serving HTTPS, assets,
  search, cron/consumers where required, outbound SMTP, persistent media.
- Bounded promises only: no arbitrary versions, no zero-downtime incompatible
  schema changes, no cross-cloud DR, no hard spending cap.
- Existing integrations stay only with honest experimental labels and must
  not drag embedded provider SDKs back into the lean core.
- If pilot interviews prove the first pilot must be AWS, flip orders 5–8 to
  AWS at intent acceptance with identical gates. Never two simultaneous
  reference implementations to avoid deciding.

## Budget discipline

| Target | Rule |
| --- | --- |
| GCP | Carries reference proof (orders 5, 8). Packed sessions, destroy on exit, per-session spend recorded. Existing functional search/operator evidence minimizes new spend. |
| AWS | Zero live spend before `aws-provider-parity`. At resume, re-verify credits and write a per-session cap with retry margin; minimum sizes, short-lived stacks. Never inherit the old ~$180 number. |
| OVH / Scaleway | Zero live spend before the deferred EU resume. Then: serialized, minimum SKUs, destroy always, no KEEP, short TTL, own-money cap. |
| Vendors (Cloudflare, Fastly, New Relic) | Attach to the GCP alpha origin or record unproven. No second origin for a vendor. |

## Non-goals to alpha

Stable v1 freeze, RC semantics, community catalog hosting, per-provider
independent release cadence beyond the contract's alpha mechanics. EKS, GKE
Standard, MKS, Kapsule Magento certification. AOSS serverless certification,
three-node HA search, X-Ray, regional DR. Provider cost adapters beyond honest
estimates. Cloud SQL attach and non-AWS brownfield. Windows package managers
beyond archive download. MkDocs migration, YAML v4 final, JSON v2,
test-runner major bumps. Three-vendor managed email.

## Working agreement

- One intent at a time through accept, spec, plan, implement, verify, archive.
- Tests ship with their implementation; acceptance never excuses deferred testing.
- Reuse valid evidence within its scope; rerun when code, packaging, or the
  public execution path changes. Final proof references the release candidate
  and artifacts, never an old commit.
- Pilot success is user evidence: diagnose from CLI output, recover without
  maintainer-only commands, understand ongoing cost and responsibilities.
- Ask-first: release tags, protected/production destroy, history rewrite,
  public module publication.
