---
status: done
slug: mocked-shop-scenarios
spec: spec.md
---

# Plan: synthetic scenario foundation (mocks prove contracts, not stores)

## Files that change

Exact paths. New vs edit. One line each on what changes.

- `tests/fixtures/synthetic/README.md` (new): fixture scope, originality statement, example-only rule, layer verdict limits.
- `tests/fixtures/synthetic/gcp-preview/magelift.yaml` (new): alpha-recipe-shaped GCP Autopilot preview config.
- `tests/fixtures/synthetic/aws-preview/magelift.yaml` (new): AWS Fargate preview shape for routing coverage.
- `tests/fixtures/synthetic/invalid/unknown-provider.yaml` (new): expects the unknown-provider failure class.
- `tests/fixtures/synthetic/invalid/unsupported-version.yaml` (new): expects the unsupported-version failure class.
- `tests/fixtures/synthetic/invalid/missing-required.yaml` (new): expects the missing-required-field failure class.
- `tests/fixtures/synthetic/invalid/expired-preview.yaml` (new): past-`expiresAt` preview expecting deploy refusal.
- `tests/fixtures/synthetic/imports/acc/.magento.app.yaml` (new): synthetic ACC app input with mappable plus unmapped keys.
- `tests/fixtures/synthetic/imports/acc/.magento/services.yaml` (new): synthetic ACC services input.
- `tests/fixtures/synthetic/imports/acc/.magento/routes.yaml` (new): synthetic ACC routes input.
- `tests/fixtures/synthetic/imports/acc/.magento.env.yaml` (new): synthetic ACC env input.
- `tests/fixtures/synthetic/imports/upsun/.platform.app.yaml` (new): synthetic Upsun app input with mappable plus unmapped keys.
- `tests/fixtures/synthetic/imports/upsun/.platform/services.yaml` (new): synthetic Upsun services input.
- `tests/fixtures/synthetic/imports/upsun/.platform/routes.yaml` (new): synthetic Upsun routes input.
- `tests/fixtures/synthetic/imports/upsun/.platform.env.yaml` (new): synthetic Upsun env input.
- `tests/synthetic/suite_test.go` (new, `synthetic` build tag): layers 1+2 — routing, contracts, failures, validation, importer, local planning, credential refusal, gaps table, mock inventory.
- `tests/synthetic/hygiene_test.go` (new, `synthetic` build tag): example-only plus no-secrets scan over fixtures with inline self-test samples.
- `tests/README.md` (edit): one Layer-table row for the synthetic offline suite.

## Order of work

Build and verify order, not a task dump. Group by area, number within the group.
Each box carries the check that closes it.

- [x] 1.1 Write the two valid recipe fixtures plus README (original content, example-only identities, zero secrets) — verify: both YAMLs load via `config.Load` and validate clean, and the hygiene gate (box 1.4) passes them
- [x] 1.2 Write the four invalid fixtures, each expecting one classified failure — verify: each fails load/validate/plan with its documented error class and none passes by substitution
- [x] 1.3 Write the synthetic ACC/Upsun importer inputs (mappable surface plus explicit unmapped markers) — verify: `magelift init --from-acc` / `--from-upsun` exit 0 on them, emitted YAML validates, unmapped keys are listed, unsupported versions error
- [x] 1.4 Add the hygiene gate (example-only identities, no secret-like values) with inline pass/fail self-test samples — verify: the gate fails on the planted bad sample, passes the good sample and the real tree, `go test -tags synthetic ./tests/synthetic/ -run TestHygiene -count=1` exits 0
- [x] 2.1 Implement layer 1 (routing dispatch to fake modules, descriptor plus plan contract handling, deterministic failures over fixtures, credential refusal denylist, gaps table plus mock inventory) — verify: `go test -tags synthetic ./tests/synthetic/ -run 'TestRouting|TestContract|TestFailure|TestValidation|TestNoCredentials|TestGaps' -count=1` exits 0 with credential env vars unset, and exits nonzero with one set
- [x] 2.2 Implement layer 2 (localdev plan plus compose template asserts over the GCP preview fixture, no containers) — verify: `go test -tags synthetic ./tests/synthetic/ -run TestLocalPlanning -count=1` exits 0 and asserts image, env wiring, queue/session/email shaping
- [x] 2.3 Enforce creds-unset execution and document the mock inventory per step — verify: the suite refuses each denylisted variable family at startup (AWS/GCP/OVH/Scaleway/Pulumi/harness markers) and the inventory names every real-vs-fake boundary
- [x] 3.1 Add the `tests/README.md` suite row — verify: the Layer table names the suite, tag, command, and no-cloud status
- [x] 3.2 Run the `humanizer` skill, then `remove-ai-marks`, on fixture READMEs and touched human docs — verify: both passes completed and the full `go test -tags synthetic ./tests/synthetic/ -count=1` still exits 0

## Risks

What could break, and the check for each.

- Fixture YAML drifts from the validating schema (new required keys): boxes 1.1–1.2 fail on load/validate errors naming the drift; fixtures track the schema, not vice versa.
- Importer surface gaps (synthetic inputs the importer cannot parse): box 1.3 fails naming the gap, filed back per the spec; no importer features built here.
- Routing seams differ from the plan's assumed call sites (registry dispatch, stack plan refusal): box 2.1 fails on compile or behavior mismatch; rewire to the real seams in the same change (plan updated if the file list moves).
- Catalog version gaps (fixture pins outside the catalog): validation asserts fail naming the catalog gap; filed back, no silent substitution.
- Credential denylist misses a real credential path (ambient ADC, config files): box 2.3 tests the documented families; ambient-credential sandboxing beyond env vars is explicitly out of scope and recorded in the report.
- Mock fidelity disputes: the 2.3 inventory plus reviewer sign-off is the arbiter; no live calls to settle arguments.

## Proof

The end-to-end evidence that the whole spec is met, not the per-step verifies
above. Tests, commands, or screenshots.

- `go test -tags synthetic ./tests/synthetic/ -count=1` exits 0 with cloud credential env vars unset.
- Same command exits nonzero with a denylisted variable set.
- Hygiene gate green: example-only identities across all fixtures.
- No line in suite output or fixture README claims store-level proof.
