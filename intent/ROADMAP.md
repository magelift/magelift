# Roadmap: GCP community alpha, then the later lots

Updated: 2026-10-17.

This page is the in-repo record of the maintainer implementation plan dated
2026-10-17 (`MageLift_Plan_Implementation_Codex.md`, plan 1.0, inspected base
`3ac1b844a7117752f3a5045054587508c16dcf6c`). For lots ML-00 through ML-09, that
plan is execution authority: do not open a new spec or plan approval per
subtask. Ask first still applies to public publish, release tags, protected
or production destroy, history rewrite, and spend outside the envelope below.

The product remains a Go CLI that consumes magelift.yaml and deploys into the
user's cloud account. MageLift is not a host.

## Goal

Ship a community alpha on one GCP recipe, with documented defects allowed,
then improve from use. Milestone A does not wait on a console, a stable
plugin API, every broker, high availability, or every restore path.

1. Milestone A: one person with Docker and a prepared cloud account can
   install a published binary, configure Magento in YAML, build from
   published base images and declared patches, deploy the documented GCP
   recipe over HTTPS, read status and logs, run an app command, redeploy,
   and destroy that disposable environment without deleting foreign
   resources. They can open a reproducible issue or PR from the contributor
   docs.
2. Milestone B, after A: bounded backup and restore, managed previews, a
   CLI surface agents can rely on, a read-only local console, and AWS on
   the common contract. None of these are retroactive gates for A.

Maintenance (first-party or community), maturity (alpha or experimental, per
capability or recipe), and the last real test (commit, date, configuration,
operations, result) stay separate facts. `NOT_RUN` is a real result. Record
them in `docs/capability-matrix.md` and `docs/evidence/`. Do not add a badge
engine. Do not rewrite historical evidence conclusions. This roadmap does
not upgrade or downgrade a certified cell by itself.

## What stays

- Go, Cobra, YAML, Pulumi Automation API, the PHP build, and OCI images.
- The monorepo, the existing artifact manifest, the operations contract,
  and the signed GCP plugin. No second manifest and no second deploy loop.
- GCP as an autonomous signed provider. AWS ECS code and evidence stay.
  Experimental providers stay frozen at their recorded level.
- Provider-owned topology (ADR 0003, 0004). No shared Pulumi graph behind
  `if provider ==`.
- Secret references only. Clean-room gate `make check-clean-room`.
- A checked item in an older plan is re-verified. It is not a prompt to
  rebuild that mechanism.

## Lots

One implementation lot is active by default. A lot may land as several
small PRs. ML-06 through ML-09 stay paused until milestone A is published.
Their intent files stay draft or absent. Paused does not mean done.

| Lot | Blocks A | Intent today | Outcome |
| --- | --- | --- | --- |
| ML-00 | yes | this roadmap, `AGENTS.md`, the alpha-loop plan, release skill; Makefile and CI are the same lot but edited separately | One priority list and one local test loop |
| ML-01 | yes | [alpha-development-loop](alpha-development-loop/intent.md), accepted; plan still open | Finish the existing GCP loop: distribution, artifact, and product gates stay independent. Do not reopen the certification matrix |
| ML-02 | yes | [transparent-provider-delivery](transparent-provider-delivery/intent.md), draft | Install and provider fetch without a second plugin manager |
| ML-03 | yes | [published-build-images](published-build-images/intent.md), draft | Published base images and one deterministic Magento build |
| ML-04 | yes for QPT and hotfixes | no new intent directory; `build/` patch lifecycle | Explicit patch policy. Cloud patches stay bounded. Do not apply the same patch twice |
| ML-05 | yes | [gcp-alpha-release](gcp-alpha-release/intent.md), draft | Documented GCP recipe and the community publish of that slice |
| ML-06 | no | no intent directory | Day-2 operations, bounded backup/restore, managed previews. Paused |
| ML-07 | no | [aws-autonomous-provider](aws-autonomous-provider/intent.md), draft | Targeted AWS, then extraction. Paused. A GCP pass does not prove AWS |
| ML-08 | no | no intent directory | Machine-readable CLI, user skills, read-only local console. Paused |
| ML-09 | no | [provider-ecosystem-readiness](provider-ecosystem-readiness/intent.md), draft | A contributor can add a provider; minimal external integrations. Paused |

## Reference recipe

Magento Open Source 2.4.9 is the repository starting point. Check applicable
security patches before any public exposure. Use the latest stable toolchains and dependencies compatible with this recipe,
as requested by the maintainer. Verify compatibility and pin exact versions;
do not resolve a mutable "latest" during a build.
Confirm the tuple against Adobe's documentation before publishing a
compatibility claim. If that check disagrees with the tree, fix the recipe
and its tests only.

| Concern | Alpha choice |
| --- | --- |
| Application | Magento Open Source 2.4.9 and the evidenced patch and PHP tuple |
| Runtime | GCP GKE Autopilot, nginx/PHP-FPM |
| Database | Cloud SQL MySQL at a version checked for this Magento |
| Cache/session | Memorystore Valkey at the verified tuple version |
| Search | Existing OpenSearch path, one replica, with indexing and search actually exercised |
| Queue | Existing database queue for the alpha path. It does not cover every extension |
| Media | Private GCS through the adapter contract already in tree. Test it |
| Secrets | Secret Manager references, materialized at runtime |
| Deployment | Candidate job and migrations before the new release is activated |
| Edge | Existing HTTPS path. No new CDN or WAF to close the alpha |
| Diagnosis | Native logs, deploy state, and errors a user can act on |

A container that starts does not prove version compatibility. AWS keeps its
existing ECS architecture. Reuse the same application image only when CPU
architecture and the runtime contract allow it, and inject AWS bindings at
runtime.

