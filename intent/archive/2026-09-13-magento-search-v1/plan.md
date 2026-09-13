---
status: done
slug: magento-search-v1
spec: spec.md
---

# Plan: search proof shapes and spend cap (decision half)

Auto-approved per the standing `/goal` instruction.

## Files that change

- NEW `docs/adr/0012-search-proof-shapes.md`: shapes, cap, criteria,
  evidence, Phase 2 handoff. EDIT `docs/adr/README.md` index.
- EDIT `mkdocs.yml`: nav entry for ADR 0012.
- EDIT `ROADMAP.md`: order 9 points at `magento-search-live-proof`.

## Order of work

- [x] 1.1 Write ADR 0012 with priced shapes, cap, criteria, and
  evidence checklist, humanizer applied — verify: re-read against
  each spec scenario
- [x] 1.2 Index plus nav plus roadmap pointer — verify: `make docs`
  strict build green
- [x] 1.3 Verify and archive — verify: `report.md` with `verdict: pass`

## Risks

- List prices move: ADR cites sources and dates the numbers; Phase 2
  re-checks before the run.

## Proof

ADR 0012 merged, `make docs` green, report archived with this folder.
No cloud spend: this pass creates no resources.
