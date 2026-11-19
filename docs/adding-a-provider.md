# Adding a provider

A provider owns its network, database, compute, service integrations and Pulumi program. The core selects an adapter, validates project settings and coordinates deployments. Share the Magento contracts in `sdk`, `internal/platform`, `internal/deploy` and `internal/config`. GCP and Azure each build a separate plugin from `providers/<name>/`. Use that layout for a new cloud. [Provider contract](adr/0013-provider-plugin-contract.md) · [Ports and adapters](adr/0004-ports-and-adapters.md)

## Provider roots

| Root | Holds | Examples |
| --- | --- | --- |
| `internal/cloud/<provider>/` | IaaS topology only: one cloud's network, database, cache, search, queue, runtime, native edge/observability, and stack. Each cloud owns its Pulumi graph; no `if provider ==` switches. | `internal/cloud/aws`, `internal/cloud/ovh`, `internal/cloud/scaleway` (GCP and Azure use `providers/gcp` and `providers/azure`) |
| `internal/external/<vendor>/` | SaaS edge/observability adapters behind typed SDK intents. | `internal/external/fastly`, `internal/external/newrelic`, `internal/external/edge` (composition), `internal/external/observability` (composition) |
| `internal/shared/<port>/` | Provider-neutral durable engines and ports. Stdlib plus `sdk` plus `internal/provider` only; no cloud SDK, no Pulumi. | `internal/shared/recovery`, `internal/shared/resilience`, `internal/shared/statearchive` |
| `internal/edge/waf/` | Provider-neutral Magento-safe WAF contract every edge adapter translates. | `internal/edge/waf` (`waf/magento-safe`) |
| Adapter-less (not a provider root) | Config strings or shell helpers with no Go adapter package. | `email.mode: ses` in `internal/config` (SMTP settings plus secret references; validation only, delivery uncertified); Cloudflare DNS shell helpers (`scripts/acceptance/lib-cloudflare-dns.sh`, `scripts/cutover-dns-cloudflare.sh`, `tests/acceptance/cloudflare_dns_helper_test.sh`; DNS cutover only, no CDN/WAF claim) |

Single exception: `internal/cloud/kube` stays where it is (see ADR 0003 carve-out). Nothing else shared lives under `internal/cloud/`.

## Registration (read first)

| Seam | Role |
| --- | --- |
| `platform.ModuleRegistry` | What the CLI uses. `preview` / `deploy` / `destroy` / `outputs` pick a `StackModule` by `target.provider` + `target.runtime`. |
| `internal/infra.Registry` | Target / Capability / Transform / Hook index for tests and SDK discovery. Registering here alone does not wire the CLI. |

A PR that only calls `infra.RegisterTarget` will not appear in `magelift deploy`.

## Checklist (copy the GCP plugin)

New providers follow [ADR 0013](adr/0013-provider-plugin-contract.md): a nested
`providers/<name>/` module built outside the root module against the public
SDK, speaking versioned typed operations mirroring the module interfaces
with explicit negotiation and fail-closed compatibility (`providers/gcp`
and `providers/azure` show the interface in use).
The in-process checklist below describes the remaining in-process tree
(AWS, OVH, Scaleway); do not start new in-process providers from it.

1. `internal/cloud/<p>/target/`: IDs, `Validate`, optional `Register(*infra.Registry)` for tests.
2. `internal/cloud/<p>/stack/`: `Spec`, `PlanFromConfig*`, Pulumi `Program` (`ctx.Export` must match `Component.Outputs()`), `component`, and a `StackModule`. If Magento Ops would cycle imports with other packages, put `HasOps` on a thin wrapper (AWS: `internal/cloud/aws/ops`).
3. Capability packages (`network`, `database`, …) as needed; keep them typed and small.
4. Optional day-2 ports on the module (`HasOps`, `HasBootstrap`, `HasState`,
   `HasSecrets`, `HasRuntimeObserve`). Return `ErrNotSupported` until ready.
   See [ADR 0004](adr/0004-ports-and-adapters.md). The GCP plugin
   implements these in `providers/gcp/ops`, reached through the plugin protocol.
