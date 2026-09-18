# Plan: alpha review corrections

- [x] 1.1 Release channels (R07: prerelease plus draft plus
  publish gate, installer channel split with verified API
  behavior, lockfile auth note, upgrade 404 channel,
  harness TMPDIR) — verify: actionlint, upgrade tests,
  installer harness green
- [x] 1.2 Connected installation (R01: URLs in lockfiles,
  pinned publisher, shared resolver, metadata bootstrap,
  verifier persistence, CI provider step)
  — verify: providerhost plus cli plus registry suites green
- [ ] 1.4 Clean-machine proof (R01/R12: fresh GCE micro VM
  executes the published onboarding literally end to end —
  install, provider, build, deploy, per-surface verify,
  destroy — plus project-pinned and generated-CI repeats;
  media proof includes a real upload (proves driver ACL
  plus HMAC plus prefix on a real bucket), app-relative
  image URL with zero storage hosts in storefront HTML,
  retrieval on a fresh web pod (get.php plus
  Synchronizer, not warm cache), replacement,
  export/import round trip, and the negative test
  (unauthenticated import/export paths fail; stock
  denies hold for customer/downloadable/import/
  custom_options); VM destroyed after) — needs rc.4
  artifacts; runs before the order-8 re-run
- [x] 1.3 Crash-safe updater (R11: copy backup, injectable
  transitions, fault tests, Windows note) — verify:
  `go test ./internal/upgrade/ -count=1`
- [x] 2.1 Provider boundary, partial (R05: execution
  into plugin plus SDK contracts done; import decoupling
  explicitly deferred with removal as a hard prerequisite
  to the second provider; spec, roadmap, and AWS intent
  reconciled; ADR plus leanness gate already honest) —
  verify: leanness gate plus registry/plugin suites green
- [x] 2.2 Credential wiring (R02: provider-owned factory
  default, exec path, production-constructor tests,
  stale-bearer assertions) — verify: plugin plus kube
  suites green
- [x] 2.3 Timeout policy (R10: reclassify by effect,
  assert all mutations) — verify: plugin suite green
- [x] 2.4 Real publication mechanics (R06: coherent fixtures,
  full provider compile `GOWORK=off` against synthetic
  modules) — verify: distribution suite green
- [ ] 2.5 Require-bump plus real verification (rc.11: stability-gated) (R06: bump
  requires to rc.2 after tagging, record proxy sums,
  verify `GOWORK=off` root plus provider builds) — needs
  rc.2 tags; runs with the rc.2 release
- [x] 3.1 Media reality (R03: GCS mechanism, provider
  media ops, key continuity; live-verified or changed)
  — verify: unit suites green, live proof in order 8
- [x] 3.2 Serving health (R04: serving-path probe,
  effective-config search, image scope, negative tests)
  — verify: kube plus platform suites green
- [x] 3.3 Lifecycle authority (R08: PHP-emitted sequence,
  golden plus regen target, scope docs) — verify:
  golden test green, phpunit green if php present
- [x] 4.1 CI coverage (R09: module jobs, filters, cache
  keys, promotion gate on checks) — verify: actionlint,
  gate-script failure demo
- [x] 4.2 Onboarding truth (R12: shipped-path rewrite,
  ADC, mail recipient; recipe fixture pending loop
  evidence) — verify: docs green, humanizer plus marks
- [ ] 5.1 Amendments plus gates (dated report notes,
  ROADMAP refresh, full local gates, report, archive)
  — verify: every gate green, verdict recorded
