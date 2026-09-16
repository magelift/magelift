---
slug: release-trust-baseline
verified: 2026-09-16
verdict: pass
---

# Report: trust baseline before any release promise

## What shipped

Gates green and honest, no product code changes:

- Resealed `docs/evidence/runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl`:
  13 stale `recordDigest` values renewed via `magelift certification seal`
  (`SealJSONL`); parsed content otherwise byte-identical to the voided file.
  `go run ./cmd/gencertdocs --check` exits 0.
- Provenance note in `docs/evidence/README.md` voiding the `77ae916` seal
  (13/13 cell digests invalid at commit, inputs unrecoverable), recording
  corroboration (artifact digest, seed, 13/1 counts vs the human evidence doc
  and Order-12 report) and the reseal date/tool, and stating the original
  attestation is void.
- Regenerated `docs/evidence/current-capability-coverage.md` via
  `make certification-docs` (second run byte-identical).
- Matrix exact-proof wording: certified AWS tuple paragraph with its evidence
  link, AWS search infra-only with the missing app sequence named plus the
  `mlaw1` reference, GCP search application-sequence proof with the `mldp6`
  reference, explicit `gcap28` sealed-run filename, and the deploy-guard scope
  on the evidence README `mldp8` row ("not a full upgrade").
- Openspec ref repair in five live docs (matrix authority line plus four
  catalog-ownership lines) to the matrix plus evidence bundle, keeping the
  `certification-*` ID tokens the doc test asserts. Historical evidence docs
  and the dated `release-readiness.md` snapshot sentence stay byte-identical.
- Website cost honesty: `destroyed · teardown reported` and the preview
  `validated budget with alerts` caption; hard-cap grep over website plus docs
  exits 1.
- Docs budget reality (`Enforced: false`, GCP `--live` unwired with the
  account-free alternative, billing delay) and honest preview-expiry ownership
  (customer scheduler owns the run, operator credentials, failure keeps
  billing, residuals named) in `docs/operations.md`.
- Architecture alignment: writable root for Magento containers with the
  Fargate reason, read-only only for the search-proxy sidecar, VSM at
  `/tmp/varnish`, plus the six-item alpha minimum-review record with the full
  review explicitly outstanding. Runtime code untouched.
- Prose pipeline: humanizer review complete with no edits required;
  remove-ai-marks service 0.7.0 healthy, 8 docs cleaned-unchanged,
  `content.js` inspected with 0 suspicious marks.

## Deviations from plan

1. Void-and-reseal instead of the open-question default (withdraw the cell and
   schedule live re-proof). Rationale: the content is independently
   corroborated by the human evidence doc and Order-12 report, this intent
   allows no live cloud work, and the proposed outcome explicitly permits
   "withdraw and replace with provenance." The original attestation is voided
   in the README, the spec, and the commit message — not silently blessed.
   Maintainer sign-off at review (per spec open question).
2. `docs/release-readiness.md` kept its sole openspec mention: it is the dated
   2026-08-07 snapshot sentence, a true point-in-time record. Rewriting it
   would falsify history. Plan file list and box 3.2 updated in the same
   change before implementing.
3. Boxes 1.1 and 2.1 ticked together: gate-green required both the reseal and
   the coverage regeneration, so 1.1's verify could only pass after 2.1 ran.
   No work skipped; order preserved.
4. Box 3.1 verify wording clarified before implementing (proof link is the
   sealed run where one exists, the evidence doc otherwise — no sealed AWS
   bundles exist yet).
5. Layer B rewrite offered and skipped for the two short website strings:
   short/highly-predictable UI text, rewrite disproportionate. Recommendation
   recorded here in place of a user offer round-trip under autonomous mode.

## Verification

### Completeness

All 12 plan boxes ticked (1.1, 1.2, 2.1, 3.1, 3.2, 3.3, 4.1, 4.2, 4.3, 5.1,
5.2, 6.1), each after its verify clause passed. Every `### Requirement:` in
`spec.md` has direct evidence:

- Void/re-attest: gate exit 0 seen twice; parsed-content diff of voided vs
  resealed empty (`jq -cS` both sides); raw diff exactly 26 lines (13 digest
  pairs); provenance note contains `77ae916`, the 13/13 defect, artifact
  digest, seed, reseal date/tool, and the void sentence.
- Derived views: `make certification-docs` exit 0; second run same sha256
  (`ea3e8ead…89a1de`).
- Exact proof: AWS tuple paragraph with `20260813ai` link present; AWS search
  row names the missing reindex/query/recycle sequence with the `mlaw1` link;
  GCP search row names the application sequence with the `mldp6` link;
  `gcap28` sealed filename explicit; README `mldp8` row carries the
  rejection-guard scope.
- Openspec refs: grep exits 1 on the matrix plus four acceptance docs;
  `release-readiness.md` holds only the dated snapshot sentence;
  `TestCapabilityMatrixDocumentsFirstPartyCatalogs` passes (ID tokens kept).
- Cost language: hard-cap grep over website plus docs exits 1; new website
  sentences state estimates/alerts without enforcement; operations carries
  the `Enforced: false`, GCP-unwired, billing-delay, and expiry-ownership
  sentences (all grepped present).
- Security alignment: three corrected architecture sentences grepped present;
  `git status` clean under `internal/cloud/aws/runtime/`; six review items
  plus the outstanding statement grepped present.

### Correctness

Bar is the intent's proposed outcome: sealed mismatch reconciled with
provenance (not silent recomputation), derived files regenerated, matrix plus
evidence naming exact proof, cost/security language matching code. All met:
the gate that failed on line-1 digest mismatch now exits 0 over verified
content; the voided seal is documented as void in three places (README, spec,
commit message); every tightened sentence quotes or cites traced code,
evidence docs, or live-run records — no new unverifiable claims introduced
(the docs build is green, so no links broke). Not a UI change; observable
moments are the green gate output, the regenerated coverage view, and the
rebuilt docs site.

### Coherence

Diff follows the spec Design: docs plus website copy plus the resealed bundle
only; historical evidence byte-identical; supported generators used for the
seal and the coverage view; no product code touched; cell tiers unchanged
(wording only). Matches repo conventions (evidence README as joint authority,
matrix exactness discipline, humanizer/marks pipeline).

## Findings

- SUGGESTION — Maintainer to confirm the void-and-reseal decision vs
  withdraw-and-reprove at review; the forensics and corroboration are in the
  spec and README note, sign-off is the remaining gate.
  `intent/release-trust-baseline/spec.md:1`
- SUGGESTION — No sealed AWS bundles exist yet, so AWS certified rows link to
  human evidence docs rather than machine-verifiable runs. Future sealed AWS
  runs would strengthen the gate's coverage.
  `docs/capability-matrix.md:91`

## Not checked

- Full `go test ./...` and `make verify`: scoped to the affected suites
  (`internal/config` doc test), the evidence gate, the docs build, and the
  prose greps per serial-build discipline; the full matrix runs in CI.
- Live CI run of this change (no PR opened from here).
- Verified in implementing session (no forked verifier; evidence is command
  output plus file diffs above).

## Verdict

Pass. The trust baseline is green and honest: evidence integrity restored with
explicit provenance, exact-proof wording in place, cost and security claims
matching traced code, and no live-cloud side effects. No CRITICAL findings.
