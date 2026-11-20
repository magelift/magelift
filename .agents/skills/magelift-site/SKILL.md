---
name: magelift-site
description: Edit and verify the MageLift Astro landing site, MkDocs guides, and GitHub Pages workflow.
metadata:
  version: "1.0.0"
---

# Maintain the public site

The landing page lives in `website/`. MkDocs builds `docs/` under
`website/dist/docs/` during the full site build. Put copy changes in their
source files. Use the current Humanizer skill and check provider claims
against [Provider support](../../../docs/capability-matrix.md).

```sh
npm --prefix website ci
npm --prefix website run build:site
```

Use `make docs` for a strict documentation build. Check navigation, page
anchors, and links in the landing page and `website/public/llms.txt`.
The site workflow builds pull requests and deploys pushes to `main` to
`magelift.dev`. Publication requires explicit authorization.
