# MageLift website

The Astro landing page lives in `src/pages/index.astro`. MkDocs builds the
guides from `../docs` and the site serves them under `/docs/`.

To preview changes to the landing page, run these commands from `website/`.

```sh
npm ci
npm run dev
```

For the guides, run `make docs-serve` from the repository root. The Makefile
uses the pinned MkDocs Material environment.

Run `npm run build:site` from `website/` to build both into `dist/`. The public
site workflow deploys that directory on pushes to `main`. Check the date and
method before making claims about a cloud target. [Provider support](../docs/capability-matrix.md)
