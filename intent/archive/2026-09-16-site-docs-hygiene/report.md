---
slug: site-docs-hygiene
verified: 2026-09-16
verdict: pass
---

# Report: site and docs hygiene

## What shipped

Three hygiene fixes, each minimal:

- Generated header: `internal/config/schema.go` `ReferenceMarkdown()` now
  emits title, blank, `This page is generated from the MageLift configuration
  schema. Do not edit it by hand.`, blank, then the existing prose —
  byte-mirroring the `cli-reference.md` and capability-coverage header shapes.
  `docs/configuration.md` regenerated via the generator (never hand-edited);
  marginal diff vs the dirty baseline is the 2-line header hunk only.
- Sitemap untrack: `/website/public/sitemap.txt` + `/website/public/sitemap.xml`
  appended to `.gitignore` (website section, after `/website/public/docs/`);
  both blobs untracked via `git rm --cached` (working-tree files kept). Site
  build regenerates both on disk with zero new tracked churn (status shows
  only the two intended staged deletions).
- Orphan resolved by link (zero content change): `docs/aws-acceptance.md:140`
  and `docs/gcp-acceptance.md:359` both link
  `[acceptance command dependencies](acceptance-dependencies.md)` in context
  (line numbers match research — no drift); no `mkdocs.yml` nav entry, so the
  link is the single resolution.

Untouched per scope: `cmd/genconfig` (thin writer), `generate-sitemap.py`
(write lines + content), nav, human prose, landing, URLs, deploy story.

## Deviations from plan

1. `make generate` / `make generate-check` wrappers fail pre-existing at
   `gencertdocs` (same sealed-evidence file, 5th intent running). Used the
   brief's substitute throughout: `go generate ./internal/config ./agents`
   for regeneration, `go run ./cmd/genconfig --check` for drift checks.
   Marginal diff: header hunk only (proven below).
2. Box 1.2's literal `git diff --exit-code` (vs HEAD) cannot pass: the header
   itself diffs by design (1.1's purpose), and pre-existing SendGrid residue
   diffs alongside. Stability proven instead three ways: second + third
   `go generate` runs byte-identical (sha256-verified), `genconfig --check`
   green after every run, `TestReferenceMarkdownIsDeterministic` green.
3. Box 2.2's literal `git status --porcelain` emptiness cannot hold: the two
   `D ` staged deletions from box 2.1 are the intended change itself.
   No-churn proven instead: status shows ONLY those two staged lines (no `M`,
   no `??`), both paths verify ignored, both files exist post-build.
4. `remove-ai-marks` + humanizer skipped per spec Design (mechanical header/
   ignore/verification-only changes; spec explicitly exempts them). No
   human-facing sentence was added anywhere — the header is the pinned
   verbatim mirror.

## Verification

### Completeness

All 7 plan boxes ticked: 1.1, 1.2, 2.1, 2.2, 3.1, 3.2, 4.1. Every spec
requirement has direct evidence:

- Header: file head exact lines 1–5 (title/blank/header/blank/prose);
  literal grep green; emitted by `ReferenceMarkdown()`, regenerated output
  only.
- Stability: three consecutive generations byte-identical (sha256);
  `genconfig --check` green twice; determinism unit test green; config suite
  191 green; `schema.go` gofmt clean.
- Sitemap: `git ls-files` empty for both; both ignore lines present; post-
  build both exist on disk; status shows only the staged deletions.
- Orphan: backlink grep shows both verbatim parent links at research-exact
  lines; nav grep exits 1.
- Docs gate: `make docs` exit 0 (~2s; only pre-existing INFO exclusion
  notices, no strict warnings).

### Correctness

Bar is the intent's proposed outcome: header matching the sibling pattern,
sitemaps as regenerating untracked products, orphan unambiguously resolved.
All met: the header is character-identical in shape to both siblings and
flows through the generator (hand-edit would fail `--check`, proven green);
the sitemap pair untracked + ignored + regenerated with zero churn; the
orphan has exactly one resolution (two in-context links, no nav entry),
closing the ambiguity with zero content change. Not a UI change; observable
moments are the file head, the `git ls-files`/status outputs, and the green
strict build.

### Coherence

Diff is exactly the plan's file table: 1 code edit (`schema.go` header
WriteStrings), 1 regenerated page (+2 lines marginal), 2 ignore lines,
2 untracks, 0 orphan edits. Status delta vs the box-start baseline is exactly
the 2 sitemap paths (all other hunks land on already-dirty files from
order-14 residue); nothing vanished, no strays. Smallest correct change per
fix; no nav redesign, no sitemap content change, no generated-content change
beyond the header.

## Findings

- WARNING — `make generate` / `make generate-check` wrappers red on
  pre-existing sealed-evidence digest mismatch (same file as orders 15–19).
  Direct `go generate` + `genconfig --check` substituted throughout.
- SUGGESTION — plan boxes 1.2 and 2.2 state literal emptiness checks that
  their own boxes' intended changes violate (header diff, staged deletions);
  future plans should assert marginal/stability form (repeat-run identity,
  non-`D` emptiness) as executed here.
  `intent/site-docs-hygiene/plan.md:46`

## Not checked

- Full `go test -race ./...`: skipped per goal brief. Compensated with the
  191-test config suite (owns the edited file) plus byte-exact generate
  checks.
- Live CI run (no PR opened from here).
- Verified in implementing session (no forked verifier; evidence is command
  output above).

## Verdict

Pass. All three hygiene bugs closed minimally with green gates and zero
content drift beyond the intended header.
