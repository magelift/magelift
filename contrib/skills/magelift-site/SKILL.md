---
name: magelift-site
description: >-
  Build and deploy the MageLift public site (Astro landing + MkDocs Material
  docs) to GitHub Pages. Use when editing website/, mkdocs.yml,
  docs/, or site CI.
version: 1.0.0
---

# MageLift public site

Canonical URL: `https://magelift.dev/` (docs at `/docs/`).

## Stack

- Landing page: Astro under `website/`
- Docs: MkDocs Material (`mkdocs.yml`), embedded into `public/docs/` at build
- Deploy: GitHub Pages, custom domain `magelift.dev`
- CI: `.github/workflows/site.yml` builds and deploys `website/dist`

Copy must go through humanizer, then remove-ai-marks. Do not invent certified
claims; mirror `docs/capability-matrix.md`. Schema `$id` and public links use
`magelift.dev`.

## Local build

```sh
cd website
npm run build:site
# → dist/  (docs at dist/docs/)
```

The script creates/uses repo-root `.venv` for MkDocs (do not use system pip).

## Deploy

```sh
cd website
npx wrangler pages deploy dist --project-name=magelift --branch=main
```

Custom domains: `magelift.dev`, `www.magelift.dev`. Prefer `/docs/` over a
separate docs hostname.

## Copy rules

- Do not invent certified claims; mirror `docs/capability-matrix.md`
- Keep landing and README free of AI-tell clusters (em dashes, “seamless”,
  fake significance)
- Schema `$id` and public links use `magelift.dev`

## Use this skill when

- You are changing website/, docs/, mkdocs.yml, or site workflows.
- A public claim about a provider, compatibility row, or certification changes.

## Leave behind

- A strict docs build result.
- Public wording that matches the capability matrix and names experimental gaps.
