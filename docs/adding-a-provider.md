# Adding a provider

New clouds are adapters. Do not share Pulumi `Network`/`Database` components with
a provider switch ([ADR 0003](adr/0003-portable-contracts-vs-topology.md), [ADR 0004](adr/0004-ports-and-adapters.md)). The core owns configuration envelopes,
command UX, plugin discovery, trust, locking, and orchestration policy; the
provider owns its cloud implementation, stack execution, and operating
capabilities ([ADR 0013](adr/0013-provider-plugin-contract.md)). Magento-facing
code stays in `sdk`, `internal/platform`, `internal/deploy`, and
`internal/config`. VPC, DB, and runtime stay under `internal/cloud/<provider>/`
until Order 5 moves providers to nested `providers/<name>/` modules; new
providers are specified as out-of-module plugins, not in-process copies.
SaaS edge and observability
adapters live under `internal/external/`, provider-neutral ports under
`internal/shared/`, and the Magento-safe WAF contract under `internal/edge/waf`.
SES (`email.mode`) and Cloudflare DNS are adapter-less by decision: config
strings and shell helpers with no Go adapter package. The full rule is
§ Provider roots below.

## Provider roots

| Root | Holds | Examples |
| --- | --- | --- |
| `internal/cloud/<provider>/` | IaaS topology only: one cloud's network, database, cache, search, queue, runtime, native edge/observability, and stack. Each cloud owns its Pulumi graph; no `if provider ==` switches. | `internal/cloud/aws`, `internal/cloud/ovh`, `internal/cloud/scaleway` (GCP lives in `providers/gcp` as the reference autonomous plugin) |
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
is the reference: 30 operations over go-plugin net/rpc).
The in-process checklist below describes the remaining in-process tree
(AWS, OVH, Scaleway); do not start new in-process providers from it.

1. `internal/cloud/<p>/target/`: IDs, `Validate`, optional `Register(*infra.Registry)` for tests.
2. `internal/cloud/<p>/stack/`: `Spec`, `PlanFromConfig*`, Pulumi `Program` (`ctx.Export` must match `Component.Outputs()`), `component`, and a `StackModule`. If Magento Ops would cycle imports with other packages, put `HasOps` on a thin wrapper (AWS: `internal/cloud/aws/ops`).
3. Capability packages (`network`, `database`, …) as needed; keep them typed and small.
4. Optional day-2 ports on the module (`HasOps`, `HasBootstrap`, `HasState`,
   `HasSecrets`, `HasRuntimeObserve`). Return `ErrNotSupported` until ready.
   See [ADR 0004](adr/0004-ports-and-adapters.md). GCP's certified module
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
10. Later for certification: implement the day-2 ports end-to-end ([ADR 0010](adr/0010-live-certification.md)
    packed sessions; [ADR 0003](adr/0003-portable-contracts-vs-topology.md) layout).

## Required stack outputs

`OutputKeys()` must include every key from `platform.RequiredOutputKeys()`. Map
native IDs into those names (e.g. GCP network → `networkVpcId`). Extra keys are
fine.

## Tiers

| Tier | Meaning |
| --- | --- |
| Certified | Shared Magento acceptance on a real account; full ops |
| Experimental | In-tree; may be infra-only; labeled in docs/CLI |
| Community | Out-of-tree `sdk.Module`; custom binary calls `cli.NewWithExtensions` |

Two certified first-party Magento origins (GCP Autopilot + AWS ECS Fargate)
satisfy the multi-cloud claim gate
([ADR 0002](adr/0002-certified-vs-experimental.md);
[ADR 0008](adr/0008-provider-load-path.md)).

For AWS product choices (runtime, natMode, databaseEngine, searchMode), see the
matrix in [architecture.md](architecture.md#aws-magento-product-matrix). Full Magento
on AWS is not every SKU.

## Community binary

The released `magelift` binary loads providers as signed subprocess
artifacts (`magelift.providers.lock`, Cosign, HashiCorp go-plugin).
`internal/providerhost` refuses unsigned or digest-mismatched lock entries,
then `Dial` starts a gRPC subprocess that serves the versioned typed
operations: Describe (negotiation), ValidateConfig, stack lifecycle (plan,
preview, apply, destroy, refresh, outputs with a redacted variant), and the
seven day-2 operations ([ADR 0013](adr/0013-provider-plugin-contract.md)).
`Dial` negotiates protocol versions and fails closed on incompatibility or
integrity failure; there is no silent fallback to embedded execution (an
explicit `--migration-mode` flag is the only sanctioned fallback).
`extensions list` reports the installed provider version, digest, and mode.
Day-2 operations run inside the provider; the `outputs` op returns secrets
decrypted for day-2 consumers while `redacted-outputs` redacts for display.
The provider subprocess runs with the user's cloud privileges: the process
boundary is a deployment and compatibility boundary, not a security sandbox.
Tests and Floci suites always load in-process. External providers
may still ship as a **compile-time custom binary** that calls
`cli.NewWithExtensions` (`examples/custom-cli`). There is no Go `plugin.Open`
ABI and no unsigned remote loader ([ADR 0008](adr/0008-provider-load-path.md)).
Current code still speaks the single-version JSON proof with exact-version
equality and in-process day-2 ports ([ADR 0011](adr/0011-subprocess-dial-proof.md));
Order 5 implements the contract above. Post-alpha extraction order: AWS ECS
Fargate next, then the experimental providers, then community plugin onboarding.

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
GOMAXPROCS=1 GOFLAGS=-p=1 go build -o /tmp/magelift-extension-contract ./examples/custom-extension-contract
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
- One YAML catalog for every cloud product
- Paid multi-cloud CI matrices by default
