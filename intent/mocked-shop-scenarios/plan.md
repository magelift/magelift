---
status: superseded
slug: mocked-shop-scenarios
spec: spec.md
---

> HISTORICAL 2026-09-16: superseded by `intent/audit.md` (F14). Retained for
> reference; not approval of the rewritten draft scope. Do not implement from
> this plan.

# Plan: mocked flagship end tests after all roadmap work (HISTORICAL)

## Files that change

Exact paths. New vs edit. One line each on what changes.

- `tests/fixtures/flagship/b2b-aws/magelift.yaml` (new): staging + production config for the Terraform-shaped shop.
- `tests/fixtures/flagship/b2b-aws/parity.md` (new): checklist mapping each inventoried Terraform concern to its Magelift owner.
- `tests/fixtures/flagship/b2b-aws/README.md` (new): fixture scope, inventoried shape, standing decisions.
- `tests/fixtures/flagship/m2-upsun/` (new): platform-shaped inputs (services, relationships, crons, routes, env) plus Fastly/Algolia/SendGrid-default/PubSub-datalake markers and README.
- `tests/flagship/scenario_test.go` (new, `flagship` build tag): both scenario suites with the mock inventory documented per step.
- `tests/flagship/hygiene_test.go` (new, `flagship` build tag): example-only domain/secret grep gate over the fixtures.

## Order of work

Build and verify order, not a task dump. Group by area, number within the group.
Each box carries the check that closes it.

- [ ] 1.1 Write the `b2b-aws` fixture (YAML, parity checklist, README) from the inventoried shape — verify: every parity row names a Magelift owner or deliberate-BYO, and `grep -rEn "example\\.(test|invalid)" tests/fixtures/flagship/b2b-aws/ | wc -l` is nonzero while no other FQDN pattern appears
- [ ] 1.2 Write the `m2-upsun` fixture (platform inputs, markers, README) from the inventoried shape — verify: importer inputs are complete per the `init --from-upsun` surface, and the same example-only grep holds
- [ ] 1.3 Add the hygiene gate failing on non-example identities or secret-like values in fixtures — verify: the gate fails on a planted violation and passes clean, `go test -tags flagship ./tests/flagship/ -run TestHygiene -count=1` exits 0
- [ ] 2.1 Implement scenario 1 (mocked staging + production deploys, parity asserts, production gates) — verify: `go test -tags flagship ./tests/flagship/ -run TestScenario1 -count=1` exits 0 with cloud credential env vars unset
- [ ] 2.2 Implement scenario 2 (importer run + asserts, mocked GCP deploy, zero-manual assert) — verify: `go test -tags flagship ./tests/flagship/ -run TestScenario2 -count=1` exits 0 with cloud credential env vars unset
- [ ] 2.3 Document the per-step mock inventory in the suite and enforce creds-unset execution — verify: the inventory names every mock boundary and the suite refuses cloud credentials when present
- [ ] 3.1 Run the `humanizer` skill, then `remove-ai-marks`, on fixture READMEs and touched human docs — verify: both passes completed and the 2.x suites still exit 0

## Risks

What could break, and the check for each.

- Runs before its prerequisites (managed email, migrate path, skills acceptance): boxes 2.x fail on missing capabilities; that is the gap mechanism working, but do not start implementation until those intents archive.
- Catalog version gaps (2.4.8, ece-tools, ElasticSuite, platform service versions): scenario asserts fail naming the catalog gap, filed back per the spec; no silent version substitution.
- Importer gaps on the Upsun shape: scenario 2 fails naming the owning migrate work; fixed there, not here.
- Mock fidelity disputes: the 2.3 inventory plus reviewer sign-off is the arbiter; no live calls to settle arguments.
- Fixture drift from the real shops: fixtures freeze at the inventoried shapes; refreshing them is a new change, not silent edits.

## Proof

The end-to-end evidence that the whole spec is met, not the per-step verifies
above. Tests, commands, or screenshots.

- `go test -tags flagship ./tests/flagship/ -count=1` exits 0 with cloud credential env vars unset.
- Parity checklist fully resolved for staging and production; `.unmapped.md` holds only the standing exclusions (Algolia, PubSub datalake); manual follow-up count is zero.
- Hygiene gate green: example-only identities across both fixtures.
