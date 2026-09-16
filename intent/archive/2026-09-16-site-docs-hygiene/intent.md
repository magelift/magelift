---
status: done
slug: site-docs-hygiene
---

# Intent: site and docs hygiene fixes

## Problem

Three small hygiene bugs invite real mistakes: a generated docs page carries no do-not-edit header, the site build dirties two tracked files on every run, and one docs page is built but reachable from nowhere in the nav.

## Evidence

- `cmd/genconfig` writes `docs/configuration.md` (`{path: "docs/configuration.md", data: config.ReferenceMarkdown()}`), but the file opens with plain prose and no generated header — while `docs/cli-reference.md` ("This page is generated from the Cobra command tree. Do not edit it by hand.") and `docs/evidence/current-capability-coverage.md` ("This page is generated… Do not edit it by hand.") both carry one. A hand edit to `configuration.md` would be silently overwritten by the next `make generate`.
- `website/scripts/generate-sitemap.py` writes `sitemap.txt`/`sitemap.xml` into tracked `website/public/` (only `website/public/docs/` is gitignored), so every `npm run build:site` dirties two tracked files.
- `docs/acceptance-dependencies.md` is the only top-level `docs/*.md` not referenced in `mkdocs.yml` nav (loop over `docs/*.md` grepping basenames in `mkdocs.yml`). `mkdocs build --strict` does not fail on orphans, so it builds but is reachable only if some page links to it.

## Proposed outcome

`docs/configuration.md` carries the same do-not-edit header as the other generated pages; sitemap outputs are untracked build products that regenerate cleanly without dirtying the tree; `acceptance-dependencies.md` is either linked from the acceptance docs, added to the nav, or explicitly recorded as an intentional orphan — no longer ambiguous.

## Affected users and systems

Contributors editing docs or running `make generate` / `npm run build:site`; `cmd/genconfig` (or `internal/config.ReferenceMarkdown`); root `.gitignore`; `mkdocs.yml` nav; the acceptance docs pages.

## Constraints

- Smallest correct change per fix; no nav redesign, no sitemap content change, no generated-content change beyond the header.
- Regenerate via `make generate`; never hand-edit generated output to add the header.
- `make generate-check`, `go run ./cmd/gendocs --check` (if nav changes), and `make docs` stay green.
- Human wording touched goes through humanizer, then remove-ai-marks.

## Out of scope

- Deploy story cleanup (covered by `github-pages-deploy`).
- Image naming or defaults (covered by `web-runtime-image-naming`).
- Docs versioning (`mike`), link checking, new docs pages.
- Website landing changes beyond the sitemap-output location.

## Open questions

- Sitemap fix shape: gitignore `website/public/sitemap.{txt,xml}` (default — the build regenerates them) or redirect the generator to write into `dist/`? Spec picks; default stands if unchallenged.
- Orphan resolution: link `acceptance-dependencies.md` from the acceptance pages, add it to the nav, or record it as an intentional orphan? Default: link it if a natural parent exists, else nav it under Cloud targets; spec confirms.
