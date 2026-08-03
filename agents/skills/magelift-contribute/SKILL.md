---
name: magelift-contribute
description: >-
  Contribute to MageLift: layout, make verify, PR expectations, and honesty
  rules for certified vs experimental cells. Use when writing code, docs, or
  tests for this repository, opening a PR, or reviewing contributor work.
---

# Contribute to MageLift

## Defaults

1. Smallest correct change. Touch only what the task needs.
2. Do not invent certified claims. Certified targets today: AWS ECS Fargate and
   GCP GKE Autopilot. EKS / OVH / Scaleway stay experimental until the matrix
   says otherwise (`docs/capability-matrix.md`).
3. Prefer existing docs and examples over new prose. Update
   `docs/capability-matrix.md` when status changes.

## Layout

| Path | Role |
| --- | --- |
| `cmd/magelift` | Production CLI registration (modules wired here) |
| `internal/cli` | Cobra commands; keep Pulumi SDKs out |
| `internal/platform` | Cross-provider ports the CLI uses |
| `internal/cloud/<provider>/` | Adapter internals (do not leak into CLI) |
| `sdk/v1` | Typed topology/validation contracts |
| `docs/` | Human docs (MkDocs) |
| `agents/skills/` | First-party agent skills |
| `websites/marketing/` | Public site (Astro + embedded MkDocs) |

## Verify before you claim done

```sh
# After clone, once:
composer install --working-dir=build

make verify
```

Narrow loops while iterating:

```sh
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/<pkg>/ -count=1
make docs
```

For heavy local compiles, load `magelift-serial-builds` and keep packaging smoke
single-target.

## PR shape

- What changed, how you verified, risk note
- Link the issue when there is one
- Docs/test-only PRs should say so

## Anti-patterns

- Marking experimental cells as production-ready
- Registering only `infra.RegisterTarget` without `cmd/magelift` module wiring
- Committing `.claude/`, `.cursor/`, `.agents/`, or vendored third-party skills
- Running full multi-platform GoReleaser matrices as a local smoke test
