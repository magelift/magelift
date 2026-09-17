# Plan: verified provider distribution

- [x] 1.1 Provider download plus cache plus install command
  (`internal/providerhost/download.go`, `internal/cli/providers.go`,
  URL semantics, atomic cache install, digest re-check on load;
  httptest tamper/missing/compat tests) — verify:
  `go test ./internal/providerhost/ ./internal/cli/ -count=1`
- [x] 1.2 Updater recovery (same-dir backup, version self-check,
  restore on failure, stale-backup replace) — verify:
  `go test ./internal/upgrade/ -count=1`
- [x] 2.1 Installer fail-closed rewrite plus harness
  (pinned cosign bootstrap, mandatory bundle, fixture-server
  success/tamper/missing tests) — verify: harness green,
  `shellcheck` clean on the touched scripts
- [x] 2.2 CI templates to verified binaries (download plus
  verify-blob plus checksum in validate/build/ENV jobs, drop
  install-only setup-go; update `ci_test.go` expectations) —
  verify: `go test ./internal/cli/ -run TestCI -count=1`
- [x] 3.1 Release pipeline ships the sequence (GoReleaser nested
  provider main, Go lockfile generator plus unit tests,
  release.yml wiring, smoke green) — verify:
  `make release-smoke` green, generator tests green
- [x] 3.2 Publication proof before any tag (file-proxy
  `GOWORK=off` SDK consumer plus provider resolve test under
  `tests/distribution/`, tag-then-require sequence documented) —
  verify: `go test ./tests/distribution/ -count=1`
- [x] 4.1 Trust docs (ADR 0014, install.md rewrite, pointers;
  humanizer plus marks) — verify: `make docs` green, prose
  passes recorded
- [x] 4.2 Full proof (root, SDK, provider, synthetic, harness,
  skills, generate, docs, lint, clean-room gates; report plus
  archive) — verify: every gate named green, report verdict
  recorded
