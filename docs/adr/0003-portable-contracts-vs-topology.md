# ADR 0003: Portable Magento contracts vs per-cloud topology

- Status: Accepted
- Date: 2026-08-22

## Context

Magento build and deploy phases are mostly portable. An Aurora cluster, an ECS service, and a GKE workload are not. A shared YAML schema of "the cloud" would hide failure modes and leak lowest-common-denominator fields into `magelift.yaml`.

## Decision

Portable contracts describe what Magento needs: application model, artifact manifest, build lifecycle, release identity, capability requirements. They do not name the cloud resources that satisfy them.

Provider topology lives in `internal/cloud/<provider>/` (network, database, cache, search, queue, runtime, edge, observability, stack). `internal/platform` and `sdk` stay provider-neutral. The PHP Composer package under `build/` is not the Go tree.

Shared Pulumi components that switch on `if provider ==` are forbidden. Each cloud owns its graph (ADR 0004).

Typed provider options may appear under `target.<provider>`. Raw provider schemas do not enter portable YAML.

Provider roots:

| Root | Holds | Examples |
| --- | --- | --- |
| `internal/cloud/<provider>/` | IaaS topology only: one cloud's network, database, cache, search, queue, runtime, native edge/observability, and stack. Each cloud owns its Pulumi graph; no `if provider ==` switches. | `internal/cloud/aws`, `internal/cloud/gcp`, `internal/cloud/ovh`, `internal/cloud/scaleway` |
| `internal/external/<vendor>/` | SaaS edge/observability adapters behind typed SDK intents. | `internal/external/fastly`, `internal/external/newrelic`, `internal/external/edge` (composition), `internal/external/observability` (composition) |
| `internal/shared/<port>/` | Provider-neutral durable engines and ports. Stdlib plus `sdk` plus `internal/provider` only; no cloud SDK, no Pulumi. | `internal/shared/recovery`, `internal/shared/resilience`, `internal/shared/statearchive` |
| `internal/edge/waf/` | Provider-neutral Magento-safe WAF contract every edge adapter translates. | `internal/edge/waf` (`waf/magento-safe`) |
| Adapter-less (not a provider root) | Config strings or shell helpers with no Go adapter package. | `email.mode: ses` in `internal/config` (SMTP settings plus secret references; validation only, delivery uncertified); Cloudflare DNS shell helpers (`scripts/acceptance/lib-cloudflare-dns.sh`, `scripts/cutover-dns-cloudflare.sh`, `tests/acceptance/cloudflare_dns_helper_test.sh`; DNS cutover only, no CDN/WAF claim) |

Single exception: `internal/cloud/kube` stays where it is (see carve-out below). Nothing else shared lives under `internal/cloud/`.

Carve-out: `internal/cloud/kube` is the single shared Pulumi helper. It holds Magento-shaped Kubernetes wiring (`SkipAwaitAnnotations`, `ToStringArray`, `EnvVars`, `BuildStaticTokenKubeconfig`, shared Observe/Steps/projection) consumed by the GKE, MKS, and Kapsule runtime adapters, pinned to one `pulumi-kubernetes` SDK version. It owns no network, database, cache, search, or queue topology and contains no `if provider ==` switch. Scope is frozen: no new shared Pulumi helpers without an ADR amending this section.

## Consequences

Moving a shop from Fargate to Autopilot keeps Magento contracts and replaces topology. Cost, recovery, and IAM stay provider-specific. Adding a cloud is a new adapter package, not a third copy of Magento orchestration. Moving `recovery`, `resilience`, and `statearchive` out of `internal/cloud/` into `internal/shared/` recertifies nothing: folder moves change no certified cell.

## Alternatives considered

- Universal infrastructure schema: rejected (weakest-shared-feature YAML).
- Raw Pulumi in project YAML: rejected (unstable public contract).

## Provenance

Original project decision. Public Pulumi and Magento docs only.
