# Coding Conventions

**Analysis Date:** 2026-07-27

## Naming Patterns

**Files:**
- Snake-free, lowercase, package-scoped: `component.go`, `config.go`, `ops.go`, `spec.go` inside per-domain packages (e.g. `internal/cloud/aws/stack/component.go`).
- Tests co-located as `<name>_test.go` in the same package (white-box tests), e.g. `internal/cloud/aws/queue/component_test.go`.

**Packages:**
- One responsibility per package under `internal/cloud/<provider>/<domain>/` (e.g. `internal/cloud/aws/queue`, `internal/cloud/aws/security`, `internal/cloud/aws/stack`). Package names are short nouns (`queue`, `stack`, `runtime`, `security`, `ops`).

**Functions:**
- Constructors named `New` (Pulumi component constructors: `func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error)`).
- Exported `Args` struct is the standard parameter-object pattern for component constructors (see `internal/cloud/aws/queue/component.go`).

**Types:**
- Config/schema structs use PascalCase field names with parallel `yaml`/`json` struct tags and a project-specific `config:`/`schema:` tag for doc generation and JSON-Schema constraints, e.g. `internal/config/model.go`:
```go
SchemaVersion int `yaml:"schemaVersion" json:"schemaVersion" config:"MageLift configuration schema version" schema:"const=1"`
```

**Errors:**
- `revive` linter has `error-naming` and `error-strings` rules disabled — sentinel/error string casing is not enforced by lint; still, exported sentinel errors follow `Err...` where present. Custom structured error type lives in `internal/usererr/usererr.go`.

## Code Style

**Formatting:**
- `gofmt` is the only formatter; `make fmt` runs `gofmt -w` over all non-vendor `.go` files, and `make lint`/CI enforce `gofmt -l` returns empty (`Makefile:9`, `Makefile:12`). `gofmt` is also enabled as a `golangci-lint` linter (`.golangci.yml:114`).
- No `goimports`, no `gofumpt` — plain `gofmt` only.

**Linting:**
- `golangci-lint` v2 config at `.golangci.yml`. Enabled linters: `govet`, `staticcheck`, `ineffassign`, `misspell`, `unused`, `errcheck`, `revive`, `gocritic`, `gofmt`.
- `staticcheck` has stylistic checks disabled (`-ST1000`, `-ST1003`, `-ST1005`, `-QF1001`, `-QF1008`, `-S1002`, `-SA1019`) — naming/doc-comment style and deprecated-API warnings are intentionally not enforced (Pulumi SDKs frequently deprecate/replace APIs).
- `revive` runs a narrow rule subset only (blank-imports, context-as-argument, error-return, errorf, indent-error-flow, range, receiver-naming, time-naming, unexported-return, increment-decrement) — capitalization/exported/var-naming/package-comments rules are explicitly disabled to avoid churn across large Pulumi resource graphs.
- `gocritic` diagnostic tag enabled; several checks disabled (`hugeParam`, `rangeValCopy`, `sloppyReassign`, `appendAssign`, `dupImport`, `ifElseChain`, `elseif`, `unlambda`, `assignOp`) — tolerates large value copies and reassignment patterns common in Pulumi args structs.
- `errcheck` ignores a small allowlist of best-effort calls (`cobra.Command.Help`, `MarkFlagRequired`, YAML encoder `Close`, `fmt.Fprint(f)`).
- CI lint is **partitioned** across six matrix jobs (`aws`, `gcp`, `ovh`, `scaleway`, `core`, `aggregate`) in `.github/workflows/ci.yml`. `.golangci.yml` no longer sets `run.concurrency` or a 30-minute `run.timeout` — job-level `timeout-minutes` and `max-parallel: 1` on the matrix replace those workarounds. See `docs/lint-policy.md`.
- Coverage guard: `go test ./internal/lintcoverage/` fails if the matrix union stops covering `go list ./...`.
- **Local machine still serial for heavy Go work:** never parallelize `go build` / `go test -race ./...` / goreleaser on the 16 GB Mac (AGENTS.md kernel-panic history). Prefer `GOMAXPROCS=1 GOFLAGS=-p=1` and narrow package sets under Cursor.

## Import Organization

**Order:**
- Standard library first, then third-party (Pulumi SDKs, AWS SDK v2, etc.), each as its own `import (...)` group separated by a blank line — standard `gofmt`/`goimports`-style grouping observed throughout (e.g. `internal/cloud/aws/queue/component_test.go`).
- Aliased imports used when package name collides or is ambiguous, e.g. `awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"`.

**Path Aliases:** None — Go module import paths only (`github.com/acourtiol/magelift/...`), no build-time path aliasing.

## Error Handling

**Two error tiers:**
1. **Internal/library errors** — plain wrapped errors (`fmt.Errorf("...: %w", err)`), returned from Pulumi component constructors and internal packages.
2. **User-facing CLI errors** — structured via `internal/usererr` (`internal/usererr/usererr.go`). `usererr.Error` carries `Cause`, `Next` (actionable step), and `Doc` (docs anchor), and implements `Unwrap()` so `errors.Is`/`errors.As` still traverse the chain. Constructors:
   - `usererr.New(cause, next, doc)` — no underlying error.
   - `usererr.Wrap(err, cause, next, doc)` — wraps an existing error, preserving the cause chain.
   - `usererr.Format(next, doc, format, args...)` — `fmt.Sprintf`-style cause message.
   - `usererr.As(err)` — typed extraction helper wrapping `errors.As`.
- Validation failures in Pulumi components are returned **before** any resource registration (see `internal/cloud/aws/queue/component.go` — invalid `Args` reject early, verified by `TestRejectsUnsafeProductionInputsBeforeRegistration` in `internal/cloud/aws/queue/component_test.go:77-107`, which asserts zero resources were registered on error).

## Comments

**Package doc comments:** Present at top of key packages explaining intent and tone, e.g. `internal/usererr/usererr.go:1-3` explains the package exists so CLI stderr is actionable without stack-trace spelunking.

**Inline comments:** Sparse; used to explain *why*, not *what* — e.g. CI/lint config comments justify disabled rules and CI-specific timeouts (`.golangci.yml`, `internal/cli/*`).

## Function Design

**Constructors:** Component constructors follow Pulumi's convention: `func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error)`, validating `Args` up front and returning an error before touching Pulumi state.

**Parameters:** Multi-field inputs are grouped into an exported `Args`/`Config` struct rather than long parameter lists (`Args{Mode, Topology, Region, ...}` in `internal/cloud/aws/queue/component.go`).

## Module Design

**Domain-per-package:** Each cloud provider has parallel subpackages for the same concerns (`stack`, `runtime`, `security`, `queue`, `ops`) under `internal/cloud/<provider>/`, keeping provider-specific Pulumi logic isolated (see AWS: `internal/cloud/aws/stack`, `internal/cloud/aws/runtime`, `internal/cloud/aws/security`, `internal/cloud/aws/queue`, `internal/cloud/aws/ops`; parallel structure exists for `gcp`, `ovh`, `scaleway`).

**Config schema:** `internal/config/model.go` defines the full user-facing YAML/JSON schema as plain Go structs with parallel struct tags feeding both marshaling and JSON-Schema generation (`schema/magelift.schema.json`) — when adding a config field, add it here with all three tag families (`yaml`, `json`, `config`/`schema` where applicable).

---

*Convention analysis: 2026-07-27*
