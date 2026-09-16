---
status: deferred
slug: v1-stable-cut
---
# Intent: stable cut (DEFERRED past alpha)

## Problem

DEFERRED 2026-09-16 by `intent/audit.md`: the architecture is not ready to
freeze. The provider boundary is not achieved (compiled provider
registrations, core-owned provider knowledge, narrow subprocess proof,
installation and module-publication gaps), deployment health and lifecycle
semantics need repair, and evidence integrity plus cost-claim honesty need a
trust baseline first. Tagging a stable v1 now would freeze contracts the alpha
plus pilot feedback plus AWS parity are meant to validate.

The original problem stands for later: without a tagged, installable CLI,
adoption stays theoretical. But the next tag is a narrowly supported public
alpha (`v0.1.0-alpha.1`, exact naming subject to the release decision), not a
stable v1 or an RC implying near-frozen contracts. No tag is authorized by the
audit or this intent.

## Evidence

`intent/audit.md` F01–F03, F07, F11: provider independence, subprocess
enforcement, SDK porosity, workspace-masked distribution, and skippable
first-install verification block any API freeze.

Prior `report.md` in this directory: PENDING (CI run outstanding, contracts
pending, shellcheck known-red on `wip/all-local-work`, tag ref stale). Retained
as historical status, not approval of the deferred scope.

`docs/versioning.md` still names `v1.0.0-rc.1` as the first public tag; that
document is updated when the alpha decision lands, not before.

## Proposed outcome

After alpha pilots and AWS parity: a stability decision with user evidence
(agencies diagnose failures from CLI output, recover without maintainer-only
commands, understand cost and responsibilities), then a tagged, installable,
checksummed release with SBOM, provenance, and verified upgrade. The freeze
surface is defined then, from contracts the alpha proved.

## Affected users and systems

Every prospective user. Release tooling, install docs, website install pages,
upgrade path, versioning policy.

## Constraints

- No stable tag until `reference-store-acceptance` passes on the shipped
  alpha path, pilot feedback is in, and `aws-provider-parity` completes the
  same public protocol.
- Single-version discipline holds through alpha; independent provider release
  lifecycles are designed in `provider-plugin-contract` and proved from
  `gcp-autonomous-provider` onward — the stable cut does not redesign them.
- Ask-first: release tags, protected/production destroy, history rewrite.

## Out of scope

- The alpha tag itself: owned by `reference-store-acceptance` plus the
  maintainer's explicit release call.
- New providers, new catalog cells, community catalog hosting.
- Windows package managers beyond archive download.

## Open questions

- Which contracts the alpha proves stable enough to freeze (YAML envelope,
  CLI verbs, exit codes, provider IDs, lockfile schema, RPC operations)?
  Owner: maintainer at the post-alpha stability review.
- Homebrew/Scoop timing relative to the stable tag. Owner: maintainer.
