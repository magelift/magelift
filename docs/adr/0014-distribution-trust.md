# ADR 0014: Distribution trust (one fail-closed policy)

- Status: Accepted
- Date: 2026-09-17

## Context

The installer verified provenance only when Cosign happened to be present
with a retrievable bundle; otherwise a checksum from the same untrusted
path was the whole story (`intent/audit.md` F11). Generated CI compiled
the toolchain in the customer job instead of using release binaries, and
the SDK resolved inside `go.work` but not as a published dependency (F07).
No complete provider download path existed (F02 distribution half). The
release pipeline built the provider from a deleted main path and emitted
lockfiles the loader refuses.

## Decision

One trust policy over four surfaces: identity and content verified before
execution, fail closed on missing or invalid material, no silent skip.

- Installer: bootstraps a pinned Cosign release (checksums embedded in the
  script), requires the Sigstore bundle on `checksums.txt`, verifies
  archive SHA-256. Trust root is the script content itself.
- Updater: verifies bundle plus checksum (as before), keeps a same-directory
  backup, swaps atomically, self-checks the new binary, restores on failure.
- Core load: digest plus bundle per lockfile entry, beside the CLI or from
  the user cache (`~/.magelift/providers/<id>/<version>/`).
- Provider download: `magelift providers install` resolves providers from
  `magelift.yaml` (never flags), fetches over HTTPS-only URLs, verifies
  before atomic cache install.

Tags are `vX.Y.Z` core, `sdk/vX.Y.Z` SDK, `providers/<name>/vX.Y.Z`
providers (ADR 0013), lockstep for alpha. Module tags push first, proxy
resolution is confirmed, the release tag pushes last and triggers the
signed release. The development path is build-from-source; no bypass flag
or variable exists.

## Consequences

- First install needs no signing tooling and skips nothing.
- CI consumes verified release binaries; customer repos need no Go toolchain.
- `go mod tidy` at root fails pre-tag by design (unpublished modules have
  no resolvable versions); the tag-then-require sequence clears it.
- Release lockfiles always load: the generator shares the loader's validation.

## Alternatives

- Require user-installed Cosign for install: rejected (first-install
  friction, version drift; the pinned bootstrap is deterministic).
- In-process Sigstore verification via Go libraries: rejected for alpha
  (large new dependency surface; the exec boundary stays explicit and
  fail-closed).
- Per-surface policies: rejected (four chances to drift; one table to audit).

## Provenance

`intent/audit.md` F02/F07/F11, `intent/verified-provider-distribution/spec.md`,
`website/public/install.sh`, `internal/upgrade`, `internal/providerhost`,
`internal/cli/ci.go`, `cmd/genproviders`, `tests/distribution`, ADRs
0005/0008/0011/0013.
