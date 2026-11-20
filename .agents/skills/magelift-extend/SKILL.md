---
name: magelift-extend
description: Add a provider module or service adapter to a custom MageLift binary through the public Go SDK.
metadata:
  version: "1.0.0"
---

# Extend MageLift

Use the [custom CLI example](../../../examples/custom-cli/README.md) and the
[provider guide](../../../docs/adding-a-provider.md). Register a provider
module with `cli.NewWithExtensions` or a service connection adapter with
`cli.NewWithServiceAdapters`.

Declare the extension ID, version, source, targets, and supported operations.
Keep cloud clients and Pulumi resource types in the extension. The shop puts
namespaced options in `extensions.<vendor>` and uses references for secrets.

Check that `magelift extensions list` reports the declared module. Run
`make extension-test` to build the SDK consumer against a published-style
Go module. Use Pulumi mocks to check cloud resources owned by the extension.
