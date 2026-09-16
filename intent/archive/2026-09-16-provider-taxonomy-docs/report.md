---
slug: provider-taxonomy-docs
verified: 2026-09-16
verdict: pass
---

# Report: provider taxonomy and docs

## What shipped

One provider-root rule, documented identically in three places, with the tree
matching it:

- Move: `internal/cloud/{recovery,resilience,statearchive}` →
  `internal/shared/{recovery,resilience,statearchive}` (18 files via `git mv`,
  package names unchanged), plus import-path rewrites in 50 Go files via
  `gofmt -r` (aliases `cloudrecovery`/`cloudresilience` deliberately kept).
  Two path-naming doc comments updated in place (OVH/Scaleway `native_objects.go`).
- Delete: `cmd/magelift-{aws,gcp,ovh,scaleway}/main.go` (`git rm`; all
  `package main`, zero importers, absent from `.goreleaser.yaml`, zero
  Makefile/CI refs). `scripts/gcp-acceptance-local.sh:699` now builds
  `./cmd/magelift` with serial flags, trimpath, and the
  `MAGELIFT_GCP_ACCEPTANCE_BIN` resume override kept.
- Docs: new § Provider roots rule table (spec-verbatim) in
  `docs/adding-a-provider.md` plus opening-paragraph extension, SaaS-homes and
  adapter-less paragraphs under step 6, and the kube-exception pointer;
  ADR 0003 § Decision extended with the rule table and the frozen-scope kube
  carve-out paragraph (all cited identifiers verified in
  `internal/cloud/kube/`), § Consequences extended (move recertifies nothing);
  AGENTS.md Map one row → three rows (Never rule intact);
  `docs/gcp-acceptance.md` slim-main passage reworded to the shipped CLI +
  resume override; capability-matrix Cross-cutting rows annotated with homes
  (`internal/external/fastly`, `internal/external/newrelic`) and adapter-less
  notes (Cloudflare shell-only, SES `email.mode`-only), zero tier-word diffs;
  `contrib/skills/magelift-provider/SKILL.md` Boundary + Leave-behind mirror
  the rule, checklist untouched.
- Generate: `agents/manifest.json` digest refreshed by `go generate ./agents`
  (fallout of the pre-existing SKILL.md edit, not rename drift — zero
  rename-related strings in any generate diff).

## Deviations from plan

1. Box 1.5/4.1 `make generate` / `make generate-check` fail on the known
   pre-existing sealed-evidence digest mismatch
   (`gcp-operator-verbs-mldp8-20260915.sealed.jsonl`, verified at clean HEAD
   per goal brief). Substituted per brief: `go generate ./internal/config
   ./agents` + `genconfig --check` (green) + `gendocs --check` (green). No
   hand-edits to generated files.
2. Box 1.3's literal grep (`internal/cloud/` anywhere under `internal/shared/`)
   still matches four legitimate per-provider adapter imports in the moved
   `adapter_test.go` (`internal/cloud/{aws,gcp,ovh,scaleway}/resilience` —
   the providers the shared projection aggregates). Substance verified instead:
   zero old-path (`cloud/recovery|resilience|statearchive`) references anywhere
   under `internal/shared/`.
3. Box 2.3 could not pass until box 3.4 landed (the `docs/gcp-acceptance.md`
   slim-main reference is reworded in the docs group by plan design). Verified
   green after 3.4: all four provider patterns empty outside `intent/`.
4. Box 4.4's literal reading ("only intended paths") is unsatisfiable — the
   tree was dirty by design (319 baseline paths). Scoped reading used instead:
   snapshotted `git status` before the first edit; the delta is exactly the 92
   intended path tokens (18 moves × 2 sides, 46 import edits, 4 deletions,
   1 script, 6 doc/skill files — two of which, AGENTS.md and the matrix, were
   already dirty with disjoint pre-existing hunks — 1 generate output), and
   nothing vanished.
5. `remove-ai-marks` service still unreachable (curl exit 7); no local cleaning
   attempted per that skill. Humanizer review applied by hand to all five
   pages (plain declaratives, spec-verbatim tables, no dashes/triads/inflation;
   no edits required). Compensating scan: zero invisible codepoints in all five
   files; all ten 3.1 greps, all five 3.2 greps, and the 3.3/3.4/3.5 guards
   re-verified green after the passes.
6. `gofmt -r` used instead of `goimports -w` (`goimports` not installed; plan
   allows either). All 50 target files confirmed gofmt-clean via `xargs`
   before rewriting, so `-w` introduced no formatting churn (64/64-line
   symmetric diff).

## Verification

### Completeness

