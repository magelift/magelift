---
slug: github-pages-deploy
verified: 2026-09-16
verdict: pass
---

# Report: GitHub Pages single deploy story

## What shipped

One documented deploy story — GitHub Pages via the Public site workflow:

- Deleted: `website/public/_headers` (18-line Cloudflare path blocks),
  `website/public/_redirects` (4 lines incl. the invalid bash shebang line —
  deleted, not fixed), `.cloudflare/cache/cloudflare-account.json` (tracked
  cache, shape-checked types-only first), `.github/workflows/docs.yml`
  (redundant 42-line workflow). All via `git rm` (staged, uncommitted).
- `.gitignore` gains `.cloudflare/` next to `.wrangler/`; the cache path
  verifies ignored.
- Skill `## Deploy` rewritten verbatim to the Pages wording (push-to-main,
  `site.yml` rebuild, `actions/deploy-pages`, PRs build-only, CNAME/`www`
  apex note, no separate docs host).
- Verified-unchanged: `website/README.md` + `docs/publishing.md` (already
  Pages-correct, zero residue — no edits, no humanizer needed), `CNAME`
  (`magelift.dev` byte-identical, incl. in `dist/`), `site.yml`, `build-site.sh`,
  `astro.config.mjs`, `mkdocs.yml`, `getting-started/index.html` meta-refresh.
- Untouched per scope: acceptance Cloudflare DNS helpers, `site.yml`,
  `build-site.sh`, Astro config, URLs (`magelift.dev`, `/docs/`, www apex).

## Deviations from plan

1. Box 2.3's literal sweep (`grep -rln ... | grep -v "mkdocs\.yml"`) cannot
   work: with `-l` the output is filenames, so the content filter never
   matches (4 files listed). Verified at content level instead: every
   `docs\.yml` match in those files is a `mkdocs.yml` substring; zero
   references to the deleted workflow. Same verdict, sound method.
2. Line citations drifted by earlier intents (`ci.yml` docs job now at 255,
   not 192-207, after order-18 inserts). Re-located rather than trusted:
   docs job still runs `mkdocs build --strict` on the docs path filter;
   `site.yml` gate (line 54), deploy action (line 65), and PR trigger
   (line 14) byte-match the plan. Deletion safety case re-proven on the live
   tree.
3. `remove-ai-marks` service still unreachable (curl exit 7, five intents
   running). Moot here: the only human-page edits possible (README,
   publishing) were no-ops on drift-free pages, and the skill Deploy section
   is spec-verbatim agent surface (its em dash rides verbatim per the plan;
   the skill's own no-dash rule scopes to landing/README copy).
4. Box 4.4 (post-merge Pages deploy observation) is merger action by plan
   design — not executed here, left unticked. Pre-merge continuity signal is
   green (full `build-site.sh` exit 0 twice); the merger watches `site.yml`
   on `main` per plan 4.4.

## Verification

### Completeness

14 of 15 plan boxes ticked; 4.4 open (merger action, deviation 4). Boxes 1.2
and 4.2 ticked as evaluated-not-triggered (primary live check passed, npm
build green — both fallbacks correctly dormant). Every spec requirement has
direct evidence:

- Live host first: apex `server: GitHub.com` exactly, `x-github-request-id`
  present, no `cf-ray`, `www` CNAME `magelift.github.io.`, all four apex A
  records in the Pages range. No fallback needed.
- Deletions: all four paths gone from tree and index; `dist/` carries no
  residue after two green builds; nothing referenced `_headers`/`_redirects`
  in build config or Astro config.
- Cache: untracked + ignored; pre-delete shape exactly `['account']` /
  `{'account': 'dict'}` (116B); values never printed, never in evidence.
- `docs.yml`: gone; content-level sweeps empty for workflow refs,
  `mkdocs-site`, and `Docs site`; branch `main` is unprotected (API 404), so
  no required check can point at the deleted workflow — stronger than the
  UI-fallback path.
- Skill: zero wrangler/cloudflare (case-insensitive), exact deleted line
  gone, `site.yml` + `deploy-pages` present; `## Stack` and all other
  sections byte-identical (single-hunk diff).
- README/publishing: both name GitHub Pages, zero residue, zero edits.
- URLs: apex config, `site_url`, skill canonical URL all pinned; three-file
  diff empty.
- Live coverage for every deleted behavior: in-tree meta-refresh survives;
  `/docs` 301, `/llms.txt` MIME (`text/plain; charset=utf-8`), and www→apex
  301 all served natively by `server: GitHub.com` (recorded verbatim).
- Build: `build-site.sh` exit 0 twice; `dist/docs` + `dist/CNAME`
  (`magelift.dev`) present.

### Correctness

Bar is the intent's proposed outcome: one deploy story stated identically in
skill, README, and publishing notes; Cloudflare residue removed; `docs.yml`
deleted; site keeps deploying to `magelift.dev` with docs at `/docs/` on
main pushes. All met: the three pages agree verbatim on Pages-via-`site.yml`;
all four residue paths are gone with no dangling references; the full build
(gating the same `site.yml` the deploy uses, minus the main-only deploy step)
is green with correct `dist/` shape; live evidence proves each deleted
behavior already served natively. Not a UI change; observable moments are the
green build output, the `dist/` tree shape, and the recorded live header
transcripts.

### Coherence

Diff is exactly the spec's file-operation table: 4 deletions, 1 ignore line,
1 skill section, zero edits to `site.yml`, `build-site.sh`, Astro config,
`mkdocs.yml`, `CNAME`, URLs, or acceptance helpers. Status delta vs the
box-start baseline is exactly the 5 intended paths (4 deletions + skill edit;
`.gitignore` hunks onto its order-18-dirty state); nothing vanished, no
strays. Smallest correct change; no redesign, no replacement header/redirect
mechanism (nothing lost — live evidence).

## Findings

- WARNING — box 4.4 (post-merge deploy observation) pending the merger; the
  merger re-runs the apex fingerprint after the next `main` deploy per plan
  4.4. Pre-merge signal green. `intent/github-pages-deploy/plan.md:82`
- SUGGESTION — plan's 2.3 sweep mixes `-l` (filenames) with a content filter;
  future plans should sweep content (`-rn`) when the exclusion is content.
  `intent/github-pages-deploy/plan.md:53`

## Not checked

- Post-merge Pages deploy (box 4.4 — requires a merged main push; owner:
  merger).
- Live CI run of the edited workflows (no PR opened from here); `make
  workflow-check` (actionlint over the full workflow set incl. fixtures)
  green locally after the deletion.
- Verified in implementing session (no forked verifier; evidence is command
  output + live transcripts above).

## Verdict

Pass. Single Pages deploy story documented and de-duplicated, residue
deleted with live-proven no-loss, build green, URLs pinned. Post-merge
observation belongs to the merger per plan.
