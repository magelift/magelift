# Custom CLI and community providers

This example registers a community module through MageLift's public Go API. It
builds a custom binary; it does not use Go's `plugin` ABI or automatically
download an unsigned provider.

From the repository root:

```sh
go build -o /tmp/magelift-ext ./examples/custom-cli
/tmp/magelift-ext version
```

[`main.go`](main.go) calls `cli.NewWithExtensions` with [`stubModule`](stub_module.go).
The stub demonstrates registration and refuses to plan a deployment. Replace it
with an implementation of `sdk.Module` before using it as a provider. The
[adding a provider](../../docs/adding-a-provider.md) guide covers the public
extension contract. Ship and sign your custom binary if others will use it.

First-party cloud providers can be installed as release plugins. A community
provider name in `magelift.php` does not cause MageLift to download its code.
Label community and experimental integrations accurately and report the checks
actually run; no compatibility cell is certified by this example.