All 24 plan boxes executed: 1.1–1.5, 2.1–2.3, 3.1–3.12, 4.1–4.4. Every `###
Requirement:` in `spec.md` has direct evidence (below and in Deviations).
Box 4.1's `make generate-check` leg stays red for the pre-existing evidence
reason in deviation 1 — no unticked plan work remains; the red is owned by the
certification/seal track, identical before and after this change.

- Rule in three places: all six greps exit 0 (guide, ADR, AGENTS Map).
- Old paths gone: three `--include='*.go'` greps empty (33/27/2-file split
  confirmed pre-rewrite; 50 unique files rewritten).
- New root builds: `ls` exits 0; shared suites 49 green in 3 packages.
- SaaS homes + adapter-less: all ten 3.1 greps exit 0 (SES/Cloudflare
  adapter-less lines present twice each: opening paragraph and step 6).
- No new adapters: three `ls` paths absent; `package cloudflare` grep empty.
- Kube carve-out: all five ADR greps exit 0; every cited symbol verified in
  `internal/cloud/kube/` (`SkipAwaitAnnotations`, `ToStringArray`, `EnvVars`,
  `BuildStaticTokenKubeconfig`, `pulumi-kubernetes` v4 imports, package header).
- Slim mains: four `ls` greps empty; four dangling-ref sweeps empty outside
  `intent/`; script builds `./cmd/magelift` (bash `-n` clean); guide no longer
  names `cmd/magelift-gcp`.
- No behavior change: `go build ./...` exit 0; shared 49 + config 191 +
  consumers 234 (aws/gcp/ovh/scaleway resilience, kube, both state packages)
  green, all serial.
- Generated drift: `genconfig --check` + `gendocs --check` exit 0;
  `generate-check` red only on the pre-existing evidence file.
- No tier change: both `certified …` anchors present; tier-word diff empty
  (pre-existing SendGrid hunk contains no `certified` token).
- Docs/website: `make docs` exit 0 (twice); `website/` grep empty.

### Correctness

Bar is the intent's proposed outcome: one documented root decision with the
tree matching it, explicit homes or explicit "not a provider" labels, shared
ports no longer reading as clouds, guide + ADR + matrix in agreement, docs
green, no runtime change. All met: the rule table is identical in meaning in
all three docs; the three ports moved with APIs unchanged (package names and
aliases kept); SaaS homes, the `internal/edge/waf` contract, and both
adapter-less cases are labeled with reasons; `internal/cloud/` now holds only
the four IaaS adapters plus the single ADR-blessed `kube` helper; slim mains
are gone with no dangling refs. Not a UI change; human-observable moments are
the built docs site (rebuilt green from the edited pages) and the `ls`/tree
shape itself. The outcome's "generate-check stays green" clause holds in
substance (deviation 1): the gate was red at baseline on the same file, and
this change adds zero drift to it.

### Coherence

Diff follows the spec Design: `internal/shared/` destination with the
documented rule, verbatim rule table and carve-out wording, validated delete
(not GoReleaser wiring) for slim mains with the heavier-link cost accepted and
the resume override kept, minimal docs list with the explicit NOT-touched set
respected (`versioning.md`, `evidence/README.md`, `architecture.md`,
`website/`, `magelift-contribute`, shipped user skills — all untouched,
website re-grepped). Aliases intentionally stale per the plan's risk note, not
as missed rewrites.

## Findings

- WARNING — `make generate` / `make generate-check` red on pre-existing
  sealed-evidence digest mismatch
  (`docs/evidence/runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl:1`).
  Identical at clean HEAD per goal brief; blocks nothing in this intent but
  blocks any future `make generate`/`make verify` run. Owned by the
  certification/seal track. `Makefile:278`
- WARNING — `remove-ai-marks` HTTP service down two intents running
  (connection refused, curl exit 7). Human pages ship with hand humanizer
  review + invisible-mark scans only. Environmental, not a content finding.
- SUGGESTION — box 2.3's verify cannot pass before box 3.4 by plan construction
  (docs reword lives in group 3); future plans should order the sweep after the
  docs edit or scope it to non-docs paths. `intent/provider-taxonomy-docs/plan.md:79`

## Not checked

- Full `go test -race ./...`: skipped per goal brief (unbounded race suites
  forbidden from IDE sessions). Compensated with `go build ./...` plus all
  474 tests across every package the rename touches.
- Live `scripts/gcp-acceptance-local.sh` run with the full-CLI link: not run
  (30+ minute link; change is a one-token path swap, syntax-checked, flags
  kept). No live-cloud spend per brief.
- Verified in implementing session (no forked verifier; re-running the build
  plus 474 tests elsewhere duplicates identical evidence).

## Verdict

Pass. Taxonomy documented in three agreeing places, tree matches it, SaaS and
adapter-less homes explicit, kube carved out, slim mains deleted without
dangling refs, no behavior or tier change. The red `generate-check` leg is
pre-existing evidence breakage, unchanged by this intent.