The alpha does not promise arbitrary Magento, PHP, database, cache, search,
or queue combinations; GKE Standard or high availability; zero-downtime
incompatible schema changes; cross-region or cross-cloud disaster recovery;
generated CI; every day-two operation; a hard cloud-spend cap; external edge
or observability; or a stable provider API.

Security checks on an endpoint the alpha exposes stay in force. Fewer
certification obligations do not drop trust-boundary tests, secret handling,
or destroy scope.

## Local commands

Canonical commands, matching the Makefile on this branch:

| Use | Command |
| --- | --- |
| Fast Go loop | `make test-fast` |
| Community alpha local gate | `make verify-community` |
| Targeted race | `make test-race` |

`make test-fast` runs `scripts/go-test-modules.sh`: root `./...`, `sdk/` with
`GOWORK=off`, and `providers/gcp/` on the workspace, without `-race`. Root
`go test ./...` is not a substitute for the nested modules. GCP does not
require `GOWORK=off` for this gate.

`make test-race` runs `scripts/go-test-race-targeted.sh`. That is root slices
`internal/lintcoverage`, `internal/usererr`, `internal/providerhost/...`,
`internal/cli`, and `internal/platform`, plus `sdk/` and GCP `operations`
with `-race`. It does not race every root package.

`make verify-community` uses the module runner with `--community`,
which excludes root AWS, OVH, Scaleway and external-vendor suites plus their
acceptance commands. The SDK and GCP suites still run in full. It depends on
`test-community`, `test-race`, `php-test`, `lifecycle-golden-check`, `check-clean-room`,
`core-leanness`, `community-test-helpers`, and the focused `development-loop-test`.

`make verify-community` does not run `generate-check`, `cli-docs-check`,
`fmt-check`, `lint`, `license-check`, `docs`, `workflow-check`,
`test-race-full`, the acceptance harness, `local-gates`, Floci, image tests,
or live cloud targets. Those targets remain. A red test stays red. Do not
delete it, weaken the assertion, or relabel it `SKIP` to close a lot.

`make test-race-full` is `go test -race ./...` in the root module. It misses
`sdk/` and `providers/gcp/`. `make verify` runs `test-fast`, `test-race-full`,
and `sdk-test`, plus lint, docs, and the other checks in its recipe. It is
the broader suite. It is not the milestone A gate, and it is not the default
local loop, because of `test-race-full`.

`make php-test` (`scripts/php-test.sh`) uses host PHP when the extensions it
checks are present, otherwise the pinned Composer image.
`lifecycle-golden-check` uses the same PHP runner. Missing Composer dependencies
are installed from the lock through that runner; no host PHP is required with
Docker. Use `MAGELIFT_PHP_TEST_CONTAINER=1` to force the pinned environment.

Hosted CI runs the full root suite without race and targeted provider/CLI
race checks; SDK and GCP have separate jobs. Do not pin `GOMAXPROCS` or `-p`.
`GOMEMLIMIT` is 75% of available RAM.

## Live cloud

GCP is the real-cloud development path: a small recipe in an explicitly
named acceptance project. Verify identity, permissions, enabled APIs, and
the ownership prefix before mutation. Credits do not prove that every
service is covered or that the current identity is allowed to create it.
Destroy on exit by default. A retained debug cell needs an explicit option,
an owner, an expiry, an inventory, and a destroy procedure. No standing
infrastructure by implication.

A mock or dry run is not a GCP pass. If credentials or an authorized target
are missing, record `NOT_RUN` and continue local work. An HTTP 500 on a
product gate stays `FAIL` until the product is fixed.

AWS keeps the existing ECS shape. Reconfirm remaining credit before a live
run. One active AWS test stack by default. No AWS matrix on every PR.
Planning ceilings, not price quotes and not a card authorization: 50 USD for
a grouped ECS recipe test, 30 USD for parity after provider extraction,
20 USD for targeted regressions, 50 USD unplanned reserve. Planned trials
stop at 100 USD of the roughly 150 USD mentioned, after remaining credit
and in-flight spend are checked. If the next run does not fit, do not create
it. Budget alerts notify. They do not prove a hard stop at 100 or 150 USD.
Cleanup of resources already created can still cost money.

Other providers: no live spend until a later lot is actually active.
Third-party paid integrations are not an alpha gate.

## Paused, not done

The 2026-09-22 queue (orders 1 through 6 as separate spec/plan cycles, a
certification seal as the close of the development loop, and `make verify`
as the alpha's full local suite) is paused where it sits outside milestone A.
Unfinished folders were not archived. Their open requirements are not
checked off.

Still paused until the trigger is real, same as before:

| Work | Trigger |
| --- | --- |
| Generated GitHub CI | The published laptop path is green and a pilot needs team automation |
| Backup/restore and failed-release recovery | Milestone A is in use; then order by operational risk |
| Production/HA GCP profile | A pilot needs that scale and accepts the cost and evidence |
| Cloud Run runtime | After the Autopilot alpha, and only if a bounded prototype is cheaper to run or to operate |
| Stable provider contract | After GCP and AWS pilots show the freeze surface (ML-09) |
| OVH and Scaleway | After AWS, unless a funded pilot is the next provider |
| EKS and GKE Standard | A concrete workload cannot fit the recorded runtimes |
| External edge and observability | Stable origin first, then user demand |

`intent/archive/` stays immutable decision history.

## Working agreement

- One active lot. Smallest correct change. Reuse the existing contracts.
- Current code and ADRs outrank stale intent prose. This roadmap outranks
  the 2026-09-22 sequence where they differ.
- Mocks prove contracts. They do not prove live Magento.
- Human docs and public claims change with their ADR or capability source.
- Ask first for release tags, public publish, protected or production
  destroy, history rewrite, and spend outside the envelope above.
