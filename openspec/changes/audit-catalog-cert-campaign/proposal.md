## Why

Persona and architecture audits showed MageLift still over-narrows Magento HTTP (nginx-only in schema) while under-documenting the catalog, and they showed a certification campaign that can prove more AWS cells now that ~$8 of ~$160 credits are spent. Adobe's on-premises table lists nginx only for 2.4.8-p3+ and 2.4.9. FrankenPHP and Apache have no Adobe row on that train. The Go architecture review showed a core that is not yet a lean plugin host: static certified tiers, fail-open GCP deploy locks, discarded unlock errors, Cobra-built provider clients, and a provider stub that cannot run. MageLift should stay honest about Adobe, keep the CLI a generic plugin host, and pack a thorough AWS Magento matrix on remaining credits.

## What Changes

- Treat HTTP frontends, clouds, edge, mail, and telemetry as **plugins** behind `sdk/v1` ports. The core CLI stays YAML, Cobra, admission, and the registry. Unknown IDs fail closed. Plugins version independently. No `plugin.Open`. No `if provider ==` in shared graphs.
- Restore **FrankenPHP classic** and add **Apache httpd + PHP-FPM** as first-party `WebRuntime` plugins. On 2.4.8-p3+ / 2.4.9 both MUST use `compatibility.allowUnsupported` (Adobe hatch), documented as having **no Adobe support**. Worker mode stays unregistered. Neither is certified from this campaign.
- Default binary MAY compile first-party plugins for RC1 DX; they still register on the same host as community go-plugin subprocesses.
- Publish an **exhaustive support vs certify** catalog. Certified only with `docs/capability-matrix.md` plus `docs/evidence/` for the **exact tuple** (runtime, compute mode, Magento release, catalog). Product gaps stay named.
- Change unset AWS standard/HA **queue default** from Amazon MQ to `ecs-rabbitmq` (**BREAKING**). Campaign still runs explicit `amazon-mq`.
- Run a **packed campaign**: unlock AWS Bedrock/Lambda ~$40 first (not Magento evidence); AWS KEEP Magento matrix (Fargate, Managed Instances, EKS, Aurora, Amazon MQ, ECS/EKS RabbitMQ, OpenSearch, HA, CloudWatch, X-Ray plugin); GCP Autopilot KEEP; OVH one MKS; Scaleway/vendors from **$50 own-money**. Worktrees; serial Go in a tree.
- Fix **release blockers** from the architecture review: fail-closed locks, generation-safe unlock, `errors.Join`, evidence-dimensioned tiers, caller context on Plan, Cobra ports, lint, runnable provider artifacts.
- Magento overlays on import; `magelift audit` is not a customer SOC 2 / ISO 27001 / GDPR certificate.
- Docs through humanizer then remove-ai-marks.

## Capabilities

### New Capabilities

- `web-runtime-plugins`: Independently versioned Magento HTTP frontends with Adobe vs MageLift vs provider honesty.
- `campaign-inventory`: Budgets, packed AWS matrix, addon honesty, SOC 2 wording, product gaps, KEEP/destroy, architecture-review blockers.
- `plugin-first-core`: Lean CLI host, generic `sdk/v1` ports, independent plugin cadence.

### Modified Capabilities

- `php-runtime-customization`: nginx-fpm default; FrankenPHP/Apache are hatch plugins, not a closed nginx-only enum.
- `product-scope`: Community web servers in scope as plugins; Adobe row remains Magento-support authority.
- `compatibility-catalog`: Adobe hatch for FrankenPHP and Apache; catalog honesty.
- `certification-matrix`: Evidence tuples; no static module certified; packed AWS cells.
- `certification-evidence`: GCP KEEP; AWS KEEP packed matrix; OVH/Scaleway thin; worktrees.
- `efficient-cloud-certification`: Packed warm/cold fingerprints; remaining AWS credits; $50 vendor cap.
- `warm-certification-sessions`: GCP KEEP and AWS KEEP for this campaign; destroy on close.
- `multi-cloud-runtime-architectures`: Named attach/cache/CDN/queue gaps.
- `local-runtime-and-preflight`: Compose follows the selected web-runtime plugin.
- `provider-extension-loading`: Same loader for web runtimes; no `CoreOutputKeys` on HTTP plugins.
- `magento-runtime-config`: Importer Magento overlays.
- `engineering-standards`: Docs lockstep; lint as release gate.
- `external-service-certification`: No vendor Magento cert from DNS/NerdGraph-only cells.
- `native-and-fastly-edge`: Claims match evidence; Armor withheld.
- `fastly-edge`: Docs-host origin is not Magento edge.
- `provider-observability-and-new-relic`: CloudWatch vs X-Ray plugin vs New Relic vs GCP ops.
- `transactional-email`: SES/SendGrid config vs certified delivery.
- `compliance-enabling-controls`: MageLift is not the customer's SOC 2 issuer.
- `provider-native-lifecycle-adapters`: Fail-closed, generation-safe deploy locks.
- `cross-cutting-invariants`: Never drop lock-release errors.
- `cli-contract`: Cobra stays provider-generic.
- `installation-and-distribution`: Releasable signed provider artifacts before claiming Dial.

## Impact

Schema `application.webRuntime` becomes an extension ID (default `nginx-fpm`). Images under `images/frankenphp-classic` and an Apache PHP-FPM runtime become plugin artifacts. `internal/config` admission, local Compose, ECS/EKS/GKE graphs, `magelift extensions`, capability matrix, ADR 0002, examples, lock/orchestrator, certification tier, and the cert harness (`MAGELIFT_*_ACCEPTANCE_KEEP`, worktrees) change. First-party plugins stay in this monorepo with their own version metadata.
