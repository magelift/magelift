---
status: specified
slug: v1-stable-cut
intent: intent.md
half: engineering (tag plus doc flips stay order 14)
---

# Spec: v1-stable-cut engineering half

Auto-approved per the standing `/goal` instruction. This cycle covers
roadmap order 7 only: release wiring verified locally, no tag, no
README or install-doc flip. Defaults taken:

- Homebrew cask publishes at the tag, with a macOS cask-install job
  in the release pipeline as the tested path (publish-then-verify in
  the same workflow beats an untested tap file; a failure fails the
  workflow visibly for fix-forward).
- `magelift upgrade` replaces the CLI only for v1; provider skew
  gets a visible warning, and full co-upgrade is post-v1 (single
  version keeps a stale provider functional, the warning keeps it
  honest).

## Requirements

### Requirement: upgrade works on Windows

`upgrade` SHALL request the `.zip` asset on Windows (matching the
GoReleaser `format_overrides`) and extract the binary from zip as
well as tar.gz.

#### Scenario: windows asset round trip

- **WHEN** installing a fake Windows release
- **THEN** the client downloads the `.zip` asset and extracts
  `magelift.exe` after checksum verification

### Requirement: provider skew warns

When the installed provider lockfile version differs from the CLI
version, the subprocess path SHALL warn on stderr naming both
versions instead of running silently stale.

#### Scenario: stale provider warns

- **WHEN** loading a provider whose lock version differs from the
  CLI version
- **THEN** a skew warning names both versions and the load proceeds

### Requirement: smoke covers the provider binary

`scripts/release-smoke-local.sh` SHALL assert the provider binary
builds beside the CLI and fail when it is missing.

#### Scenario: smoke pins both binaries

- **WHEN** `make release-smoke` runs
- **THEN** it reports both `magelift` and
  `magelift-provider-gcp` present in `dist/`

### Requirement: cask install verified in-pipeline

`release.yml` SHALL gain a macOS job that taps the published cask
after release and installs it, running `magelift version`.

#### Scenario: cask job validates

- **WHEN** `make workflow-check` runs
- **THEN** the release workflow passes actionlint with the new job

### Requirement: records current

The packaging smoke record, license-check record, and launch
checklist SHALL reflect fresh local runs, and the install doc's
post-tag section SHALL match the actual GoReleaser asset names.

#### Scenario: records match runs

- **WHEN** a reader opens the readiness and install docs
- **THEN** smoke and license records carry this cycle's dates and
  the documented asset names match the release config

## Design

- `internal/upgrade`: platform asset suffix plus zip extraction
  branch; tests craft tar.gz and zip fixtures with a fake HTTP
  server (existing test style).
- `internal/cli/subprocess.go`: version compare at load, warning to
  `o.stderr`; CLI `Version` is the reference.
- Smoke script: `find` for the provider binary, same failure style.
- `release.yml`: new `cask-verify` job, `needs: [release]`,
  `runs-on: macos-latest`, tap plus install plus version.
- Docs: `release-readiness.md` records, `install.md` asset-name
  audit (fix only if wrong), `post-beta-roadmap.md` co-upgrade row.

## Gotchas / policy flags

- No tag, no Releases flip, no README change in this cycle.
- Never log secret values; the smoke and tests use fake tokens.
- The cask job cannot run until a tag exists; actionlint plus a
  dry workflow read is the local proof.

## Open questions carried forward

- Gate-board sign-off owners stay open until order 14 (tag cycle).
- Full provider co-upgrade is post-v1; the warning is the v1 cover.
