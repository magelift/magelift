---
status: done
slug: release-trust-baseline
intent: intent.md
---

# Spec: trust baseline before any release promise

## Requirements

What the system must do. Testable. Not file paths. One block per requirement,
each with at least one scenario.

### Requirement: Void stale seal and re-attest with provenance

The defective `mldp8` seal SHALL be explicitly voided and the verified content
re-attested with recorded provenance. The original attestation SHALL NOT be
presented as valid.

Forensics (2026-09-16): all 13 cell lines in
`docs/evidence/runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl` fail
`recordDigest` verification while line 14 (cleanup SKIP) verifies; the other
five sealed bundles verify. The file is byte-identical to its sealing commit
(`77ae916`); the sealing code (`recordDigest`) is unchanged since (only a
mechanical import rewrite). No field-subset variant reproduces the recorded
digests, so the original seal inputs are unrecoverable. Content is
independently corroborated by the human evidence doc (artifact digest
`8588b13f…fdb2be4`, seed
`magento-249-sanitized-definer-free.sql.gz`, five verb outcomes) and the
Order-12 verification report (13 PASS / 1 SKIP).

#### Scenario: Gate green on verified content

- **WHEN** `go run ./cmd/gencertdocs --check` runs
- **THEN** it exits 0 and the `mldp8` bundle loads with all 14 records verifying

#### Scenario: Provenance recorded, original voided

- **WHEN** a reader opens `docs/evidence/README.md`
- **THEN** they find a dated note naming the voided `77ae916` seal, the defect
  (13/13 cell digests invalid at commit, inputs unrecoverable), the
  corroboration performed (artifact digest, seed, 13/1 counts against the
  human evidence doc and Order-12 report), the reseal date and tool
  (`magelift certification seal` over `SealJSONL`), and the statement that
  the original attestation is void and the reseal is a new attestation over
  verified content — not a blessing of the original seal

#### Scenario: No silent recomputation

- **WHEN** the sealed file diff is reviewed
- **THEN** every changed line is a `recordDigest` value only (content bytes
  otherwise identical to the voided file), and the commit message records
  the void reason, corroboration, and reseal tool

### Requirement: Derived evidence views regenerated

Derived evidence files SHALL be regenerated through supported generators with
no hand edits.

#### Scenario: Coverage view current

- **WHEN** `make certification-docs` (or the generator it wraps) runs
- **THEN** `docs/evidence/current-capability-coverage.md` reflects the resealed
  bundle and a second run produces zero diff

### Requirement: Matrix and evidence name exact proof

The capability matrix plus the evidence README SHALL name exactly what each
certified cell proves: the exact proved tuple, the AWS preview shape with
search disabled, the GCP proved dimensions, the operator run's deploy result
as a rejection guard (not a successful complete store upgrade), and the
preserved distinction between GCP application-level search proof (reindex,
HTTP product result, post-recycle querying) and AWS infrastructure-only
search proof. No preview cell SHALL support a blanket production-readiness
claim.

#### Scenario: Exact tuples named

- **WHEN** a reader opens `docs/capability-matrix.md`
- **THEN** each certified row states its exact proved tuple (provider,
  runtime, Magento release, preset, database, search, queue, cache, edge)
  with a link to the sealed run that proves it

#### Scenario: Operator run scoped honestly

- **WHEN** a reader follows the operator-verbs evidence reference
- **THEN** the text states the deploy result was a rejection guard, lists the
  13 exercised verbs, and does not claim a complete store upgrade

#### Scenario: Search proof distinguished

- **WHEN** a reader compares the GCP and AWS search rows
- **THEN** the GCP row claims the application-level sequence and the AWS row
  stays infrastructure-only with the missing application sequence named

### Requirement: Deleted planning tree leaves no live dangling refs

Live docs SHALL NOT reference the deleted `openspec/` tree. Historical
evidence docs SHALL keep their contemporary references intact as
point-in-time records.

#### Scenario: Live docs clean

- **WHEN** `grep -rni "openspec" docs/capability-matrix.md docs/aws-acceptance.md
  docs/gcp-acceptance.md docs/ovh-experimental.md docs/scaleway-experimental.md
  docs/release-readiness.md` runs
- **THEN** it exits 1 (no matches); catalog-ownership lines point at the
  capability matrix plus the evidence bundle instead

#### Scenario: History preserved

- **WHEN** the evidence docs under `docs/evidence/` are reviewed
- **THEN** their contemporary OpenSpec task references remain byte-identical
  (no history rewrite inside point-in-time records)

### Requirement: Cost language matches enforcement reality

Website and docs cost language SHALL match the code: budgets report
`Enforced: false` (AWS and GCP readers), GCP `--live` pricing returns "not
wired yet", and no hard-cap or spend-stopped promise SHALL appear. Estimates,
missing prices, alerts, and limits SHALL be reported honestly. Expiry SHALL
be specified with its owner, authentication, failure mode, and the resources
that can keep costing after teardown; a CLI-process timeout SHALL NOT be
presented as an autonomous expiry service.

#### Scenario: No hard-cap promises

- **WHEN** `grep -rni "spend stopped\|budget cap\|spending cap\|hard cap"
  website/src/data/content.js docs/` runs
- **THEN** it exits 1 (no matches)

