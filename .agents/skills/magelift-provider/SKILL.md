---
name: magelift-provider
description: Implement or maintain a first-party MageLift cloud plugin, including its runtime registration, Pulumi resources, and Floci tests.
metadata:
  version: "1.0.0"
---

# Maintain a provider plugin

Start with [Adding a provider](../../../docs/adding-a-provider.md) and the
[protocol contract](../../../docs/adr/0013-provider-plugin-contract.md).

AWS, Google Cloud, and Azure each build a separate plugin under
`providers/<name>/`. Keep resource definitions and vendor clients in that
module. Use the SDK types for messages between the plugin and CLI.

- List first-party runtimes and output keys in `internal/providerhost/standalone.go`.
  The CLI registry and provider installer both use these descriptors.
- Validate config and plan before contacting the cloud. Use account admission
  for region, identity, and resource availability checks.
- Exercise the plugin through a CLI command. Use Pulumi mocks to inspect
  resource relationships and Floci for supported cloud API calls.
- Run `make provider-tidy-check`, `make core-leanness`, and `make local-gates`.
  Update the guide and examples for the changed operation.
