# MageLift site (Astro + MkDocs)

Public surface for https://magelift.dev/

- Marketing landing: Astro (`src/pages/index.astro`)
- Docs: MkDocs Material, built into `public/docs/` then shipped in the same
  Cloudflare Pages deploy

## Develop marketing only

```sh
cd websites/marketing
npm install
npm run dev
```

Docs locally: from repo root, `mkdocs serve` (separate from Astro).

## Production build (marketing + docs)

```sh
cd websites/marketing
npm run build:site
# → dist/  (docs at dist/docs/)
```

Deploy `dist/` to the Cloudflare Pages project `magelift`.

Do not invent certified claims. Copy status wording from
[docs/capability-matrix.md](../../docs/capability-matrix.md).