#### Scenario: Budget reality stated

- **WHEN** a reader opens the cost/budget docs and website pricing-adjacent copy
- **THEN** they find that budgets are validated estimates with alerts, that
  enforcement reads `Enforced: false`, that GCP live pricing is unwired
  (account-free mode only), and that provider billing delay plus residual
  resources (named) prevent an exact spend-stop claim

#### Scenario: Expiry ownership honest

- **WHEN** a reader opens the preview-expiry docs
- **THEN** they find who runs the expiry (scheduler owner), how it
  authenticates, what happens if it fails, and which backups, volumes,
  addresses, or other resources can continue to cost money after teardown

### Requirement: Security claims match runtime settings

Security documentation SHALL match the traced runtime: Magento containers
(php-fpm, web/nginx, varnish, cron/consumers) use a writable root
(`ReadonlyRootFilesystem: false`) with the code-cited reason (env.php,
generated files, nginx pid; Fargate empty volumes mount root:root); only the
search-proxy sidecar uses a read-only root. The Varnish VSM path SHALL be
stated as image `/tmp` (`-n /tmp/varnish`), not tmpfs at `/var/lib/varnish`.
The alpha recipe SHALL record the minimum review (secret references, log
redaction, least-privilege permissions, network exposure, artifact trust,
recovery material); the full security review SHALL stay explicitly named as
outstanding.

#### Scenario: Writable root stated

- **WHEN** a reader opens `docs/architecture.md`
- **THEN** the runtime section states Magento containers use a writable root
  with the Fargate reason, names the search-proxy sidecar as the sole
  read-only-root container, and the Varnish paragraph states VSM at
  `/tmp/varnish` — all consistent with `internal/cloud/aws/runtime/containers.go`,
  `sidecars.go`, and `runtime_test.go`, which SHALL require no changes

#### Scenario: Minimum review recorded

- **WHEN** a reader opens the alpha-recipe security section
- **THEN** they find the six minimum-review items each marked reviewed with
  its evidence pointer or explicitly open, plus the statement that a full
  penetration test and public-installation proof remain outstanding

## Design

How it fits the existing codebase: surfaces, data, APIs, ownership.

No product code changes. The reseal runs the existing supported path:
strip `recordDigest` per line with `jq -c 'del(.recordDigest)'` into
unshaled candidates (throwaway, never committed), run
`magelift certification seal` (backed by `certification.SealJSONL`), move
the output over
`docs/evidence/runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl`, then
`go run ./cmd/gencertdocs --check` must exit 0. The only bytes that change
in the sealed file are the 13 stale `recordDigest` values.

Surfaces touched (docs and website copy only, plus the resealed bundle):
`docs/evidence/runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl` (digests
only), `docs/evidence/README.md` (provenance note), `docs/capability-matrix.md`
(exact-proof wording, openspec ref repair), `docs/aws-acceptance.md`,
`docs/gcp-acceptance.md`, `docs/ovh-experimental.md`,
`docs/scaleway-experimental.md`, `docs/release-readiness.md` (openspec ref
repair), `docs/evidence/current-capability-coverage.md` (regenerated),
`website/src/data/content.js` (cost language), cost/budget and expiry docs
(honest enforcement wording), `docs/architecture.md` (writable-root
alignment, VSM path), alpha-recipe security section (minimum review record).
Historical files under `docs/evidence/` stay byte-identical.

Ownership: the matrix plus the evidence README remain the joint authority
for cell status; this intent tightens their wording without changing any
cell tier. Cost enforcement stays unimplemented by design for alpha (honest
estimates, not caps); expiry mechanics stay owned by the provider plus
acceptance intents (this intent documents the policy honestly). The
writable-root setting stays as coded (Magento requirement); only the claims
change.

## Gotchas / policy flags

Security, auth, PII, compatibility, contradictions the spec cannot satisfy.

- Never present the reseal as the original attestation. The README note, the
  commit message, and this spec all state the original seal is void.
- No cloud resources created, changed, or destroyed. No live calls. The
  corroboration reuses the committed human evidence doc and Order-12 report.
- Regenerate derived files through supported generators only; no hand edits
  to generated schema, CLI reference, or certification coverage.
- Human docs and website copy go through humanizer, then remove-ai-marks.
  If the marks service is unreachable, record the check as unperformed (do
  not claim it) per the audit precedent.
- A preview cell never supports a blanket production-readiness claim.
- The full security review and penetration test stay outstanding and named;
  this intent records the minimum review only.
- Matrix tier changes are out of scope; wording changes only.

## Open questions carried forward

Unresolved items from intent.md, plus new ones. Each has an owner or a default.

- Sealed-run original recoverable or withdraw-and-reprove? Resolved by
  forensics: no valid original exists (seal invalid at commit, inputs
  unrecoverable). Decision: void the stale seal and re-attest verified
  content with provenance (this spec) rather than withdrawing the
  corroborated cell. Rationale recorded in Requirement 1. Owner: implementer
  with maintainer sign-off at review.
- Which website claims beyond budgets need rewording? Resolved by inventory
  in this spec: `destroyed · spend stopped` (line 37) and the preview
  `budget cap` caption (line 105) in `website/src/data/content.js`, plus
  budget/expiry docs wording. Owner: implementer.
