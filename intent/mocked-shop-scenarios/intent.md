---
status: draft
slug: mocked-shop-scenarios
---
# Intent: synthetic scenario foundation (mocks prove contracts, not stores)

## Problem

Nothing pins what mocked tests are allowed to claim. The previous scope mixed
private-shop-shaped requirements, multiple cloud recipes, migration
compatibility, and mocked healthy output in two scenarios, making a green run
hard to interpret. A mock can prove command routing, contracts, deterministic
failures, and configuration handling; it cannot prove Magento installed, served
static assets, searched products, or recovered its database.

## Evidence

`intent/audit.md` F14: the current intent combines too much for one verdict.
Passing mocks must not be presented as store evidence.

No original public synthetic fixtures exist: `examples/sample-shop/` is a
configuration sketch, not a runnable shop. Prior shapes referenced private
forks with no live access; fixtures must be original public work compatible
with the clean-room policy (`make check-clean-room`, `docs/provenance.md`).

## Proposed outcome

Three explicit test layers with honest claims: (1) offline protocol and
configuration tests that prove routing, contracts, deterministic failures, and
validation without credentials; (2) local Magento application tests where they
add signal without cloud; (3) bounded live acceptance of the published recipe
(owned by `reference-store-acceptance`, not this intent). Original synthetic
fixtures under `tests/fixtures/` model public default-stack shapes with
example-only identities and zero secrets. Unsupported imported versions and
services are reported, never silently substituted. Gaps fail loudly and file
back to their owning intent.

## Affected users and systems

Maintainer release confidence; the acceptance harness and fixture layout;
config validation and the importer surfaces under test (no new adapters here —
the scenarios consume capabilities owned elsewhere).

## Constraints

- Mocked execution only in this intent: Pulumi mocks, contract fakes, local
  planning. No cloud credentials, no spend, no live calls; the suite refuses
  credentials when present.
- Fixtures are original work modeled on public default-stack shapes. Nothing
  copied from private repos; no employer source, IDs, or secrets; example-only
  domains enforced by a grep gate plus review.
- Keep fixtures independent of private shop source and documents.
- Layer 1 and 2 verdicts never claim store-level proof; store proof belongs to
  bounded live acceptance on the shipped path.
- Human docs touched here go through humanizer, then remove-ai-marks.

## Out of scope

- Touching real shops, accounts, or environments.
- New adapters or importer features: gaps found here file back into their
  owning intents.
- Live Magento data-plane proof (index/query/reconnect/IAM): owned by
  `reference-store-acceptance` on the alpha recipe.
- Real-merchant production cutover procedures.

## Open questions

- Fixture home and shape: `tests/fixtures/synthetic/` with recipe-shaped dirs,
  or beside the harness? Default: `tests/fixtures/synthetic/`. Owner:
  implementer.
- Runner form: Go suite with a build tag, or script? Default: Go suite for
  deterministic asserts. Owner: implementer.
- Which importer surfaces the fixtures must cover for alpha (GCP recipe only,
  or also the migration inputs onboarding documents)? Default: alpha recipe
  plus the importer inputs onboarding supports. Owner: spec author with
  `full-deployment-coverage`.
