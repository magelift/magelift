# Lint policy

CI runs golangci-lint with a narrow, high-signal set so Pulumi-heavy graphs stay
tractable. Expand deliberately; do not dump every available linter into CI.

## Enabled (required)

| Linter | Role |
| --- | --- |
| `govet` | Compiler-adjacent correctness |
| `staticcheck` | Bug-finding (selected checks; see `.golangci.yml`) |
| `ineffassign` | Ineffectual assignments |
| `misspell` | Identifier/comment typos |
| `unused` | Dead code |
| `errcheck` | Unchecked errors |
| `revive` | Style subset (narrow rules in `.golangci.yml`) |
| `gocritic` | Selected diagnostics (`#diagnostic` only; opinionated off) |

`govulncheck` runs as a separate CI step inside `go-verify` (not inside golangci-lint).

## CI partitioning (QUALITY-06)

Lint no longer runs as one `golangci-lint run ./...` job. The import graph forces
six partitions in `.github/workflows/ci.yml`:

| Partition | Packages | Why separate |
| --- | --- | --- |
| `aws` | `./internal/cloud/aws/...` | Largest Pulumi AWS SDK graph |
| `gcp` | `./internal/cloud/gcp/...` | Independent GCP SDK graph |
| `ovh` | `./internal/cloud/ovh/...` | Independent OVH SDK graph |
| `scaleway` | `./internal/cloud/scaleway/...` | Independent Scaleway SDK graph |
| `core` | Explicit list of everything else (see workflow) | Go patterns cannot subtract `./internal/cloud/...` from `./internal/...` |
| `aggregate` | `./cmd/magelift/...` | Only binary that imports all four providers; irreducible |

Ordered fallbacks when a partition OOMs or the runner is reaped (do **not** raise
timeouts as the first response. A dead runner with no lint output is a memory
signal, not a slow one):

1. Keep `cache-prime` as module-download only (compiling the full module was
   SIGTERM'd on private runners).
2. Serialize the lint matrix (`max-parallel: 1`) and run `go-verify` after lint.
3. Soft-fail `cache-prime` so a cold matrix still gates merges.
4. If a single partition still dies, shrink that partition's package list further
   before touching timeouts.

Coverage guard: `go test ./internal/lintcoverage/` asserts the union of matrix
patterns equals `go list ./...` (minus `.golangci.yml` path exclusions). A new
top-level package that joins the module without joining a partition fails that
test.

### Force-all verification run

Path filters skip `php`, `docs`, and `workflow-lint` on Go-only changes. To
reproduce a full `make verify` mapping in one CI execution, dispatch with the
`all` input:

```bash
gh workflow run ci.yml --ref main -f all=true
```

Map jobs → verify targets: `generate-check` / `cli-docs-check` / `fmt-check` /
`test` / `license-check` in `go-verify`; `php-test` in `php`; `docs` in `docs`;
`workflow-check` in `workflow-lint`.

**force-all proof:** Hosted Actions minutes are available on the public repo.
Record the latest force-all URL and per-target outcomes below when a
`workflow_dispatch` with `all=true` completes.

```bash
gh workflow run ci.yml --ref main -f all=true
```

Map jobs → verify targets: `generate-check` / `cli-docs-check` / `fmt-check` /
`test` / `license-check` in `go-verify`; `php-test` in `php`; `docs` in `docs`;
`workflow-check` in `workflow-lint`.

| Run | Date | Outcome |
| --- | --- | --- |
| [30836267745](https://github.com/magelift/magelift/actions/runs/30836267745) | 2026-08-03 | force-all green (lint, go-verify, images, varnish, floci, php, docs) |

## `go test -race` measurement

Criterion 1 requires `go test -race ./...` inside `go-verify`. `-race` roughly
doubles per-package memory over a graph that peaks near 8.5 GB when provider SDKs
link into one binary, so fit on the **2 vCPU / 7 GB** runner class must be
measured, not assumed.

| Environment | Command | Wall time | Peak RSS | Outcome |
| --- | --- | --- | --- | --- |
| Local (serial sample) | `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/lintcoverage/ ./internal/usererr/ -count=1` | ~4.7s | ~102 MiB | pass (narrow sample only) |
| Local (full race) | `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./... -count=1` | deferred-local | deferred-local | Prefer CI; see `agents/skills/magelift-serial-builds` for local smoke guidance |
| CI `go-verify` (2 vCPU / 7 GB) | `go test -race ./...` | ~12m | n/a | pass on run 30836267745 |

If CI fails on memory or the runner is reaped with no test output, partition the
test step by the same six package groups as the lint matrix; do not raise
`timeout-minutes` as the first fix. Record the partitioning reason here when that
happens.

## Deferred linters

| Linter | Why deferred |
| --- | --- |
| `gocyclo` / `cyclop` | Pulumi composition graphs trip low thresholds; revisit with a high bar |
| `gosec` | Prefer `govulncheck` + CodeQL + Trivy for security signal |
| `wrapcheck` | Noisy across CLI/`fmt.Errorf` boundaries |

## Suppressions

Prefer fixing the finding. If a false positive is unavoidable, use a scoped
`//nolint:<linter>` with a short why on the same line. Package-wide nolint is
not allowed without an ADR-linked rationale.
