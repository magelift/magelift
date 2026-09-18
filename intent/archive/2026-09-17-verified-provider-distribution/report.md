# Report: verified provider distribution

verdict: pass

## What shipped

- One fail-closed trust policy over installer, updater,
  core load, and provider download (ADR 0014).
- `magelift providers install`: YAML-driven provider
  download into the user cache, HTTPS-only, digest plus
  Cosign verified before atomic install; load falls back
  to the cache and names the command only when the binary
  is missing everywhere.
- Updater recovery: same-dir backup, atomic swap, version
  self-check, restore on failure, stale-backup replace.
- Installer bootstraps pinned Cosign and requires the
  Sigstore bundle; 14-check fixture harness proves every
  fail-closed path.
- Generated CI installs verified release binaries; no
  `go install`, no install-only `setup-go`.
- Release pipeline fixed: nested provider build with
  version stamp, Go lockfile generator emitting schema 2
  plus protocol, snapshot smoke asserting the provider
  matrix.
- Publication proof before any tag: file-proxy
  `GOWORK=off` SDK consumer build plus provider
  proxy-servability, fully offline; tag sequence
  documented.
- install.md rewritten binary-first.

## Evidence

- Root suites: 107 packages ok, zero failures.
- Provider plus SDK suites: ok. Synthetic: 13 passed.
  Distribution: 2 passed.
- skills-test, installer harness (14/14),
  acceptance-harness-test: green.
- generate-check, cli-docs-check, fmt-check, lint
  (0 issues), sdk-test, license-check, check-clean-room,
  workflow-check (actionlint), docs build: green.
- release-smoke green (snapshot release, provider binary
  plus checksums entry asserted).

## Deviations

- Spec R6 amended during implementation: every load runs
  full digest-plus-bundle verification (uniform, stronger)
  instead of digest-only cache checks.
- No live tag pushed (ask-first gate holds); end-to-end
  proof with a real tag belongs to Order 8.
- Root `go mod tidy` fails pre-tag by design
  (unpublished modules unresolvable); the documented
  tag-then-require sequence clears it.

## Follow-ups

None. Next: Order 8 reference-store-acceptance.

## Correction (2026-09-17, alpha review R01/R06/R07/R11)

The components existed but were not connected. R01: one shared
resolver (project locks control the cached version), pinned
first-party publisher, absolute lockfile URLs, release-metadata
bootstrap writing a reviewable project pin, persisted verifier, and
CI provider steps; the clean-machine proof runs against rc.2
artifacts. R06: coherent proxy fixtures plus a real GOWORK=off
provider compile; first require-bump at rc.2. R07: draft plus
prerelease plus never-latest channels with a checks-gated publish
step (dialproof.6 metadata corrected). R11: copy-based upgrade
backup with per-transition fault tests. Fixed in
intent/alpha-review-corrections (boxes 1.1, 1.2, 1.3, 2.4, 2.5).

## Correction (2026-09-18, alpha review 5.1)

R07 publish gate behaved as designed: draft rc.15 assets exist; publish
refused because CI acceptance failed (`rg` on a jq-only image). The
harness now uses `grep -E`. CloudWatch, S3 recovery, and Secrets
recovery dry-run without a region or `aws configure`. Go jobs set
`GOMEMLIMIT` to 75% of available RAM and do not pin `GOMAXPROCS` or
`-p`. Hosted force-all CI is green on `d5f781a`
([35382767735](https://github.com/magelift/magelift/actions/runs/35382767735)).
Box 1.4 still needs the next published (non-draft) RC; the first
require-bump was rc.2, and later RCs repeat the same staging
sequence. The order-8 re-run waits for that published candidate,
not rc.2.
