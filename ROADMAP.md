# Roadmap to first stable

Goal: one tagged, installable CLI that lets an agency or SME team without DevOps
capacity install, import or preset, and run Magento on a certified origin, with
experimental EU providers honestly labeled. GCP carries live testing because
budget there is effectively unlimited. AWS live work stays inside the remaining
~$180. Everything else proves locally first.

This file orders `intent/` by dependency, not by enthusiasm. Each intent keeps
its own problem, evidence, and open questions; this file only says what goes
first and why. `docs/post-beta-roadmap.md` owns post-v1 tracks. `openspec/BACKLOG.md`
still names the single active objective when live cloud work runs.

## Vision verdict

Lean core plus provider interface: yes, and it mostly exists (`sdk/v1`,
`platform.StackModule`, `internal/cloud/<provider>/`, ADR 0003/0004). Freeze
that boundary for v1.

Providers as signed subprocess plugins: v1 proves Dial end to end for one
first-party adapter (maintainer decision; GCP Autopilot is the candidate since
the host already serves its RPCs) instead of blocking on full extraction. The
remaining adapters stay in-process. Per-provider independent releases and the
community catalog still land post-v1 without breaking the frozen YAML/CLI
contract. Community plugins ride the compile-time SDK contract for v1
(`cli.NewWithExtensions`, no `internal/` imports), proven by the clean-room
example.

Record the sequencing in an ADR under `lean-core-provider-boundary` so the
decision sticks.

## Phase 0: decisions (no cloud spend)

| Order | Intent | Why first |
| --- | --- | --- |
| 1 | `lean-core-provider-boundary` | Determines what v1 ships as, including Dial proof scope for one adapter. Until the ADR lands, every provider change risks rework. |
| 2 | `magento-search-v1`, decision half | Decided: live cells on both origins, AWS done at minimum spend. The costing shapes Phase 2 budgeting. Implementation lands in Phase 2. |

Exit: ADR accepted; search shapes and AWS spend cap written down.

## Phase 1: ship vehicle and onboarding (local only)

| Order | Intent | Why here |
| --- | --- | --- |
| 3 | `onboarding-migration-presets` | Install, import or preset, validated YAML in 30 minutes local. Unblocks trial adoption before any cloud proof. Mostly local: importer fixtures, presets, doctor output. |
| 4 | `local-dev-parity` | Fast credible loop with drift warnings. Same audience as onboarding, same local-only cost. |
| 5 | `pulumi-error-causes` | Small fix with large operator payoff: failed deploys name their cause. Fold into Phase 1 while onboarding error paths are fresh. |
| 6 | `lean-core-provider-boundary`, Dial engineering | Host plus proof-adapter RPCs, contract tests, local subprocess proof with signature and digest enforcement. No cloud needed; live Dial proof attaches to the GCP packed session in Phase 2. |
| 7 | `v1-stable-cut`, engineering half | Release wiring (GoReleaser, checksums, SBOM, provenance, upgrade verification, install docs) proceeds in parallel. The tag itself waits for Phase 2 proof. |

Exit: a new team validates YAML and runs local from docs alone; deploy
failures print classified causes; Dial subprocess proof passes locally; release
artifacts build and verify locally.

## Phase 2: prove the origins (GCP first, AWS once)

| Order | Intent | Why here |
| --- | --- | --- |
| 8 | `certified-origins-gcp-aws` | GCP packed sessions on the unlimited project including the live Dial proof, then one packed AWS session inside the remaining credits. Create-once plus warm transitions, minimum sizes, destroy plus `assert_clean` always. |
| 9 | `magento-search-live-proof` | GCP workload cell plus efficient AWS provisioned cell, each attached to its origin's packed session. No standalone search stacks. One AWS attempt; on failure keep GCP proof and re-plan. Shapes, cap, and criteria frozen in ADR 0012. |
| 10 | `preview-env-lifecycle` | PR preview up, staging on main, gated production, sweep expired. Live loop proof rides the GCP packed session. |

