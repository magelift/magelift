## Context

See `proposal.md`. `application.webRuntime` is a closed enum (`nginx-fpm`). Cloud providers already use `sdk/v1.Module` plus optional `EdgeAdapterFactory` / `ObservabilityAdapterFactory` / `PlanAdmissionFactory` ports and HashiCorp go-plugin. Magento HTTP must be a sibling `WebRuntime` port (no `CoreOutputKeys`). Adobe's current on-premises table lists nginx only for 2.4.8-p3+ and 2.4.9; FrankenPHP and Apache have no Adobe row on that train. AWS still has most of the ~$160 credits (~$8 spent) plus a ~$40 promo quest. A Go architecture review found fail-open GCP locks, static certification tiers, discarded unlock errors, Cobra provider leaks, and a non-runnable provider stub.

## Goals / Non-Goals

**Goals:**

- Lean core: Cobra + YAML + registry + plugin host. Generic `sdk/v1` ports. No `if provider ==` in shared graphs. Plugins version on their own cadence.
- `WebRuntime` port + first-party `nginx-fpm`, `frankenphp-classic`, `php-apache`.
- Adobe hatch (`compatibility.allowUnsupported`) for both FrankenPHP classic and Apache on the current Magento line.
- Packed AWS KEEP matrix on remaining credits; GCP Autopilot KEEP; thin OVH/Scaleway; $50 vendor cap.
- Evidence-dimensioned `CertificationTier`. Fail-closed deploy locks. `errors.Join` on unlock. X-Ray as an observability plugin before any X-Ray cell.

**Non-Goals:**

- FrankenPHP worker Magento storefront.
- Certifying FrankenPHP or Apache (hatch ≠ certified).
- Magento `websites[]` YAML.
- Cloud SQL / OVH / Scaleway brownfield DB attach.
- Unsigned `plugin.Open`.
- Homebrew/Scoop as a Magento cell (after first public core tag).
- Claiming SOC 2 / ISO 27001 / GDPR for the customer.
- Cartesian Magento shops per SKU.

## Decisions

1. **Ports, not strings.** Stack `Module` owns cloud graphs and `CoreOutputKeys`. `WebRuntime` owns Magento HTTP. Edge, observability, collectors, plan admission, and resilience stay optional factories on `sdk/v1`. Cobra never constructs cloud clients. Alternative: one mega-`Module` for Apache. Rejected: fake VPC outputs.

2. **Lean host, independently versioned plugins.** First-party adapters MAY compile into the default binary for RC1 DX but MUST register on the same registry, with their own `version`, as community go-plugin subprocesses. Extracting SDKs from the default binary is allowed without a core tag when `ExtensionAPIVersion` matches. GoReleaser MUST publish a provider artifact before claiming Dial. Alternative: nginx-only in core forever. Rejected: community pace.

3. **Adobe hatch for every non-nginx HTTP plugin.** `nginx-fpm` is Adobe-supported on 2.4.8-p5 / 2.4.9. `frankenphp-classic` and `php-apache` are Adobe-unsupported on 2.4.8-p3+ / 2.4.9: fail closed without `compatibility.allowUnsupported`, hatch-warn when set, never called Adobe-supported or MageLift-certified from this campaign. Worker stays unregistered. Alternative: FrankenPHP warn-and-proceed. Rejected: Adobe publishes no FrankenPHP row.

4. **Product queue default `ecs-rabbitmq`; campaign still runs Amazon MQ.** Unset AWS standard/HA `queueMode` becomes `ecs-rabbitmq` (**BREAKING**). Explicit `amazon-mq` remains experimental-warn and is an AWS KEEP warm cell.

5. **AWS KEEP packed matrix; GCP KEEP; no KEEP on OVH/Scaleway.** Unlock Bedrock/Lambda credits first. Then `MAGELIFT_AWS_ACCEPTANCE_KEEP` with cold compute families (Fargate, Managed Instances, EKS) and warm catalog transitions (queues, search, Aurora, HA, CloudWatch, X-Ray plugin). One artifact digest. Destroy at close. `GOMAXPROCS=1 GOFLAGS=-p=1` per worktree.

6. **CertificationTier is a tuple.** `internal/cloud/aws/stack.Module.CertificationTier` MUST NOT return certified for every ECS shape. Autopilot MUST NOT certify HA or other Magento releases from preview evidence.

7. **Locks fail closed.** GCP `NewGCS` errors MUST NOT become a no-op lock. `Release` MUST use caller context, owner, and generation. Orchestrator MUST `errors.Join` unlock failures.

8. **Importer writes Magento overlays only.** `frontName`, cookies, CORS, consumers. No fake website graph.

## Risks / Trade-offs