5. Optional `sdk.ResilienceAdapter` for backup, restore, integrity, fencing,
   failover, and cleanup. Its descriptor MUST declare each data-class status,
   supported destination, polling requirement, and protection requirement.
   Call `sdk.CompileResiliencePlan` from the adapter's side-effect-free plan
   method instead of rebuilding the recovery graph. Registration validates the
   descriptor and rejects a provider mismatch before the module can plan a
   stack. Use the opaque `ServiceBoundaryIntent.ResourceReference` only for a
   provider-owned resource identity; never add provider SDK fields to the
   public contract. A module without this port cannot claim automated backup,
   restore, or disaster-recovery certification.
6. Optional `sdk.EdgeAdapter` and `sdk.ObservabilityAdapter` ports expose
   provider-owned plan/apply-or-verify/destroy operations behind the same
   versioned SDK boundary. Provider-specific adapters must return explicit
   unavailable signal/capability results, use ownership-scoped resources, and
   prove health, delivery, retention, redaction, and cleanup through injected
   probes. The certification scheduler executes the provider callback; it does
   not import a provider SDK or infer success from a plan.
   First-party native resource graphs follow the same intent: AWS
   `internal/cloud/aws/observability` owns CloudWatch resources, GCP
   `providers/gcp/observability` owns GKE collection plus Cloud Monitoring
   dashboards and log-based policies, Scaleway
   `internal/cloud/scaleway/observability` owns Cockpit data sources, and OVH
   `internal/cloud/ovh/observability` owns only the documented Kubernetes audit
   subscription path. The last path requires the opaque
   `observability.nativeReference` stream identity; it does not pretend to
   provision generic workload logs or traces. These Pulumi graphs are offline
   resource evidence, not live delivery or cleanup certification.
   For a planned target-owned lifecycle, implement the matching optional
   `sdk.ResilienceAdapterFactory`, `sdk.EdgeAdapterFactory`, or
   `sdk.ObservabilityAdapterFactory`. The factory receives the opaque public
   `sdk.ModulePlan` after planning and must only construct an adapter; native
   SDK clients and credentials remain in the provider package. First-party
   modules use the equivalent injected `platform.LifecycleFactories` seam.
   Fastly and New Relic are external adapters: their descriptor identity is
   independent from the origin target and they run through the typed edge or
   observability plan request rather than being mislabeled as native target
   adapters.
   SaaS homes: Fastly lives in `internal/external/fastly`, New Relic in
   `internal/external/newrelic`, shared edge and observability composition in
   `internal/external/edge` and `internal/external/observability`, and every
   edge adapter translates the Magento-safe WAF contract in `internal/edge/waf`.
   SES is adapter-less by decision: `email.mode: ses` in `internal/config`
   carries SMTP settings plus secret references with validation only, delivery
   uncertified, and no Go adapter package. Cloudflare is adapter-less by
   decision: DNS cutover runs through shell helpers
   (`scripts/acceptance/lib-cloudflare-dns.sh`,
   `scripts/cutover-dns-cloudflare.sh`,
   `tests/acceptance/cloudflare_dns_helper_test.sh`) with no CDN/WAF claim and
   no Go adapter package.
7. `internal/config`: provider block, enums, validation, then `make generate` for schema.
8. For a first-party provider, register the `StackModule` in
   `internal/registry` and construct provider hooks in
   `registry.RegisterHooks()`. For a community provider, implement `sdk.Module`
   and call `cli.NewWithExtensions(...)` from a custom binary. Keep Pulumi SDKs
   out of `internal/cli` so `gendocs` and CLI unit tests stay linkable on small
   CI runners; non-test code under `cli/` and `internal/cli/` imports zero
   `internal/cloud/*`.
9. Mock Pulumi graph tests; docs for experimental vs acceptance.
10. Record the operation, provider and test type when you add a day-2 adapter.
    Keep historical runs attributable to their original environment and date.

## Required stack outputs

`OutputKeys()` must include every key from `platform.RequiredOutputKeys()`. Map
native IDs into those names (e.g. GCP network → `networkVpcId`). Extra keys are
fine.

## Service connection adapters

A community binary can connect a new service vendor to an existing cloud runtime.
Implement `sdk.ServiceAdapter` and pass it to `cli.NewWithServiceAdapters`. Its
descriptor identifies the source, version, exact targets and service roles.
Run `magelift extensions list` to see which connection adapters the binary contains.
The project's PHP explicitly selects the adapter, for example
`new Service(family: 'hosted-search', version: '3.1', adapter: 'vendor.search')`.
Settings in `extensions['vendor.search']` are available to that resolver.

