# MageLift site (Astro + MkDocs)

Public surface for https://magelift.dev/

- Landing page: Astro (`src/pages/index.astro`)
- Docs: MkDocs Material, built into `public/docs/` then shipped in the same
  Cloudflare Pages deploy

## Develop the landing page only

```sh
cd website
npm install
npm run dev
```

Docs locally: from repo root, `make docs-serve` (pinned MkDocs Material in the
repo venv — a system/Homebrew mkdocs does not bundle the theme).

## Production build (landing + docs)

```sh
cd website
npm run build:site
# → dist/  (docs at dist/docs/)
```

Deploy `dist/` to the Cloudflare Pages project `magelift`.

Do not invent certified claims. Copy status wording from
[docs/capability-matrix.md](../../docs/capability-matrix.md).
