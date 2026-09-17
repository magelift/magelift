# Spec: verified provider distribution

## Requirements

### R1 — One trust policy, four surfaces

Installer (`install.sh`), updater (`magelift upgrade`), core
plugin load (`providerhost.Load`), and provider download (new,
R6) enforce the same rule: identity and content verified before
execution, fail closed on missing or invalid material, no silent
skip when tooling or bundles are absent. The explicit development
path is build-from-source (`go build` / `go run`), which never
enters verification; no bypass flag or environment variable is
added.

Trust table (normative):

| Surface | Identity | Content | Missing material |
| --- | --- | --- | --- |
| Installer | Sigstore bundle on checksums.txt, identity pinned to release.yml @ tag | SHA-256 of the archive against verified checksums | Fail: refuse install |
| Updater | Same bundle check (already built) | Same checksum check (already built) | Fail (already built) |
| Core load | Cosign bundle per lockfile entry (already built) | Digest per lockfile entry (already built) | Fail (already built) |
| Provider download | Bundle fetched with the binary, verified before install | Digest verified before and after fetch | Fail: nothing installed |

### R2 — Publication order proved before any tag

Tag scheme is the contract's (ADR 0013): `vX.Y.Z` core,
`sdk/vX.Y.Z` SDK, `providers/<name>/vX.Y.Z` providers, lockstep
for alpha. Push order: module tags first (`sdk/`, then
`providers/`), verify proxy resolution, then `vX.Y.Z` (which
triggers `release.yml`). Real tags stay ask-first; this intent
proves the mechanism, not by pushing tags.

Proof before any tag:

- A clean-module consumer test builds a temp module with
  `GOWORK=off`, `GOPROXY` pointed at a file proxy populated
  from the working tree (synthetic versions), requiring the
  SDK and asserting `go build` succeeds with no workspace.
- The same fixture resolves the provider module the same way.
- Root `go.mod` gains no fake version: the tag-then-require
  sequence is documented (tag, bump the SDK requirement to the
  tagged version, re-run the consumer proof with the real
  version). The file-proxy test proves the mechanism today.

### R3 — Installer verifies without user tooling

`install.sh` bootstraps its own verifier: a cosign release
pinned per OS/arch with SHA-256 checksums embedded in the
script, downloaded from Sigstore releases, checksum-verified
against the in-script pin, then used for a mandatory
`verify-blob` of `checksums.txt` against the pinned
release.yml identity. Bundle download failure, checksum
mismatch, or verification failure aborts the install. The
trust root stays the script content itself (served over TLS);
the script documents the pin rotation procedure (new cosign
release: update version plus checksums together, never one
alone). A shell harness test drives the installer against a
local fixture server covering success, tampered archive,
tampered checksums, and missing bundle.

### R4 — Interrupted updates recover

`upgrade Install` keeps the current binary as a same-directory
backup, atomically swaps in the verified replacement, executes
`version` on the new binary as a self-check, restores the
backup on self-check failure, and removes the backup on
success. A stale backup from a killed run is replaced, never
executed.

### R5 — CI uses verified release binaries

Generated workflows install MageLift from release archives:
download the pinned-version archive plus `checksums.txt` and
its bundle, `verify-blob` via the cosign-installer action
(already used by the build job; added to the validate job),
checksum-check, extract to PATH. No `go install`, and no
`setup-go` step that exists only to serve the install. The
version pin stays a full release tag.

### R6 — YAML-driven provider download

Which provider to fetch derives from the project's
`magelift.yaml` target plus `magelift.providers.lock`, never
from CLI flags. New `magelift providers install` resolves the
entry for the configured provider, downloads binary and bundle
when the lockfile names remote locations, verifies via the
existing `VerifyLocal`, and atomically installs into the user
cache. URL semantics (backward compatible):

- `Artifact.URL` empty: binary expected beside the CLI
  (release bundles ship together; current behavior kept).
- `Artifact.URL` https: binary downloaded from that URL.
- `Cosign.Bundle` relative: resolved beside the lockfile
  (current behavior kept).
- `Cosign.Bundle` https: bundle downloaded from that URL.

Cache layout: `~/.magelift/providers/<id>/<version>/<binary>`
plus the bundle beside it (`os.UserCacheDir` honors
`XDG_CACHE_HOME` on POSIX and `LocalAppData` on Windows).
Load order: beside-CLI first, then cache. Every load runs the
existing full verification (digest plus bundle) on whichever
copy loads: uniform and stronger than digest-only cache
checks, at the same per-load cost the beside-CLI path
already pays. Compatibility stays on the existing checks
(lockfile schema/API/protocol plus Describe negotiation).

### R7 — The release pipeline ships the sequence

Fix what is broken today:

- Both GoReleaser configs build the provider from the deleted
  `./cmd/magelift-provider-gcp`; they must build
  `providers/gcp/cmd/magelift-provider-gcp` from its nested
  module (per-build `dir`).
- `release.yml` generates lockfiles with schema 1 and no
  protocol field, which the CLI refuses. Generation moves to a
  Go generator (`cmd/genproviders`, same convention as
  `genconfig`/`gendocs`), emitting schema 2 with the
  `magelift-v2` protocol marker; `release.yml` calls it.
  The generator has unit tests over schema, protocol, digest
  format, and bundle naming.
- The local release smoke covers the provider binary (already
  asserted) and fails visibly while the main path is wrong.

No live tag is pushed by this intent; end-to-end proof with a
real tag stays behind the ask-first gate (Order 8 runs the
release candidate).

### R8 — Docs say binary-first

`docs/install.md` rewritten: installer verification story,
upgrade, CI binaries, provider download and cache, the
build-from-source dev path, and what each failure looks like.
Humanizer plus marks before commit.

## Design notes

- New code: `internal/providerhost/download.go` (fetch +
  cache + atomic install), `internal/cli/providers.go`
  (`providers install`), `cmd/genproviders` (lockfile
  generator), `tests/distribution/` (file-proxy consumer
  proof), installer harness under `scripts/`.
- Changed: `website/public/install.sh`, `internal/upgrade`
  (backup + self-check), `internal/cli/ci.go` plus its tests,
  `.goreleaser.yaml`, `.goreleaser.dialproof.yaml`,
  `.github/workflows/release.yml`, `docs/install.md`, new ADR
  0014 recording the trust table.
- No protocol, provider-implementation, or recipe changes.
- Windows cache path and atomic replace covered by design
  (same-dir temp plus rename); POSIX plus Windows paths
  tested where the harness runs (POSIX here, Windows by
  inspection plus `GOOS=windows go build`).
