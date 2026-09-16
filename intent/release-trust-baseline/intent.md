---
status: accepted
slug: release-trust-baseline
---

# Intent: trust baseline before any release promise

## Problem

The release gates cannot be trusted as they stand. The evidence integrity
check fails on a sealed GCP operator run, certification language can be read
as broader than the proved cells, website copy promises budget caps and
spend-stopped behavior the code does not enforce, and security documentation
disagrees with runtime settings. Building an alpha on red gates means every
later claim inherits the doubt.

## Evidence

`intent/audit.md` F09, reproduced 2026-09-16: `go run ./cmd/gencertdocs
--check` rejects line 1 of
`docs/evidence/runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl` (record
digest does not match contents). The generated evidence view is untrustworthy
until reconciled.

F08: the AWS predicate in
`internal/cloud/aws/stack/certification_tier.go` includes a preview shape with
search disabled; the GCP predicate does not itself express every proved
dimension. The archived GCP search report records reindexing plus HTTP product
results plus post-recycle querying; the AWS search report records
infrastructure success without that full application sequence. The archived
operator run exercised 13 verbs but its deploy result was a rejection guard,
not a successful complete store upgrade.

F10: `website/src/data/content.js` uses budget-cap and spend-stopped language
while `internal/cloud/aws/cost/budget.go` and
`internal/cloud/gcp/cost/budget.go` report `Enforced: false`, and
`internal/cloud/gcp/cost/estimate.go` returns an error for `--live` ("not
wired yet"). Validating a positive budget is not spending enforcement.

F12: architecture docs describe read-only container filesystems while the
traced AWS container configuration uses a writable root filesystem with a
temporary reason pending writable storage design.

## Proposed outcome

Gates are green and honest before any alpha work builds on them: the sealed
evidence mismatch is reconciled against the original artifact (restore the
valid original, or explicitly withdraw and replace with provenance — never
recompute hashes to bless altered contents), derived files regenerate through
supported generators, the matrix plus evidence README name exactly what each
cell proves, website and docs language matches enforcement reality (estimates,
missing prices, alerts, limits; no hard-cap promise), and security claims
match runtime settings (writable-path requirements resolved, then config and
claims aligned). A full security review stays outstanding and is named as such;
at minimum the alpha recipe gets reviewed secret references, log redaction,
least-privilege permissions, network exposure, artifact trust, and recovery
material.

## Affected users and systems

Maintainer release confidence; every prospective pilot reader. `docs/evidence/`
(sealed runs, README, generated views), `docs/capability-matrix.md`,
`website/src/data/content.js` and install/onboarding copy, container runtime
settings and their docs, `cmd/gencertdocs` gate.

## Constraints

- Do not recompute hashes to make an altered assertion look like its original
  attestation. Provenance or withdrawal, with a record.
- Regenerate derived files through supported generators only; no hand edits to
  generated schema, CLI reference, or certification coverage.
- Certification claims stay bounded by matrix plus evidence; a preview cell
  never supports a blanket production-readiness claim.
- A CLI-process timeout is not an autonomous expiry service; expiry ownership,
  auth, failure mode, and residual-cost resources get specified honestly
  (full mechanics land in the provider plus acceptance intents).
- Human docs and website copy go through humanizer, then remove-ai-marks.
- No cloud resources created, changed, or destroyed in this intent; this is
  reconciliation plus prose plus config-claim alignment.

## Out of scope

- New provider code, new catalog cells, or the provider contract (owned by
  `provider-plugin-contract` and followers).
- Magento health/lifecycle repair (owned by `magento-deployment-safety`).
- The full penetration test or public installation proof (tracked, not closed
  here).
- The alpha tag decision.

## Open questions

- Is the sealed-run original recoverable from git history or archives, or must
  the cell be withdrawn and re-proved? Default: restore if the valid original
  exists; otherwise withdraw explicitly and schedule re-proof in
  `reference-store-acceptance`. Owner: implementer with maintainer sign-off.
- Which website claims beyond budgets need rewording once enforcement reality
  is inventoried? Default: audit every cost and security sentence against code
  in the spec. Owner: spec author.
