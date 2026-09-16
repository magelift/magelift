# Custom CLI / community provider registration

The released `magelift` binary only registers first-party modules. Community and
experimental providers ship as compile-time custom binaries that call the
public `github.com/magelift/magelift/cli` and `sdk` packages. There is no Go
`plugin` ABI and no unsigned auto-download loader in RC1.

## Clean-cache verification

Follow only this README and [adding a provider](../../docs/adding-a-provider.md).
Do not browse `internal/cloud/**` for how to register. From the repo root:

```sh
GOMAXPROCS=1 GOFLAGS=-p=1 go build -o /tmp/magelift-ext ./examples/custom-cli
./magelift-ext version   # or /tmp/magelift-ext version
```

The custom binary links every first-party Pulumi adapter and can use several
gigabytes during compilation and linking. The routine repository check builds
[`examples/custom-extension-contract`](../custom-extension-contract) with
empty caches instead. Build the full custom binary on a worker with enough
memory, and keep the internal adapter tests in the normal Go test suite.

## Template

```sh
GOMAXPROCS=1 GOFLAGS=-p=1 go build -o magelift-ext ./examples/custom-cli
./magelift-ext version
```

[`main.go`](main.go) registers the same first-party modules as upstream, then
registers [`stubModule`](stub_module.go) as a community slot. The stub proves
`NewWithExtensions` wiring; its `Plan` refuses so it is not a fake deployable.
Replace `Plan` / `Program` with a real stack when shipping a real provider.

## Contract

1. Implement `sdk.Module` and return one target in its extension descriptor.
2. Call `cli.NewWithExtensions(yourModule)` from the custom binary.
3. Ship your custom binary (or fork); sign release artifacts with Cosign if you
   expect others to trust them.
4. Label the target **experimental** or **community** in your docs. Certification
   stays with MageLift maintainers after the shared Magento acceptance suite.

## In-tree PRs

Core accepts experimental first-party providers under `internal/cloud/<provider>/`
only when they meet [adding a provider](../../docs/adding-a-provider.md) plus
mocks/Floci where applicable. See [capability matrix](../../docs/capability-matrix.md).
