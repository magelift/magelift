# Plan: reference-store acceptance

- [ ] 1.1 Sweep residual block (hoist retained backups into a
  top-level `residual` entry with billing-honest framing plus
  `residualUnknown`; unit tests) — verify:
  `go test ./internal/cli/ -run TestSweep -count=1`
- [ ] 1.2 Recipe doc plus runbook skeletons (new
  `docs/alpha-recipe.md` with pins and bounds; operations
  runbook section shells marked unproved until the loop
  fills them; humanizer plus marks) — verify: `make docs`
  green, prose passes recorded
- [ ] 2.1 Live loop part 1 (ASK-GATE: account, RC tag, spend;
  install RC, build digest, initial deploy, per-surface
  verify, application release) — verify: phases 1-4 pass
  with evidence rows
- [ ] 2.2 Live loop part 2 (failed release with CLI diagnosis
  and recovery, credential-expiry ops, backup plus restore
  into a fresh env) — verify: phases 5-7 pass with
  evidence rows
- [ ] 2.3 Live loop part 3 (preview expiry with residual
  report, orphan assertion, destroy on exit, spend record;
  runbooks filled from executed commands) — verify:
  phase 8 passes, `assert_clean ok`, runbooks match the log
- [ ] 3.1 Evidence plus verdict (proof file, sealed JSONL,
  go/no-go plus tag command, pilot intake format, full
  local gates, report, archive) — verify: evidence
  complete, verdict recorded, gates green
