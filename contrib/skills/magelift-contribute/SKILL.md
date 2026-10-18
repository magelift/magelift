---
name: magelift-contribute
description: >-
  Contribute to MageLift: layout, make verify, PR expectations, and honesty
  rules for certified vs experimental cells. Use when writing code, docs, or
  tests for this repository, opening a PR, or reviewing contributor work.
version: 1.0.0
---

# Contribute to MageLift

## Defaults

1. Smallest correct change. Touch only what the task needs.
2. Bound claims to the exact recipe and evidence in `docs/capability-matrix.md`
   and `docs/evidence/README.md`. Historical certification does not prove a new
   alpha build. EKS / OVH / Scaleway / GKE Standard stay experimental. Milestone
   A requires the GCP recipe, not exhaustive provider certification.
3. Prefer existing docs and examples over new prose. Update the matrix and the
   current evidence file when status changes.
4. When a public contract, certification tier, or topology rule changes, update
   the ADR and the human page in the same change. Human docs and website copy
   go through humanizer, then remove-ai-marks.
5. Layout is the contributor map in `CONTRIBUTING.md`.
6. When a change adds or alters CLI behavior or `magelift.yaml` keys, update
   the matching user skill in the same change so skills never drift behind
   the CLI they describe.

## Verify before you claim done

```sh
# After clone, once:
composer install --working-dir=build

make verify-community
```

The community gate covers the root, SDK and GCP modules independently; nested
modules are not covered by root `go test ./...`. `make test-fast` runs the three
Go suites without race. `make test-race` runs bounded race checks. The broader
`make verify` suite remains available; it is not the alpha publication threshold.

Narrow loops while iterating:

```sh
go test ./internal/<pkg>/ -count=1
make docs
```

Do not pin `GOMAXPROCS` or `-p`. Makefile sets `GOMEMLIMIT` to 75% of
available RAM. Local packaging smoke uses the dialproof (host-only)
GoReleaser config; the full matrix stays on CI.

## PR shape

- What changed, how you verified, risk note
- Link the issue when there is one
- Docs/test-only PRs should say so

## Anti-patterns

- Marking experimental cells as production-ready
- Registering only `infra.RegisterTarget` without `cmd/magelift` module wiring
- Committing `.claude/`, `.cursor/`, `.agents/` caches, or vendored third-party skills
- Omitting `.agents/knowledge/` (that bundle is tracked; its authoring rules are `.agents/knowledge/CONVENTIONS.md`)
- Running full multi-platform GoReleaser matrices as a local smoke test
- Filing session resumes or KEEP run IDs into `.agents/knowledge/`
- Using a private sibling Magento shop as certification evidence or as the
  only supported AWS architecture

## Use this skill when

- You are changing MageLift code, configuration, docs, or tests.
- You need to decide whether a provider cell is certified or experimental.
- You are preparing a review and need the repository verification gates.

## Leave behind

- A focused diff with tests or a clear reason a test cannot run.
- Updated ADRs and human docs when a decision or claim changes.
- No credentials, generated agent caches, or unverified certification claims.
