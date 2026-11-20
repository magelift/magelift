# ADR 0003. Share Magento contracts across providers

Accepted on 2026-08-22.

Magento has the same build and rollout phases across clouds. Each provider
has different resources and failure modes, so the portable settings describe
what the shop needs. The provider interprets those settings and owns its
resource graph.

Shared contracts describe the application, services, artifact, release identity
and deployment phases. Put options for a specific cloud under
`target.<provider>`. The PHP build package in `build/` handles Composer,
patches and Magento build commands.

| Package | Responsibility |
| --- | --- |
| `providers/<name>/` | One cloud's network, database, services, workloads and operations |
| `internal/cloud/kube/` | Magento workload wiring and Kubernetes services reused by providers |
| `internal/external/` | Fastly, New Relic and other external adapters |
| `internal/shared/` | Recovery, resilience and state archive logic shared across clouds |
| `internal/edge/waf/` | WAF rules for Magento that each edge adapter translates |

Cache and sessions have separate settings. Choose each service's version,
ownership and capacity independently of web replica count. A highly available
web tier can still depend on a single-replica service, which the config check
reports. [Ports and adapters](0004-ports-and-adapters.md) · [Service choices](../service-bindings.md)
