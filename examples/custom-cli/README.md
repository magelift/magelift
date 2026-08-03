# Custom CLI / community provider registration

The released `magelift` binary only registers first-party modules. Community and
experimental providers ship as **compile-time custom binaries** that you link
into a custom main (ADR 0007). There is no Go `plugin` ABI and no unsigned
auto-download loader in v1.

**Honesty about `internal/`:** registration uses this module's
`internal/platform` + `internal/cli`. A separate Go module path cannot import
those packages. Do not claim publish-to-proxy.golang.org of an external module
that imports `internal/`. Use this example (or a fork) until a public platform
export exists (out of Phase 2 scope).

## Clean-cache verification

Follow only this README and [adding a provider](../../docs/adding-a-provider.md).
Do not browse `internal/cloud/**` for how to register. From the repo root:

```sh
export GOMODCACHE="$(mktemp -d /tmp/magelift-modcache.XXXXXX)"
export GOCACHE="$(mktemp -d /tmp/magelift-gocache.XXXXXX)"
GOMAXPROCS=1 GOFLAGS=-p=1 go build -o /tmp/magelift-ext ./examples/custom-cli
./magelift-ext version   # or /tmp/magelift-ext version
```

## Template

```sh
GOMAXPROCS=1 GOFLAGS=-p=1 go build -o magelift-ext ./examples/custom-cli
./magelift-ext version
```

[`main.go`](main.go) registers the same first-party modules as upstream, then
registers [`stubModule`](stub_module.go) as a community slot. The stub proves
`RegisterModule` wiring; its `Plan` refuses so it is not a fake deployable.
Replace `Plan` / `Program` with a real stack when shipping a real provider.

## Contract

1. Implement `platform.StackModule` (and optionally `HasOps`, `HasBootstrap`, …).
2. Call `modules.RegisterModule(yourModule)`.
3. Ship your custom binary (or fork); sign release artifacts with Cosign if you
   expect others to trust them.
4. Label the target **experimental** or **community** in your docs. Certification
   stays with MageLift maintainers after the shared Magento acceptance suite.

## In-tree PRs

Core accepts experimental first-party providers under `internal/cloud/<provider>/`
only when they meet [adding a provider](../../docs/adding-a-provider.md) plus
mocks/Floci where applicable. See [capability matrix](../../docs/capability-matrix.md).
