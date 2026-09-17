---
status: accepted
slug: verified-provider-distribution
accepted: 2026-09-16
acceptance: defaults adopted (contract tag scheme proved end to end;
  installer verifies without user-installed signing tooling;
  cache plus atomic-replace specified with per-path tests);
  no live cloud, local registries/proxies prove the sequence
---
# Intent: verified provider distribution (install you can trust)

## Problem

Workspace success masks distribution work, and first-install verification can
be skipped. The SDK resolves inside `go.work` but not as a published consumer
dependency; generated CI compiles the broad toolchain via versioned `go
install` instead of using release binaries; and the installer verifies
strongly only when Cosign happens to be present with a retrievable bundle.
None of that is a binary-first experience a pilot team can trust.

## Evidence

`intent/audit.md` F07: `sdk/go.mod` is separate and dependency-free, but the
root `go.mod` lacks the consumer requirement needed outside the workspace;
`GOWORK=off GOPROXY=off GOSUMDB=off GOFLAGS=-mod=readonly go list
github.com/magelift/magelift/sdk` fails to resolve. The archived extraction
report defers publication — not evidence of a working external consumer
today. Generated CI in `internal/cli/ci.go` uses versioned `go install`,
compiling the broad toolchain in the customer job.

F11: `website/public/install.sh` runs Cosign verification only when `cosign`
exists on the machine AND the bundle download succeeds; otherwise only a
checksum from the same untrusted distribution path is checked. The updater's
negative signature tests and signed pipeline do not prove the initial
installer enforces the same policy.

F02 distribution half: no complete YAML-driven provider download path exists.

## Proposed outcome

One trust policy for installer, updater, core, and plugins: identity and
content verified before execution, failing closed on invalid or missing
required material, with an explicit development path kept separate. Publication
order and consumer resolution are proved before any tag: an SDK consumer and a
provider build from clean modules with `GOWORK=off`, local registries or
module proxies prove the sequence before authorized public publication, and
the supported install plus CI paths use verified release binaries — never a
customer-side toolchain compile. Provider download is YAML-driven with
checksum, signature, digest, and compatibility enforcement. Atomic install,
cache integrity, compatibility checks, and interrupted-update recovery are
defined and tested. The user never needs to understand signing infrastructure.

## Affected users and systems

Every installer and CI user. `sdk/` publication, provider module publication,
`website/public/install.sh`, `magelift upgrade`, generated CI in
`internal/cli/ci.go`, provider download and lockfile handling, release
workflows (GoReleaser, Cosign, SLSA, SBOM), install docs.

## Constraints

- Verify identity and content before execution; fail closed. No silent skip
  when tooling or bundles are missing.
- Prove with `GOWORK=off` clean-module consumers before any public tag.
- Supported install and CI paths use verified release binaries.
- No secret values in logs or evidence during download/verify/install.
- Ask-first: release tags and any public publication step.
- Human docs and website copy go through humanizer, then remove-ai-marks.

## Out of scope

- The provider protocol itself (owned by `provider-plugin-contract`).
- Provider implementation (owned by `gcp-autonomous-provider`).
- The alpha recipe and onboarding (owned by `full-deployment-coverage`).
- Community catalog hosting; per-provider independent release cadence beyond
  what the contract specifies for alpha.

## Open questions

- Publication order and tag scheme for SDK plus provider modules (nested tags,
  proxy resolution, release notes)? Default: the contract's scheme, proved
  here end to end. Owner: spec author with the contract author.
- Installer behavior when Cosign is absent: bundle verification via a
  statically linked verifier, or a hard dependency with a clear error?
  Default: verify without requiring the user to install signing tooling.
  Owner: spec author.
- Cache layout and atomic-replace mechanics for core plus providers on all
  supported platforms (POSIX plus Windows)? Default: specified with tests on
  each path. Owner: spec author.
