# Local development vs cloud

`magelift local` generates Docker Compose for an account-free Magento loop
([ADR 0006](adr/0006-generated-local-compose.md)). The goal is the same image
digests and service major versions as the certified cloud matrix where that is
practical, not a bit-for-bit copy of AWS.

## What matches

| Piece | Local | Cloud (certified AWS ECS) |
| --- | --- | --- |
| App image contract | PHP branch selected by the local compatibility row | signed immutable image digest on promote |
| OpenSearch / Valkey / RabbitMQ / Artemis majors | digest-pinned images from the local row | managed or ECS broker versions from the provider catalog |
| Magento CLI verbs | `local exec`, cache/reindex helpers | `exec`, `cache-flush`, `reindex` |
| HTTP entry | selected web-runtime plugin, with nginx + PHP-FPM by default and optional Varnish | CloudFront → ALB → Varnish/nginx on the evidenced AWS tuple; `frankenphp-classic` and `php-apache` require the Adobe hatch and are not certified |

## Honest deltas

| Concern | Local | Cloud |
| --- | --- | --- |
| Database | MySQL or MariaDB container from the verified row | Certified: RDS MySQL. Aurora is experimental-warn |
| Queue | RabbitMQ or Artemis container (when enabled) | Certified: `db`, `ecs-rabbitmq` (unset standard/HA default). Explicit `amazon-mq` is experimental-warn. `ecs-artemis` is experimental |
| Email | Magento SMTP config for SMTP, SendGrid, or SES; credentials stay in `.magelift/local.env` | provider or extension-owned integration; credentials stay outside YAML |
| PHP settings and extensions | `.magelift/local.php.ini` plus the verified runtime extension baseline | builder/runtime contract and provider deployment settings |
| Edge / WAF / DNS | none | CloudFront, WAF, Route 53, ACM. Local does not emulate them |
| Secrets | `.magelift/local.env` (mode 0600) | Secrets Manager / SSM / GCP Secret Manager references |
| Multi-AZ / HA | single Compose project | preset-driven topology |
| Deploy migrations | local Magento commands | candidate ECS task + lock |

The local catalog is narrower than the cloud catalog. Redis and
provider-managed email transports stay explicit gaps until their image,
health, Magento connection, and cleanup contracts are verified. Local Mailpit
is a verified SMTP sink, not cloud delivery. If something works only on
Compose, treat it as a bug in the local contract or an undocumented delta.
Local planning cross-checks its pinned PHP, Composer, and service choices
against the shared source-dated compatibility catalog before it writes
Compose. File a genuine mismatch rather than papering over it with
"works on my machine."

## Closer iso (later)

A cheap cloud `preview` stage that reuses the same `magelift.yaml` is the path
to true environment iso. That is a later track, not a substitute for Compose
today.
