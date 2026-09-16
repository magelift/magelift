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

Push to `main`. `.github/workflows/site.yml` (Public site) rebuilds Astro +
MkDocs via `website/scripts/build-site.sh` and deploys `website/dist` to
GitHub Pages with `actions/deploy-pages`. PRs build only; only a `main`
push deploys.

Custom domain: `magelift.dev` (via `website/public/CNAME`); `www` redirects
to the apex. Docs live at `/docs/` inside the same deploy — there is no
separate docs host, redirect file, or header file.

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
