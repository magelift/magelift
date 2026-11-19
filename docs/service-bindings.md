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

Smile ElasticSuite is a Magento search and merchandising extension that can use the same OpenSearch service as Magento’s native search. Set the service family to `opensearch` in either case. MageLift passes the connection details to both clients and preserves the adapter selected in Magento. For an existing shop, check `bin/magento config:show catalog/search/engine` and reindex a representative catalog before cutover. [Smile’s compatibility table](https://github.com/Smile-SA/elasticsuite#which-version-should-i-use-)

For a fresh local installation with the extension installed, run `magelift dev --search-engine elasticsuite` or use the same flag with `magelift local seed`. You can also select the adapter for local and cloud environments by setting `CONFIG__DEFAULT__CATALOG__SEARCH__ENGINE` under `application.magento.variables`. A fresh local installation defaults to `opensearch` when you leave the adapter unspecified. [Smile’s installation guide](https://github.com/Smile-SA/elasticsuite/wiki/ModuleInstall)

ElasticSuite’s default analyzers need the OpenSearch plugins `analysis-icu` and `analysis-phonetic`. Check the nodes with `GET /_cat/plugins?v`. For self-hosted OpenSearch, build the plugins into the shop’s search image and set `local.search.image` for local Compose. For example, this Dockerfile adds both plugins to OpenSearch 3.1.0. [OpenSearch plugin management](https://docs.opensearch.org/latest/install-and-configure/plugins/) · [Amazon OpenSearch plugins](https://docs.aws.amazon.com/opensearch-service/latest/developerguide/supported-plugins.html)

```dockerfile
FROM opensearchproject/opensearch:3.1.0
RUN bin/opensearch-plugin install --batch analysis-icu analysis-phonetic
```

Custom binaries can register an adapter to resolve a service vendor to a
supported connection. A full provider module can own new infrastructure
and define its lifecycle. [Extending MageLift](adding-a-provider.md)
