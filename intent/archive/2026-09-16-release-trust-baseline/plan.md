---
status: done
slug: release-trust-baseline
spec: spec.md
---

# Plan: trust baseline before any release promise

## Files that change

Exact paths. New vs edit. One line each on what changes.

- `docs/evidence/runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl` (edit): 13 stale `recordDigest` values replaced by reseal; content bytes otherwise identical.
- `docs/evidence/README.md` (edit): new Provenance notes section voiding the `77ae916` seal and recording corroboration plus reseal tool/date.
- `docs/evidence/current-capability-coverage.md` (regenerate via generator only): reflects the resealed bundle.
- `docs/capability-matrix.md` (edit): exact proved tuples per certified row, operator deploy-guard scoping, GCP app-level vs AWS infra-only search distinction, openspec ref repair.
- `docs/aws-acceptance.md` (edit): catalog-ownership openspec ref repair; expiry honesty pass.
- `docs/gcp-acceptance.md` (edit): catalog-ownership openspec ref repair; expiry honesty pass.
- `docs/ovh-experimental.md` (edit): catalog-ownership openspec ref repair; expiry honesty pass.
- `docs/scaleway-experimental.md` (edit): catalog-ownership openspec ref repair; expiry honesty pass.
- `docs/release-readiness.md` (verify-only, zero edits): its sole openspec mention is the dated 2026-08-07 snapshot sentence (a true point-in-time record), kept intact like historical evidence.
- `docs/bootstrap.md` (edit): expiry honesty pass.
- `docs/operations.md` (edit): expiry/cost honesty pass.
- `website/src/data/content.js` (edit): reword `destroyed · spend stopped` and the preview `budget cap` caption.
- `docs/architecture.md` (edit): writable-root alignment, Varnish VSM path correction, alpha minimum-review record.
- `docs/evidence/*.md` under `docs/evidence/` (verify-only, zero edits): historical point-in-time records stay byte-identical.

## Order of work

Build and verify order, not a task dump. Group by area, number within the group.
Each box carries the check that closes it.

- [x] 1.1 Void and reseal the `mldp8` bundle (strip `recordDigest` per line to throwaway candidates, `magelift certification seal`, publish over the sealed path) — verify: `go run ./cmd/gencertdocs --check` exits 0 and `git diff --stat` on the sealed file shows content lines unchanged except `recordDigest` values
- [x] 1.2 Record seal provenance in `docs/evidence/README.md` (voided commit, defect, corroboration, reseal date/tool, new-attestation statement) — verify: the note names `77ae916`, the 13/13 defect, the human-doc corroboration (artifact digest, seed), and the void sentence
- [x] 2.1 Regenerate the coverage view through the supported generator — verify: `make certification-docs` (or wrapped generator) exits 0 and a second run produces zero diff
- [x] 3.1 Tighten matrix exact-proof wording (tuples, deploy-guard scope, search distinction, no blanket claims) and repair its openspec ref — verify: each certified row names its tuple plus proof link (sealed run where one exists, evidence doc otherwise); the deploy-guard sentence is present; GCP/AWS search rows differ as specified; `grep -ni openspec docs/capability-matrix.md` exits 1; `go test ./internal/config/ -run TestCapabilityMatrixDocumentsFirstPartyCatalogs` passes
- [x] 3.2 Repair catalog-ownership openspec refs in the four acceptance/experimental docs to point at the matrix plus evidence bundle — verify: the grep across those four files exits 1, and `release-readiness.md` still contains only its dated 2026-08-07 snapshot mention
- [x] 3.3 Confirm historical evidence docs untouched — verify: `git status --short -- docs/evidence/` shows only the resealed `.sealed.jsonl` and `README.md` (plus the regenerated coverage view)
- [x] 4.1 Reword website cost promises (spend-stopped line, budget-cap caption) to honest estimates/alerts/limits language — verify: the hard-cap grep over `website/src/data/content.js` exits 1 and the new sentences state estimates without enforcement
- [x] 4.2 Honest budget reality in docs (Enforced:false, GCP live unwired, billing delay, residual resources) — verify: budget passages state validation-plus-alerts (not caps) and name GCP `--live` as unwired with the account-free alternative
- [x] 4.3 Honest expiry ownership in the six hand-editable expiry docs (owner, auth, failure mode, residual-cost resources; no CLI-timeout-as-service claim) — verify: each file's expiry passage names owner/auth/failure/residuals or makes no expiry-service claim
- [x] 5.1 Align architecture runtime claims (writable root for Magento containers with the Fargate reason, read-only only for the search-proxy sidecar, VSM at `/tmp/varnish`) — verify: the three corrected sentences are present and `internal/cloud/aws/runtime` requires zero changes (`git status` clean there)
- [x] 5.2 Record the alpha minimum-review six (secret refs, log redaction, least-priv, network exposure, artifact trust, recovery material) with evidence pointers or explicit opens, plus the outstanding full-review statement — verify: all six items plus the outstanding statement are present in `docs/architecture.md`
- [x] 6.1 Run humanizer, then remove-ai-marks, on touched human pages — verify: both passes completed (or the marks check recorded as unperformed with reason) and boxes 1.1–5.2 verifies still pass

## Risks

What could break, and the check for each.

- Reseal changes content bytes beyond digests (jq transform drift): box 1.1 diff check fails the box; compare content with digests stripped before publishing.
- Generator rewrites unrelated derived files: box 2.1 second-run check plus `git status` scoping catch it; keep only the coverage view.
- Openspec repair misses a live ref: boxes 3.1–3.2 greps fail the box; historical evidence excluded by path.
- Cost/expiry rewording introduces new unverifiable claims: boxes 4.1–4.3 verify against code (`Enforced: false`, the GCP live error string, expiry scheduler reality); quote code, do not paraphrase enforcement.
- Security wording drifts from traced settings: box 5.1 pins sentences to `containers.go`/`sidecars.go`/`runtime_test.go` lines; any runtime change fails the box.
- Marks service unreachable (audit precedent): box 6.1 records unperformed explicitly; never claim the pass.

## Proof

The end-to-end evidence that the whole spec is met, not the per-step verifies
above. Tests, commands, or screenshots.

- `go run ./cmd/gencertdocs --check` exits 0.
- `make certification-docs` twice with zero second-run diff.
- Hard-cap grep over website plus docs exits 1; openspec grep over the six live docs exits 1.
- `git status` shows only the 13 listed paths changed (plus this intent's spec/plan/report).
- No cloud resources created, changed, or destroyed (no live commands run).
