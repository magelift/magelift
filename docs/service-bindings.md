# Choose services for a shop

Each role under `services` selects a family, version and optional ownership.
Declare the common choices at the root of `magelift.php`. An environment can
override a single role. Magento compatibility advice uses a dated Adobe
snapshot. The shop owns the version decision, and the cloud API determines
availability in the selected account and region.

When a Magento release recommends Valkey 8.1, the GCP starter selects Memorystore 8.0. You can set an explicit version in the PHP project. Check availability before deploying. [Memorystore versions](https://docs.cloud.google.com/memorystore/docs/valkey/supported-versions)

Inside a PHP project definition, a staging shop can reuse an existing private
OpenSearch endpoint while the other stages keep their own search resources.

```php
'services' => [
    'database' => new \MageLift\Service(family: 'mysql', version: '8.4'),
    'queue' => new \MageLift\Service(family: 'database'),
],
'environments' => [
    'staging' => new \MageLift\Environment([
        'services' => [
            'search' => new \MageLift\Service(
                family: 'opensearch',
                ownership: 'existing',
                endpoint: 'https://search.staging.example.com',
            ),
        ],
    ], class: 'staging'),
],
```

| Role | AWS ECS Fargate | GCP GKE | Azure AKS |
| --- | --- | --- | --- |
| Database | RDS MySQL or MariaDB, Aurora through AWS settings, or an attached database | Cloud SQL for MySQL or an attached MySQL-compatible database | MySQL Flexible Server |
| Cache | ElastiCache Valkey or an attached Redis-compatible cache | Memorystore Valkey or an attached Redis-compatible cache | Self-hosted Valkey or Redis, or an existing unauthenticated endpoint |
| Search | Managed OpenSearch, with additional modes in AWS settings, or an attached endpoint | Self-hosted OpenSearch or an attached endpoint | Self-hosted OpenSearch |
| Queue | Database, RabbitMQ on ECS, Amazon MQ or an attached RabbitMQ broker | Database, RabbitMQ on GKE or an attached RabbitMQ broker | Database |

For Azure production, supply an existing highly available cache that AKS can reach. The bundled self-hosted cache runs a single replica. The AKS starter exposes a load balancer address. A custom hostname needs an ingress controller, DNS and TLS configured for that host. [Azure setup](bootstrap.md#azure-aks)

The external-search example works with AWS ECS and GKE for reachable,
unauthenticated endpoints. Keep attached services in their owning stack and
configure connectivity from Magento. Refer to AWS Secrets Manager or a
version in Google Secret Manager when an attachment needs credentials. The
built-in database and cache connections on these targets use ports 3306 and
6379. The planner reports unsupported authentication, connection or provisioning
modes before creating replacement resources. Local Compose provisions
development containers from the same family, version or explicit local image.

The built-in Magento runtime sets `catalog/search/engine` to `opensearch` when it wires a search endpoint. It supplies connection settings for ElasticSuite as well. If the shop selects `elasticsuite` or another Magento search engine, preserving that selection requires a runtime change. Check the effective engine and reindex a representative catalog before moving traffic.

Custom binaries can register an adapter to resolve a service vendor to a
supported connection. A full provider module can own new infrastructure
and define its lifecycle. [Extending MageLift](adding-a-provider.md)
