---
name: magelift-serial-builds
description: >-
  Keep MageLift Go, GoReleaser, and Docker builds serial for local smoke and
  constrained runners. Use whenever compiling, testing, packaging, or running
  release smoke outside full CI matrices.
---

# Serial builds

MageLift pulls large Pulumi provider graphs. Parallel local `go build`, full
GoReleaser multi-target matrices, and buildx multi-platform builds can exhaust
RAM on typical developer machines.

## Required for local packaging smoke

- Use `./scripts/release-smoke-local.sh` or `make release-smoke`
  (`--single-target --parallelism=1`, `GOMAXPROCS=1`, `GOFLAGS=-p=1`)
- Ad-hoc builds: prefer `GOMAXPROCS=1 GOFLAGS=-p=1` and one `go build` at a time
- Prefer `goreleaser build --single-target --parallelism=1`
- Full multi-platform matrices belong on CI runners, not local smoke scripts

## Avoid locally

- `goreleaser release` without `--single-target` (or an equivalent host-only build)
- Raising `--parallelism` above 1 for local smoke
- Concurrent heavy builds while other large compiles are already running

## CI

Hosted workflows may parallelize on GitHub runners within job limits. This skill
is about local smoke and small runners, not forbidding CI matrices.