- [Operators think FrankenPHP is Adobe-certified] → Hatch + matrix wording; no certified badge.
- [AWS KEEP spend] → Packed warm cells, spend pause when the envelope is gone, destroy on close.
- [Aurora/OpenSearch/HA cost] → One transition per family, not three Magento shops.
- [X-Ray absent today] → Plugin first, then one Magento-origin cell; typed unavailable until then.
- [First-party still linked in the default binary] → Same ports now; subprocess extract is a version bump of the plugin, not a new Cobra.

## Migration Plan

1. Land `WebRuntime` + nginx-fpm behind the registry (no YAML change for defaults).
2. FrankenPHP classic + Apache plugins with Adobe hatch; worker unregistered.
3. Fail-closed GCP/AWS lock + `errors.Join`; evidence-dimensioned tiers; Cobra ports.
4. Flip unset queue default; X-Ray observability plugin (typed unavailable until registered).
5. Docs (humanizer, remove-ai-marks). Unlock AWS promo credits. Packed cert: GCP KEEP, AWS KEEP matrix, OVH/Scaleway thin, vendors on GCP origin.
6. No enum rollback: removing a first-party plugin is a new change.

## Do not forget (release path)

### Plugin-first core

- Host: `cmd/magelift`, `internal/cli`, config, generate, doctor.
- Ports already in `sdk/v1`: `Module`, `EdgeAdapterFactory`, `ObservabilityAdapterFactory`, `CollectorDeploymentAdapterFactory`, `PlanAdmissionFactory`, `ResilienceAdapterFactory`. Add `WebRuntime`.
- Adapters: `internal/cloud/<provider>/`, `internal/webruntime/<id>/`, edge/mail/APM plugins.
- No Pulumi in Cobra. No `plugin.Open`. No unsigned working-directory exec.
- `make lint` green (gofmt `internal/cloud/gcp/target/target.go`, `internal/cloud/kube/observe_test.go`; staticcheck S1011 `internal/cloud/aws/stack/component.go`).

### Money and AWS matrix (remaining credits)

1. Complete Bedrock playground + Lambda web-app promo (~$40), no Magento evidence.
2. AWS KEEP: Fargate Magento origin → warm `ecs-rabbitmq`, `amazon-mq`, OpenSearch, `aurora-mysql`, HA, CloudWatch, X-Ray plugin, Magento-origin CloudFront.
3. Cold: Managed Instances Magento; EKS Magento + EKS RabbitMQ.
4. GCP KEEP Autopilot Magento + vendor attach if $50 cap remains.
5. OVH one MKS; Scaleway one Kapsule from the cap.
6. Destroy KEEP stacks at close.

### HTTP

- Default `nginx-fpm`.
- `frankenphp-classic` and `php-apache`: Adobe hatch, documented no Adobe support, never certified here.
- `frankenphp-worker`: unregistered.

### Architecture-review blockers (must fix before first Magento release)

1. Evidence-dimensioned certification (not static module certified).
2. GCP lock fail-closed; generation-safe release with caller context.
3. Always join lock-release errors.
4. Plan() caller context; immutable plan fields; runnable provider artifacts.
5. Cobra behind registries.
6. Lint green.
7. X-Ray plugin before X-Ray claims.

### Addon honesty

| Area | Code today | Campaign may prove | Must not claim |
| --- | --- | --- | --- |
| CloudWatch | AWS graph | Magento-origin on Fargate KEEP | Three-signal cert without evidence |
| X-Ray | No adapter (IAM snippet on EKS only) | After observability plugin, Magento-origin traces | Traces from IAM policy alone |
| GCP ops | Logs/metrics | Remaining Autopilot Magento signals | Open traces as certified |
| New Relic | GKE + bounded NerdGraph | Magento three-signal on GCP if cap allows | Ops cell as full APM |
| CloudFront+WAF | Fargate path | Magento-origin only | Docs-host alias HTTPS as Magento edge |
| Cloud Armor | Withheld | Do not retry Magento body exclusions | Magento-safe Armor |
| Fastly | Experimental | Magento origin if cap allows | Docs origin as Magento edge |
| Cloudflare | DNS cutover | DNS on GCP origin if cap | CDN/WAF product |
| SES/SendGrid | SMTP config | Delivery or typed unsupported | Config-only certified mail |

### Product gaps named until closed

No `websites[]`; importer overlays; no Cloud SQL attach; SQS/Pub/Sub are Magento modules; OVH CDN fail-closed; Scaleway Redis; split Valkey AWS-only; RabbitMQ 1↔quorum refuse; Homebrew/Scoop after first tag.

## Open Questions

- Magento Commerce live cells stay out unless a licensed artifact is supplied later.
- First-party adapters stay compile-in for RC1 DX; subprocess extract of AWS/GCP SDKs from the default binary MAY follow in the same change if Dial is proven, and MUST NOT block Magento HTTP plugins.
