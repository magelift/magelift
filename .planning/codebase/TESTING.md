# Testing Patterns

**Analysis Date:** 2026-07-27

## Test Framework

**Runner:**
- Standard library `testing` package only. No testify, no ginkgo, no gomock detected anywhere in the repo (`grep -rl testify` returns no results).

**Assertion Library:**
- None — plain `if got != want { t.Fatalf(...) }` / `t.Fatal(...)` checks throughout.

**Run Commands:**
```bash
make test          # go test with the race detector (Makefile:14)
make lint          # gofmt -l check + golangci-lint (Makefile, .golangci.yml)
make floci-test    # Account-free AWS state/lock integration tests via Floci (Makefile:63)
make image-test    # Build and inspect the local PHP runtime image
make build-e2e-test # Build the fixture through CLI, Docker, and BuildKit (Makefile:60)
```

## Test File Organization

**Location:**
- Co-located with source, same package (white-box), e.g. `internal/cloud/aws/queue/component_test.go` next to `component.go`.
- Separate integration suite under `tests/floci/` (e.g. `tests/floci/storage_test.go`, `tests/floci/state_test.go`, `tests/floci/ecs_test.go`, `tests/floci/operations_test.go`) — these are account-free AWS state/lock integration tests, run via `make floci-test`.
- SDK/public-surface tests under `sdk/v1/` (`sdk/v1/topology_test.go`, `sdk/v1/validation_test.go`).

**Naming:**
- `<source>_test.go` per source file, or a consolidated `<domain>_test.go` covering a whole package (e.g. `internal/config/config_test.go` covers config loading, resolution, and validation together).
- Test functions: `func Test<Behavior>(t *testing.T)` describing the scenario in plain English, e.g. `TestResolveMergeAndProvenance`, `TestRejectsUnsafeProductionInputsBeforeRegistration`, `TestProductionUsesEncryptedThreeNodeCluster`. Names read as behavior specifications, not just method names under test.

## Test Structure

**Suite Organization (flat, one func per scenario):**
```go
func TestResolveAppliesBoundedDefaultsWhenAWSTargetIsConfigured(t *testing.T) { ... }
func TestResolveDoesNotInventAWSTargetForMinimalConfig(t *testing.T) { ... }
```
(`internal/config/config_test.go`)

**Table-driven subtests for input-variation scenarios**, using `t.Run`:
```go
tests := []struct {
    name   string
    mutate func(*Args)
}{
    {name: "two zones", mutate: func(args *Args) { ... }},
    {name: "plaintext password", mutate: func(args *Args) { args.Credentials.SecretARN = "password" }},
}
for _, test := range tests {
    t.Run(test.name, func(t *testing.T) {
        args := base
        // deep-copy slice fields before mutating to avoid cross-case pollution
        args.AvailabilityZones = append([]string(nil), base.AvailabilityZones...)
        test.mutate(&args)
        ...
    })
}
```
(`internal/cloud/aws/queue/component_test.go:77-107`) — 51 occurrences of this `tests := []struct{...}` / `for _, tt/tc/test := range` pattern repo-wide.

**Patterns:**
- Fixture YAML/config strings defined as package-level `const base = ...` at top of test file (`internal/config/config_test.go:8-27`), reused across multiple `Test*` functions in that file.
- Assertions use `t.Fatalf("field = %q/%v", got, ...)` immediately after each check — fail fast, one condition per `if`.

## Mocking

**Framework:** Pulumi's own `pulumi.WithMocks` test harness — no third-party mock library.

**Patterns:**
```go
type mocks struct {
    mu        sync.Mutex
    resources []pulumi.MockResourceArgs
    invokes   []pulumi.MockCallArgs
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
    m.mu.Lock()
    m.resources = append(m.resources, args)
    m.mu.Unlock()
    state := args.Inputs.Copy()
    if args.TypeToken == "aws:mq/broker:Broker" {
        state["arn"] = resource.NewStringProperty(...)
        // synthesize provider-computed outputs the real API would return
    }
    return args.Name + "-id", state, nil
}

func (m *mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
    m.mu.Lock()
    m.invokes = append(m.invokes, args)
    m.mu.Unlock()
    return resource.PropertyMap{"secretString": resource.MakeSecret(...)}, nil
}
```
(`internal/cloud/aws/queue/component_test.go:14-40`)