Test pyramid inside this phase, in order: unit and static analysis, Pulumi
mock graphs, harness shape checks, Floci AWS plus floci-gcp contracts, then
live cells that only a real provider can prove. Emulators never certify
Autopilot, Armor data plane, managed TLS, Cloud SQL PITR, live WAF, OIDC token
exchange, or regional DR. Vendor cells (Fastly, New Relic, SendGrid,
Cloudflare) attach to the GCP origin or stay unproven.

Exit: two certified origins with current evidence, live Dial proof on the
proof adapter, both search cells closed or AWS honestly experimental, preview
loop proved live, AWS spend inside budget with retry margin intact.

## Phase 3: breadth and operator catalog (capped spend)

| Order | Intent | Why here |
| --- | --- | --- |
| 11 | `eu-providers-experimental` | One serialized destroy-on-exit preview per EU provider, minimum SKUs, Magento stays experimental/`not-run`. Runs after certified origins so it cannot compete for attention or money. |
| 12 | `operator-finops-catalog` | Harden run, debug, and spend commands on certified origins; live-prove the top five, Floci plus unit for the rest. Needs Phase 2 origins to exist. |
| 13 | `ai-skills-tracks`, acceptance | Skills update in every intent as behavior ships; this step is the acceptance run (scripted agent on a fixture shop) plus contributor-track check. Last because it verifies everything above. |
| 14 | `v1-stable-cut`, tag | Gate board signed, tag pushed, archives published, install docs flipped to archives first. |

Exit: EU adapters honestly labeled, operator catalog live-proved where it
matters, skills verified, `v1.0.0-rc.1` installable without a Go toolchain.

## Deferred post-v1

These stay in `intent/` but are not on the v1 path:

| Intent | Reason |
| --- | --- |
| `docs-toolchain-after-mkdocs-1` | Pinned MkDocs 1.6.1 builds green; migration risk outweighs warning noise before the tag. |
| `yaml-v4-final` | Blocked on upstream; no final v4 tag exists and the RC pin passes its tests. |

Removed from `intent/` in the v1 review (recoverable from git history): five
toolchain micro-intents with no user-visible v1 gain (`encoding-json-v2`,
`cutlast-cli-image-refs`, `copy-link-application-image`,
`phpunit-without-raising-php-floor`, `psalm-7-stable`). Reopen any of them
post-v1 if a concrete failure justifies it.

## Budget discipline

| Target | Rule |
| --- | --- |
| GCP | Unlimited project carries reference-origin proof, live Dial proof, both search workload proof, and all vendor attachments. Still destroy on exit and record spend per session. |
| AWS | ~$180 remaining. One packed session covering origin proof plus the efficient provisioned search cell: minimum sizes, short-lived stacks, written per-session cap with retry margin. No Aurora or Amazon MQ live. |
| Scaleway | Shared $50 own-money cap with vendor cells. One serialized preview, minimum SKUs, destroy always, no KEEP. |
| OVH | Same restraint as Scaleway. One serialized preview, destroy always, no KEEP. |
| Cloudflare, Fastly, New Relic, SendGrid | Attach to the GCP packed origin or record unproven. No second origin for a vendor. |

## Non-goals for v1

Per-provider independent release lifecycles (Dial is proved for one adapter;
releases stay single-version), community catalog hosting. EKS, GKE Standard,
MKS, Kapsule Magento certification. AOSS serverless certification, three-node
HA search, X-Ray, regional DR. Provider cost adapters beyond AWS ECS live
pricing. Cloud SQL attach and non-AWS brownfield. Windows package managers
beyond archive download. MkDocs migration, YAML v4 final, JSON v2, and
test-runner major bumps.

## Working agreement

One intent at a time through accept, spec, plan, implement, verify, archive.
When a packed live session is open, it is the only active objective. Local
CLI contracts a live session will exercise exist with unit or fake-client
proof before KEEP. Floci, mocks, schema, and docs tracks may proceed in
parallel while they provision nothing paid.
