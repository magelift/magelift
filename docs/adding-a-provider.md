# Adding a provider

AWS, Google Cloud and Azure use the same plugin contract. The CLI evaluates the shop project and coordinates the deployment. The plugin plans and operates the cloud resources. Use the AWS module for a multi-runtime example, GCP for Kubernetes operations, or Azure for a smaller implementation. [Provider source](https://github.com/magelift/magelift/tree/main/providers) · [Protocol decision](adr/0013-provider-plugin-contract.md)

## Start a first-party module

1. Create `providers/<name>/go.mod` and add the directory to `go.work`. Implement the executable under `cmd/magelift-provider-<name>/` inside that module. Keep resource definitions, native clients, stack plans and operations in the provider.
2. Implement the SDK handshake and `Describe`. Return the provider ID, version, runtimes, output keys and the version of each supported operation. The host requires planning, Pulumi lifecycle, state locking and Magento deployment phases for each first-party deployment plugin. It checks optional operations such as cost and media transfer when a command requests them. Add an operation to the SDK only when a typed contract is needed by both sides.
3. Implement config validation and planning. Preserve the project, account or subscription, region, runtime, service bindings, environment identity and protection flags in the plan. Ordinary planning should run offline. Put account or regional availability checks in the admission step that precedes a cloud change.
4. Keep Pulumi resource creation, updates, outputs and destroys in the plugin. Wire up bootstrap, state locks, secrets, media, logs and deployment phases for the operations the provider implements. User-facing outputs must preserve secret markings. A destroy must select the existing stack and respect environment protection.
5. Add each runtime to `internal/providerhost/standalone.go`. The registry uses that list to register the runtime and the installer uses it to find the provider. The SDK discovery index in `internal/infra` lists capabilities. Check a real CLI command to exercise runtime registration.
6. Add the provider to the signed release build. Add matching `provider-<name>` and `floci-<name>` jobs, a `make provider-<name>-test` target and a `make floci-<name>-test` target. Keep the Compose definition and test entry point in the module. [Release procedure](release-readiness.md) · [Signature policy](adr/0014-distribution-trust.md)

The root CLI and the plugin exchange versioned messages over local net/rpc. Keep cloud SDK types off the wire. The plugin runs under the same operating-system identity as the CLI. A plugin installation therefore requires a verified artifact from a trusted publisher.

For a CDN purge, list the public stack outputs your adapter needs in `EdgeAdapterDescriptor.PurgeOutputKeys`. The CLI reads those outputs from the deployed environment and passes them to the plugin. Before invalidating the cache, the plugin must check that the resource belongs to that shop and environment.

## What to test

- Plan a synthetic shop through the plugin executable and check that its selected runtime, services, image digest and environment survive the round trip.
- Use Pulumi mocks to inspect resource types, relationships, secret inputs and exported values. Return the applicable keys from `platform.RequiredOutputKeys()` plus any extra outputs the provider needs.
- Test interrupted deployments. After a second deployment acquires the lock, the first deployment’s release callback must leave the new lock in place. A provider that restarts must still be able to release its own lock using the original receipt.
- Exercise cloud API calls against Floci where the emulator implements them. Test partial failures, retries and cleanup in Go for the remaining operations. Record the test method and its scope in the capability guide. [Provider evidence](capability-matrix.md)
- Run the release module build outside the Go workspace and check that the core CLI dependency graph excludes the provider. Run `make provider-tidy-check`, `make core-leanness` and `make local-gates` from the repository root.

## Project settings and shared operations

Portable service bindings describe database, cache, sessions, search, queue and media. Each adapter maps these selections onto resources or existing services. The shop owns the versions. Keep provider-specific settings under the provider’s target or a named extension. Generate the config schema and CLI reference with `make generate` after changing a public field or command. [Service bindings](service-bindings.md)

Kubernetes runtimes can reuse the Magento rollout and workload code in `internal/cloud/kube`. S3-compatible backends can reuse `internal/s3state`. Fastly and New Relic have their own adapters under `internal/external`. The first-party modules share some implementation packages with the root Go module, so release builds pin the matching root and SDK revisions.

## Community extensions

A community binary can register an `sdk.Module` with `cli.NewWithExtensions`. For a vendor that supplies a service connection to an existing runtime, implement `sdk.ServiceAdapter` and pass it to `cli.NewWithServiceAdapters`. A shop can select the adapter in PHP with `new Service(family: 'hosted-search', version: '3.1', adapter: 'vendor.search')`. Keep credential values in the shop’s secret manager. [Custom CLI example](https://github.com/magelift/magelift/tree/main/examples/custom-cli) · [Service bindings](service-bindings.md)

Pin the public SDK as `github.com/magelift/magelift/sdk vX.Y.Z`. Its Go module tag is `sdk/vX.Y.Z`. First-party providers use `providers/<name>/vX.Y.Z` tags. The development workspace resolves local modules. The release check resolves their published versions. Run `make extension-test` to build the public contract example against the SDK. [Public SDK example](https://github.com/magelift/magelift/tree/main/examples/custom-extension-contract)
