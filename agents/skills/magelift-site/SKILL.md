---
name: magelift-site
description: >-
  Build and deploy the MageLift public site (Astro landing + MkDocs Material
  docs) to Cloudflare Pages. Use when editing website/, mkdocs.yml,
  docs/, or site CI.
---

# MageLift public site

Canonical URL: `https://magelift.dev/` (docs at `/docs/`).

## Stack

- Landing page: Astro under `website/`
- Docs: MkDocs Material (`mkdocs.yml`), embedded into `public/docs/` at build
- Deploy: Cloudflare Pages project `magelift`
- CI: `.github/workflows/site.yml` builds and uploads `website/dist`

`docs/knowledge/**` is excluded from the public docs build.

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
