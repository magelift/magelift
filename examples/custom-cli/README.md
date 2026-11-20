# Custom CLI and community providers

This example registers a community module through MageLift's public Go API. Compile the module into a custom CLI binary.

From the repository root:

```sh
go build -o /tmp/magelift-ext ./examples/custom-cli
/tmp/magelift-ext --output json extensions list
/tmp/magelift-ext version
```

[`main.go`](main.go) calls `cli.NewWithExtensions` with [`stubModule`](stub_module.go).
Look for `example.community-stub` in the extension list. The stub deliberately
refuses planning. A deployable cloud target also needs project validation and
the MageLift deployment operations wired into the host.
[Public extension contract](../../docs/adding-a-provider.md#develop-an-external-extension)

Use the custom binary for the extensions you link into it. Its first-party
provider installer works as usual. Report which operations you tested and how.
[Install providers](../../docs/install.md#providers)
