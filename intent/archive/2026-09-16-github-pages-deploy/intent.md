---
status: done
slug: github-pages-deploy
---

# Intent: GitHub Pages as the single site deploy story

## Problem

The website deploy story is told three contradictory ways, so contributors and agents cannot tell where magelift.dev actually ships from or which config files matter. Stale Cloudflare residue sits next to a live GitHub Pages pipeline, and a redundant docs workflow rebuilds on every main push for an artifact nobody deploys.

## Evidence

- `.github/workflows/site.yml` builds Astro + MkDocs and deploys `website/dist` via `actions/deploy-pages` (GitHub Pages).
- `docs/publishing.md` launch checklist says "Site on GitHub Pages (`magelift.dev`): Done"; `website/README.md` says the Public site workflow deploys to GitHub Pages.
- `contrib/skills/magelift-site/SKILL.md` documents deploy via `npx wrangler pages deploy` (Cloudflare) instead.
- `.github/workflows/docs.yml` header comment says "GitHub Pages is unavailable on the current private plan; deploy `site/` via Cloudflare Pages", and the workflow uploads a 14-day `mkdocs-site` artifact on main pushes that nothing consumes (`ci.yml` already runs the strict docs check on PRs).
- `website/public/_headers` and `website/public/_redirects` use Cloudflare/Netlify-only conventions, which GitHub Pages ignores entirely, so the `llms.txt` MIME forcing and `/docs` redirect in them are dead config. `_redirects` additionally opens with a `#!/usr/bin/env bash` line, which is not valid redirect-file content.
- `.cloudflare/cache/cloudflare-account.json` is tracked in git (`git ls-files` confirms); cache files should not be committed.
- User decision this session: GitHub Pages is the host.

## Proposed outcome

One documented deploy story: GitHub Pages via the Public site workflow, stated identically in the skill, the website README, and the publishing notes. The Cloudflare residue (`_headers`, `_redirects`, tracked `.cloudflare/` cache) is removed, `docs.yml` is deleted as redundant, and the site keeps deploying to magelift.dev with docs at `/docs/` on pushes to main.

## Affected users and systems

Contributors editing `website/`, `docs/`, or site workflows; agents following `magelift-site`; the magelift.dev site and its `/docs/` path; `.github/workflows/site.yml` (kept), `.github/workflows/docs.yml` (deleted).

## Constraints

- GitHub Pages stays the host; no URL changes (`magelift.dev`, docs under `/docs/`, `CNAME` kept).
- Smallest correct change: delete dead config, do not redesign the site or its build script.
- Human pages touched go through humanizer, then remove-ai-marks.
- The site must keep deploying: verify with a `site.yml` build (and a main-push deploy observation or an explicit reason it cannot be observed in the change).
- Verify before deleting that no active Cloudflare Pages project actually serves magelift.dev (DNS/CNAME resolution check); the file evidence says Pages is live, but deletion deserves one live confirmation.

## Out of scope

- Migrating hosting to Cloudflare Pages or anywhere else.
- Astro upgrades, landing redesign, new pages.
- Docs versioning (`mike`), link checking.
- Sitemap output churn (covered by `site-docs-hygiene`).
- Image naming or defaults (covered by `web-runtime-image-naming`).

## Open questions

- Is there any live Cloudflare Pages project still serving magelift.dev (including preview or fallback), or is GitHub Pages the sole host? Default: Pages is sole host per `publishing.md`; confirm via DNS/hosting check in spec, then delete.
- Should the `llms.txt` MIME forcing lost with `_headers` be replaced (Astro-native headers are limited on Pages; browsers sniff `.txt` as text anyway)? Default: accept sniffing, no replacement.
