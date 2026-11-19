---
name: magelift-provider
description: Add a cloud adapter, register its runtimes and test the provider boundary.
version: 1.0.0
---

# Adding a provider

Start in the [provider guide](../../../docs/adding-a-provider.md). AWS, GCP and Azure each build a separate plugin. OVH and Scaleway are existing in-process experimental adapters. Each provider owns its Pulumi resources.

1. Define the provider and runtime IDs, supported operations and config. Use the public SDK types for the plugin protocol. Keep vendor resource types inside the adapter.
2. Implement planning and infrastructure in `providers/<name>/`. A new plugin has its own Go module. Add it to the workspace, build, distribution and tests. See the [plugin contract](../../../docs/adr/0013-provider-plugin-contract.md).
3. Register each first-party runtime with `platform.ModuleRegistry` in `internal/registry`. The CLI resolves bootstrap, preview, deploy and day-two operations there. The discovery index in `internal/infra` serves a different purpose.
4. Use Pulumi mocks for resource relationships and Go tests for validation and operations. Exercise SDK calls against Floci where it implements the API. Record which cases ran in the [capability matrix](../../../docs/capability-matrix.md).
5. Run `make generate` for config changes. Update the shop guide and the example for the affected runtime.
