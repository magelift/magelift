# Custom CLI / community provider registration

The released `magelift` binary only registers first-party modules. A community
provider publishes a Go module; you build a small main that calls
`platform.ModuleRegistry.RegisterModule`.

Sketch:

```sh
go run ./examples/custom-cli
```

See [Adding a provider](../../docs/adding-a-provider.md) and ADR 0007. v1.1 has
no unsigned dynamic plugin loader.