- Tests invoke the component under test via `pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, name, args); return err }, pulumi.WithMocks("magelift", "test", m))`, then assert on `m.resources` / `m.invokes` recorded by the mock.
- Helper methods `m.count("type:token")` / `m.resourcesOf("type:token")` filter recorded resources by Pulumi type token for assertions — implement similar helpers when writing new component tests to avoid ad hoc filtering per test.
- Resource inputs are inspected by JSON-marshaling `brokers[0].Inputs.Mappable()` and asserting on substring presence in the encoded text — a pragmatic but string-matching approach to structural assertions; prefer this pattern for consistency in new Pulumi component tests rather than introducing a JSON-path/reflection library.
- Secrets are asserted via `resource.PropertyValue.IsSecret()` — always verify sensitive fields (passwords, keys) are wrapped with `resource.MakeSecret(...)` in mocks and checked with `IsSecret()` in tests.

**What to Mock:**
- Only the Pulumi provider boundary (`NewResource`/`Call`) — component logic itself (validation, args construction) runs for real against the mock provider.

**What NOT to Mock:**
- Internal packages (config parsing, validation) are tested directly against real logic with real fixture YAML — no mocking of internal collaborators.

## Fixtures and Factories

**Test data:**
- Inline YAML string constants for config tests (`internal/config/config_test.go:8-27`).
- Factory functions building known-good baseline args for a domain, then mutated per test case, e.g. `productionArgs() Args` in `internal/cloud/aws/queue/component_test.go:109-116` — call this pattern "valid baseline + targeted mutation" when adding new Pulumi component tests.

**Location:** Fixtures live inline in the `_test.go` file that uses them; no shared `testdata/` fixture directory pattern detected for these packages (check `tests/floci/` separately for integration fixtures).

## Coverage

**Requirements:** Not enforced via a configured coverage threshold in CI config observed; `make test` runs with the race detector but no `-cover` flag baked into the target definition line (`Makefile:14`) — check current `Makefile` test target and `.github/workflows/ci.yml` for any coverage gate before assuming none exists in CI.

**View Coverage:**
```bash
go test ./... -race -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Test Types

**Unit Tests:**
- Package-scoped tests using Pulumi mocks for infra components (`internal/cloud/**/*_test.go`), and pure-logic tests for config/schema/validation (`internal/config/*_test.go`, `internal/topology/presets_test.go`, `internal/usererr/usererr_test.go`).

**Integration Tests:**
- `tests/floci/*_test.go` — account-free AWS state and lock integration tests, run separately via `make floci-test`, exercising storage/state/ECS/operations behavior against Floci rather than live AWS.

**E2E Tests:**
- `make build-e2e-test` builds a fixture app through the CLI, Docker, and BuildKit end-to-end (`Makefile:60`).
- Image-level smoke/health tests via shell scripts: `scripts/image-health-test.sh`, `scripts/varnish-test.sh`, `scripts/release-smoke-local.sh` — not Go tests, invoked from Make targets (`make image-test`, `make varnish-test`, etc.).

## Common Patterns

**Error-path testing (reject-before-register):**
```go
err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("magelift", "test", m))
if err == nil {
    t.Fatal("unsafe queue arguments were accepted")
}
if len(m.resources) != 0 {
    t.Fatal("resources registered before validation")
}
```
(`internal/cloud/aws/queue/component_test.go:98-104`) — the standard shape for validating that Args validation happens before any side effect.

**Structured user-error testing:**
```go
err := usererr.Wrap(inner, "cannot reach AWS", "check AWS_PROFILE and network", "docs/bootstrap.md")
if !errors.Is(err, inner) {
    t.Fatalf("errors.Is failed: %v", err)
}
```
(`internal/usererr/usererr_test.go:19-25`) — always verify `errors.Is`/`Unwrap` chain integrity when adding new wrapped-error constructors.

---

*Testing analysis: 2026-07-27*
