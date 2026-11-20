# Add a provider to MageLift

Use a provider module when you need to run the Magento application on a new
cloud or runtime. Use a service adapter when the application can stay on its
current runtime and needs to connect to a service from another vendor.

| What you need | Where to start |
| --- | --- |
| Use a database, cache, session store, search service or broker the runtime already understands | Configure an existing service binding. [Service choices](service-bindings.md#connect-to-existing-services) |
| Connect an externally managed service with its own connection settings | Write a public SDK service adapter and link it into a custom CLI. |
| Develop a cloud integration outside the MageLift repository | Start with an SDK module for the plan and Pulumi program. Use a fork to wire the new target and full deployment lifecycle into the CLI. |
| Ship a new cloud or runtime through MageLift's releases | Propose the target in an issue or contribute the provider and host integration in a PR. |

The stock CLI registers AWS, Google Cloud and Azure. It downloads verified
executables for those provider IDs. A project lock pins an executable for a
registered provider. Adding an entry to the lock by itself adds no runtime
to the CLI registry. [Provider support](capability-matrix.md) · [Provider protocol](adr/0013-provider-plugin-contract.md)

## Develop an external extension

### Try the custom CLI

From a MageLift source checkout, build the example and inspect the registry.

```sh
go build -o /tmp/magelift-ext ./examples/custom-cli
/tmp/magelift-ext --output json extensions list
```

Look for `example.community-stub` in `modules`. The example registers one
provider and runtime and reports its metadata alongside the built-in targets.
Its planner deliberately returns an error, so use this check to verify
registration. [Custom CLI example](https://github.com/magelift/magelift/tree/main/examples/custom-cli)

### Implement the provider contract

Keep your provider code and cloud SDK dependencies in your own Go module.
Implement `sdk.Module` and pass it to `cli.NewWithExtensions` in the CLI entry
point. Follow the entry point in the custom CLI example.

For local development, set `MAGELIFT_SOURCE` to a MageLift checkout. Run these
commands from your extension's Go module in a new workspace.

```sh
go work init . "$MAGELIFT_SOURCE" "$MAGELIFT_SOURCE/sdk"
go build -o magelift-ext .
./magelift-ext --output json extensions list
```

For a published version, pin
`github.com/magelift/magelift` and `github.com/magelift/magelift/sdk` to matching
versions. The SDK tag uses the `sdk/vX.Y.Z` prefix.

| Method | What to implement |
| --- | --- |
| `Descriptor` | Return the SDK API version, extension ID, version, source, tier, one provider and runtime pair, and the output keys from `sdk.CoreOutputKeys()`. Declare capabilities you implement and add any extra outputs your code needs. |
| `Plan` | Build an `sdk.ModulePlan` from `sdk.ModulePlanRequest` without contacting the cloud. Keep the requested environment, region, protection and image digest. Use `Opaque` for provider-specific plan data. |
| `Program` | Return a Pulumi `pulumi.RunFunc` that consumes the plan and defines the provider's resources. Export the declared outputs and mark sensitive values as secrets. |

Use Pulumi mocks to exercise the resource graph and its exported values.
The public SDK also has optional interfaces for account admission, edge,
recovery, telemetry and collector deployment. Declare and test each operation
separately. Run `make extension-test` in MageLift to check its SDK consumer
example. [Public SDK source](https://github.com/magelift/magelift/tree/main/sdk) · [SDK contract example](https://github.com/magelift/magelift/tree/main/examples/custom-extension-contract)

### Wire a new cloud into a private build

The custom CLI can register extension metadata today. Project validation
still accepts the built-in provider and runtime pairs, and the normal Magento
rollout needs provider operations beyond `sdk.Module`. For a new cloud to
deploy a shop, wire the target into a MageLift fork using the integration
steps below. The provider implementation can stay in a separate repository.

For an out-of-process provider, build the runtime's executable against the
matching SDK protocol. Register the runtime in your fork, and distribute the
CLI and the signed executable with a matching lock and Cosign bundle. The
explicit lock records the executable's digest and trusted signer. Verify the
plan and a deployment through the custom CLI before distributing it.
[Provider installation and locks](install.md#providers) · [Distribution trust](adr/0014-distribution-trust.md)

### Connect a service to an existing runtime

Implement `sdk.ServiceAdapter` for a service connection. Its descriptor
lists the adapter ID, version, target pairs and roles such as `search`. Its
resolver returns an attached service binding that the chosen runtime can
use. For example, it can translate your vendor's search settings into an
OpenSearch endpoint and a secret reference. Keep resolution deterministic
and account-free so it works during validation, preview and teardown.

Register it with `cli.NewWithServiceAdapters`. Select its ID on the shop's
service binding. The resolver receives the settings under
`extensions.<adapter-id>` as options. Keep credential values in the shop's
secret store. The runtime uses the connection while the service owner
continues to manage the external resource. [Service bindings](service-bindings.md)

## Propose a target in an issue

Open the feature request form. Give it a title such as "Add Example Cloud
Kubernetes as a Magento target" and choose "New cloud provider or runtime"
in the affected-area field. [Open a feature request](https://github.com/magelift/magelift/issues/new?template=feature.yml)

Include enough detail for someone to plan a first deployment.

- The Magento shop you want to deploy and why that runtime is a fit.
- The first configuration worth testing. Name the region, database, cache,
  search, queue, media and registry choices. Say which are managed,
  self-hosted or already owned by the shop.
- The workflows you need. A staging rollout with logs and teardown is a
  useful starting scope. Describe previews, backup, restore or failover if
  they are part of the request.
- A synthetic project or a link to public provider docs. Include any checks
  you can reproduce and what help you can offer with implementation or testing.

Use placeholders for account IDs, endpoints and secret references. For a large
provider change, use the issue to agree on the first scope and test method
before implementing it.

## Contribute a first-party provider

Start a branch in your fork. Use the AWS module for a provider with two
runtimes, Google Cloud for Kubernetes and
managed services, or Azure for a smaller standalone plugin. Keep the
provider's resource types and vendor clients in its module.
[Provider modules](https://github.com/magelift/magelift/tree/main/providers) · [Contributor guide](https://github.com/magelift/magelift/blob/main/CONTRIBUTING.md)

1. Create `providers/<name>/go.mod`, add the module to `go.work` and put the
   executable at `providers/<name>/cmd/magelift-provider-<name>/`. Define the
   provider and runtime IDs and add their typed configuration and defaults.
   Keep the shop-facing settings in PHP. Generate the schema and reference
   when public fields or flags change.
2. Implement the SDK handshake and `Describe`. Advertise the runtime, public
   output keys and versioned operations. The host requires `describe`,
   `validate-config`, `plan`, `preview`, `apply`, `outputs`, `destroy`,
   `state-lock`, `state-unlock` and `deploy-app-phase` for a first-party
   deployment. Keep provider API types inside the executable.
3. Validate and plan before mutation. Include project and environment identity,
   account, region, runtime, image digest, service bindings and protection in
   the plan. Use read-only account admission to check identity and regional
   availability before changing resources. The provider owns its Pulumi
   program and keeps its private plan data across calls.
4. Implement the Magento rollout and teardown. Wire the candidate image,
   migration, web update, checks and cleanup. Preserve secret output markings.
   Destroy must select the existing stack and respect protection and
   ownership. Add bootstrap, secrets, logs, media transfer, edge, backup or
   cost for the operations you can support.
5. Register the runtime and output keys in `internal/providerhost/standalone.go`.
   Wire starter defaults and configuration validation, and update
   `internal/infra` capability discovery where applicable. Add a GitHub
   Actions renderer when the target supports generated shop workflows.
   Exercise a normal CLI command to check the host selects the plugin.
6. Add the module to local and CI checks and the signed release build. Give
   the module its own test command and dependency checks. If Floci supports
   the new provider API, add a module-local emulator runner. Keep the provider
   artifact and lock in the release workflow's verification.
   [Release procedure](publishing.md)

Kubernetes targets can reuse Magento workloads in
`internal/cloud/kube`. S3-compatible state uses `internal/s3state` where
applicable. For CDN purge, declare the required public outputs in
`EdgeAdapterDescriptor.PurgeOutputKeys` and verify that the target belongs
to the selected shop and environment before cache invalidation.
[Architecture](architecture.md) · [Protocol decision](adr/0013-provider-plugin-contract.md)

### Verify and open the PR

Use a synthetic Magento project and verify a plan through the executable.
Check that the runtime, image digest, service bindings and environment
reach the plugin. With Pulumi mocks, inspect resource inputs, dependencies
and secret outputs. Test a failed rollout and cleanup. A restarted plugin
should be able to release its own lock with the original receipt, and a stale
receipt must leave a replacement lock in place.

Exercise the cloud APIs the emulator implements. Use SDK fakes for cases
such as retries, partial failure and ownership mismatches where you need
more control. Report the exact method, configuration and outcome. If the
provider needs a live check, use an authorized dedicated account with a
cleanup owner and teardown on exit.

Run `make provider-tidy-check`, `make core-leanness` and `make local-gates`
after wiring the provider into those checks. Run `make ci-act-project` if the
change affects generated shop workflows, and `make generate` after schema
or CLI changes. Review the published dependency setup outside the Go workspace
before a release.

Open the PR against `main`. Link the issue if there is one, list which
operations work, include the checks you ran and describe what still needs
verification. Update the capability guide, setup instructions and examples.
[Open a pull request](https://github.com/magelift/magelift/compare) · [Provider evidence](capability-matrix.md)