The resolver runs during planning and returns an existing or external
`sdk.ServiceBinding` using a protocol the target understands. Preserve the
selected version, store credentials as provider secret references, and keep
resolution deterministic and independent of cloud access. For ownership of
new Pulumi resources and their lifecycle, implement a full `sdk.Module`.

## Community binary

The CLI verifies the signed provider binary against `magelift.providers.lock` and starts it through HashiCorp go-plugin using net/rpc. `Describe` reports the protocol version and operations. The host checks that the plugin supports the requested operation before using it. The plugin runs with the operator’s cloud privileges.

GCP and Azure use this interface for planning, lifecycle and day-to-day operations. User-facing `outputs` uses the redacted operation. Internal consumers can request decrypted outputs when a supported operation needs them. `magelift extensions list` shows installed plugin versions and digests. AWS, OVH and Scaleway still run inside the CLI. A custom binary can register a community provider with `cli.NewWithExtensions`. [Provider load paths](adr/0008-provider-load-path.md)

The public extension boundary avoids `internal/` imports. An extension implements
the versioned provider operations, returns provider-neutral plan data, and
executes its own Pulumi stacks inside the provider process; only typed results
cross to the core. The opaque `Program`-returns-`any` shape is retired: stack
work runs provider-side and the core never sees a Pulumi program.
The plan request includes typed `sdk.EdgeIntent` and `sdk.ObservabilityIntent`
values. This lets an extension implement Fastly, a telemetry vendor, or a cloud
native destination without making the core configuration depend on that vendor.
When projection recovery is selected, the request also carries the typed
`sdk.ResilienceIntent.Projection` target; the extension translates its ECS or
Kubernetes identity through an injected provider SDK client instead of parsing
the resolved configuration map. The map remains available for provider-owned
semantic extension namespaces, never for raw SDK argument bags.
An extension may additionally implement `sdk.ResilienceAdapter`; the core owns
the portable recovery graph, ownership, idempotency, scheduling, evidence, and
cleanup gates while the extension owns native backup, restore, fencing, and
failover calls. Credentials and vendor-specific options stay under the
extension's namespaced configuration. Extensions are linked at build time and
are never loaded from an arbitrary path.

The first-release configuration has no legacy singular `edge.provider` or
`observability.provider` field. Edge and observability are independent
composable boundaries: use `nativeProvider` and/or `externalProvider`. The
optional `observability.nativeReference` is an opaque identity for an existing
provider-owned destination; it is not a provider selector, SDK object, or
credential field. The
`provider` fields on `target` and existing-resource identity remain identity
data and are not part of this cleanup.

### SDK pinning

Extensions pin the SDK module, not the monolith:

```text
require github.com/magelift/magelift/sdk vX.Y.Z
```

SDK tags look like `sdk/vX.Y.Z` (nested module at `sdk/`, lockstep with the
CLI version for v1). Provider modules tag as `providers/<name>/vX.Y.Z`,
lockstep with core and SDK for alpha; independent cadence rules are specified
in [ADR 0013](adr/0013-provider-plugin-contract.md) and activate post-alpha.
Do not add a `replace` directive: local development
resolves `./sdk` through the committed `go.work`, and registry builds resolve
the tag.

### Extension build verification (RELEASE-03)

Use only this document and `examples/custom-extension-contract`. Do not browse
`internal/cloud/**` to learn the public contract. From the repo root, run the
SDK contract proof as the only Go command in progress:

```sh
export GOMODCACHE="$(mktemp -d /tmp/magelift-modcache.XXXXXX)"
export GOCACHE="$(mktemp -d /tmp/magelift-gocache.XXXXXX)"
go build -o /tmp/magelift-extension-contract ./examples/custom-extension-contract
/tmp/magelift-extension-contract
```

The full `examples/custom-cli` binary remains the integration example, but it
statically links every first-party Pulumi adapter and can use several
gigabytes during linking. Build that binary on a worker with enough memory;
the routine repository test deliberately checks the public SDK contract in a
small isolated binary and covers the internal adapter through package tests.

A green `version` line proves the custom binary builds from an empty module and
build cache without consulting core source beyond the example tree's imports.

## Non-goals

- Shared Pulumi components that switch on provider
- A single list of settings for every cloud product
- Paid multi-cloud CI matrices by default
