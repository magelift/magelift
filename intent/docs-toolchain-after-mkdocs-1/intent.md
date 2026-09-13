---
status: draft
slug: docs-toolchain-after-mkdocs-1
---

# Intent: leave MkDocs 1.6 before the 1.x core is the risk

## Problem

We cannot take MkDocs 2. Material for MkDocs will never run on it (plugins gone, theming rewritten, no migration). Magelift is pinned to `mkdocs==1.6.1` and `mkdocs-material==9.7.7`. MkDocs 1.x is unmaintained. Site builds already print Material's MkDocs 2 warning. The 2026-09-12 bump left this on purpose.

## Evidence

`.agents/knowledge/lessons/MkDocs 2.0 breaks Material - docs toolchain exit options.md` (2026-08-04): stay put, revisit in about six months. Exit options recorded: Zensical (Material team's SSG) or Astro Starlight. `npm run build:site` on 2026-09-12 still printed the Material MkDocs 2 warning and succeeded. MkDocs 2 release date and Zensical drop-in status: not checked since that lesson.

## Proposed outcome

`make docs` / `website` docs build uses a toolchain that is maintained and still matches the capability matrix. Magelift.dev `/docs/` URLs and nav stay. The MkDocs 2 warning is gone because we are not on MkDocs 1 + Material, or because Material's successor replaced it.

## Affected users and systems

Public docs on magelift.dev, `docs/requirements.txt`, `website/scripts/build-site.sh`, GitHub Pages site workflow. Contributors running `make docs`.

## Constraints

Do not invent certified claims. Copy still goes through humanizer then remove-ai-marks. Exact pins until the cut. No MkDocs 2 while Material is the theme. Deferred until after the v1 stable cut (see ROADMAP.md): the pinned toolchain builds green today and a docs migration before the tag adds risk for no user gain.

## Out of scope

Taking MkDocs 2 under Material. Rewriting the Astro landing except as needed to share a docs design system. PHP or Go dependency policy.

## Open questions

Zensical vs Starlight now that we are past the August 2026 revisit window? Hash-pin `docs/requirements.txt` as part of this cut or separately?
