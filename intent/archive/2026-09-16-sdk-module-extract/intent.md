---
status: done
slug: sdk-module-extract
---

# Intent: SDK module extract

## Problem

`sdk/v1` is dependency-pure but lives inside the root CLI module, so it cannot be tagged, pinned, or consumed independently. Nothing enforces its no-internal-imports boundary except discipline, and community extension authors build against the whole monolith instead of a stable versioned contract.

## Evidence

- `sdk/v1` (~8.8k LOC, 37 files) has zero external imports outside stdlib; `grep` for `github.com|golang.org|google.golang|go.` excluding `magelift` returns nothing in non-test files.
- Module path is still `github.com/magelift/magelift` (root `go.mod`); `Glob go.work*` finds no workspace; `go list -m all` is 448 modules for any consumer.
- `examples/custom-cli` builds a community binary against the monolith, not against a pinned SDK.
- `.release-please-manifest.json` is a single entry (`{ ".": "1.0.0-rc.1" }`); no SDK tag scheme exists.
- `v1-stable-cut` explicitly constrains v1 to a single CLI version with independent provider releases post-v1.

## Proposed outcome

`sdk/v1` is its own Go module with its own `go.mod`/`go.sum`, consumed by the root module through a `go.work` workspace for local development. CI has an SDK-only gate. A tag following the documented SDK scheme proves the multi-module workflow. For v1 the SDK version stays lockstep with the CLI tag (no independent SDK release yet); the change proves the mechanics without breaking the v1 single-version promise. No SDK API break; `make verify` stays green.

## Affected users and systems

Community extension authors; `sdk/v1`, root `go.mod`/`go.sum`, new `go.work`; CI (`ci.yml` path filters and gates); `examples/custom-cli`; `docs/adding-a-provider.md` and `magelift-extend` skill if import paths or pinning change; release tooling if tags change.

## Constraints

- Respect `v1-stable-cut`: SDK version == CLI version for v1; independent SDK/provider releases are post-v1. This intent proves mechanics only.
- No `sdk/v1` API break; additive changes only unless the spec justifies a break with migration.
- `sdk/v1` must keep zero external dependencies and gain zero `internal/` imports; the module split must enforce this by construction.
- Smallest split that proves the workflow; keep serial-build discipline (`GOMAXPROCS=1 GOFLAGS=-p=1`, no parallel heavy builds from IDE sessions).
- `make generate`, `make generate-check`, `cli-docs-check`, and offline gates stay green.
- If the public extension boundary changes, update the ADR and the human page together (humanizer, then remove-ai-marks).

## Out of scope

- Provider module splits (`providers/aws`, `providers/gcp`, others) or per-provider releases.
- Independent SDK releases ahead of v1; SDK API redesign.
- Subprocess extraction beyond the existing GCP proof; day-2 over RPC.
- Core import-seam removal (covered by `sever-core-imports`); taxonomy moves (covered by `provider-taxonomy-docs`).
- Toolchain migrations (Renovate, `tool` directives, coverage thresholds) unless the split forces them.

## Open questions

- Module path: DECIDED 2026-09-16 (user, Option A):
  `github.com/magelift/magelift/sdk` rooted at `sdk/`. The earlier
  `.../sdk/v1` submodule default is disproven — Go rejects `/v1` module
  suffixes (empirical: `go mod init .../foo/v1` fails, require breaks all go
  commands); see spec Design for the disproof record.
- Tag scheme: DECIDED 2026-09-16 (user): nested `sdk/vX.Y.Z` for the module
  rooted at `sdk/` (Go nested-module convention; proxy-resolvable; disjoint
  from the `v*` CLI trigger). Supersedes the `sdk/v1.0.0` vs `sdk/v1/v1.0.0`
  question.
- Does GoReleaser need any change for an SDK-only module (libraries usually need no binaries), or do tags alone suffice with `gh release create` notes?
- Does `examples/custom-cli` switch to the pinned SDK module in this change, or stay on a `replace` until the first SDK tag lands?
