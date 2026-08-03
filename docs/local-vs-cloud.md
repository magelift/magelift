# Local development vs cloud

`magelift dev` generates Docker Compose for an account-free Magento loop
([ADR 0005](adr/0005-local-development-compose.md)). The goal is the same image
digests and service major versions as the certified cloud matrix where that is
practical, not a bit-for-bit copy of AWS.

## What matches

| Piece | Local | Cloud (certified AWS ECS) |
| --- | --- | --- |
| App image contract | digest-pinned PHP runtime / FrankenPHP classic | same GHCR digests on promote |
| OpenSearch / Valkey / RabbitMQ majors | pinned Compose images | managed or ECS broker versions from catalog |
| Magento CLI verbs | `dev exec`, cache/reindex helpers | `exec`, `cache-flush`, `reindex` |
| HTTP entry | localhost Caddy | CloudFront → ALB → Varnish/nginx |

## Honest deltas

| Concern | Local | Cloud |
| --- | --- | --- |
| Database | MySQL container | Aurora Serverless v2 or RDS MySQL |
| Queue | RabbitMQ container (when enabled) | `db`, `amazon-mq`, `ecs-rabbitmq`, or `ecs-artemis` |
| Edge / WAF / DNS | none | CloudFront, WAF, Route 53, ACM |
| Secrets | `.magelift/local.env` (mode 0600) | Secrets Manager / SSM references |
| Multi-AZ / HA | single Compose project | preset-driven topology |
| Deploy migrations | local Magento commands | candidate ECS task + lock |

If something works only on Compose, treat it as a bug in the local contract or an
undocumented delta. File it rather than papering over with "works on my machine."

## Closer iso (later)

A cheap cloud `preview` stage that reuses the same `magelift.yaml` is the path to
true environment iso. That is a v1.1 track, not a substitute for Compose today.
