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
