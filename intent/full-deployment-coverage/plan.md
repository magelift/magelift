---
status: superseded
slug: full-deployment-coverage
spec: spec.md
---

> HISTORICAL 2026-09-16: superseded by `intent/audit.md` (F13). Retained for
> reference; not approval of the rewritten draft scope. Do not implement from
> this plan.

# Plan: every resource Magento needs, managed by Magelift (HISTORICAL)

## Files that change

Exact paths. New vs edit. One line each on what changes.

- `intent/full-deployment-coverage/audit.md` (new): triaged audit of every manual deployment-path item.
- `internal/config/model.go`, `internal/config/config.go`, `internal/config/config_test.go` (edit): `tem`/`ovh` modes plus managed-provisioning fields and validation.
- `internal/cloud/aws/email/` (new): SES identity, DKIM records, SMTP credentials, Magento wiring.
- `internal/cloud/scaleway/tem/` (new): TEM domain, validation, SMTP credentials, Magento wiring.
- `internal/cloud/ovh/email/` (new): MX Plan mailbox account, Magento wiring. Exact homes follow the order-16 taxonomy at implementation time.
- `internal/cloud/aws/*_test.go`, `internal/cloud/scaleway/*_test.go`, `internal/cloud/ovh/*_test.go` (new): mock-graph tests per adapter.
- `schema/magelift.schema.json`, `docs/configuration.md` (regenerate only via `make generate`).
- `docs/capability-matrix.md`, `docs/operations.md` (edit): managed cells, limits, remaining manual procedures.
- `agents/skills/magelift-configure/SKILL.md`, `agents/skills/magelift-operate/SKILL.md` (edit): teach managed email.
- `docs/evidence/` (new): one live cell per managed adapter (SES, TEM, OVH mailbox).

## Order of work

Build and verify order, not a task dump. Group by area, number within the group.
Each box carries the check that closes it.

- [ ] 1.1 Write `audit.md`: disposition every researcher-found item as deliberate-BYO (with deciding doc + procedure) or gap-to-close (covered here or follow-up stub) — verify: zero items without a disposition and every stub names a slug
- [ ] 2.1 Add `tem`/`ovh` modes plus managed-provisioning fields to config with validation (fr-par gate, quota surfacing, secret-ref rules) — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/config/ -count=1` exits 0
- [ ] 2.2 Regenerate schema and configuration reference — verify: `make generate-check` exits 0
- [ ] 3.1 Implement the AWS SES adapter (identity, DKIM into the supplied zone, SMTP credentials to a managed secret, Magento env wiring) with mock-graph tests — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/email/ -count=1` exits 0
- [ ] 3.2 Implement the Scaleway TEM adapter (domain, validation, credentials, wiring) with mock-graph tests — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/scaleway/tem/ -count=1` exits 0
- [ ] 3.3 Implement the OVH mailbox adapter (account on the supplied domain, wiring) with mock-graph tests — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/ovh/email/ -count=1` exits 0
- [ ] 4.1 Update the capability matrix, operations procedures, and the two user skills; record sandbox/quota/tier limits — verify: `make docs` exits 0 and `grep -F "managed" docs/capability-matrix.md` shows the three new cells
- [ ] 4.2 Run the `humanizer` skill, then `remove-ai-marks`, on touched human docs and skill prose — verify: both passes completed and `make generate-check` still exits 0
- [ ] 5.1 Live SES managed-send cell with evidence ( light smoke, destroy on exit) — verify: evidence file committed and matrix row cites it
- [ ] 5.2 Live TEM managed-send cell with evidence (fr-par, Essential tier, destroy on exit) — verify: evidence file committed and matrix row cites it
- [ ] 5.3 Live OVH mailbox managed-send cell with evidence (existing MX Plan domain, destroy mailbox on exit) — verify: evidence file committed and matrix row cites it

## Risks

What could break, and the check for each.

- Runs before order 16: adapter homes are guesses until taxonomy lands; do not start boxes 3.x until `provider-taxonomy-docs` is archived, and re-home on drift.
- SES sandbox blocks first sends: box 5.1 needs an exited-sandbox identity or records sandbox as the explicit manual step; never fake the send.
- OVH provider coverage gap (domain service creation): box 3.3 proves accounts-only against the pinned provider version; if the service itself is creatable, the spec default upgrades.
- TEM tier surprise (Essential limits vs Scale cost): box 5.2 runs Essential and records actual limits; Scale stays a later change.
- Secret sprawl: Pulumi-created credentials use the same secret-ref validation as operator ones; box 2.1 tests refuse plaintext.
- Live-cell spend across three providers: each 5.x box is destroy-on-exit with its own cap line in the evidence file.

## Proof

The end-to-end evidence that the whole spec is met, not the per-step verifies
above. Tests, commands, or screenshots.

- `intent/full-deployment-coverage/audit.md` shows zero untriaged items.
- Config, AWS email, Scaleway TEM, and OVH email suites all exit 0; `make generate-check` and `make docs` exit 0.
- Three evidence files (SES, TEM, OVH mailbox managed sends) cited from the capability matrix.
- `magelift skills list` and the drift guard unchanged (no new user skill; configure/operate cover managed email).
