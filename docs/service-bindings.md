# Choose services for a shop

Declare common service choices under `services` in `magelift.php`. Each environment can override a role. Versions come from the shop or its PHP project. Adobe's requirements guide the choice, and each cloud controls which managed versions it offers in a region. A container tag such as MySQL 8.4.3 can remain exact in Compose or in a self-hosted image. Cloud SQL and Azure Flexible Server use the 8.4 service branch and select the patch. RDS accepts an offered full patch or a family such as MariaDB 11.4. [Adobe system requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements)

| Role | AWS ECS Fargate | GCP GKE | Azure AKS |
| --- | --- | --- | --- |
| Database | RDS MySQL or MariaDB, Aurora through AWS settings, or an existing database | Cloud SQL for MySQL, self-hosted MariaDB, or an existing MySQL-compatible database | MySQL Flexible Server, self-hosted MariaDB, or an existing MySQL-compatible database |
| Cache and optional separate sessions | ElastiCache Valkey or existing Redis-compatible endpoints | Memorystore Valkey, self-hosted stores, or existing Redis-compatible endpoints | Self-hosted Valkey or Redis, or an existing unauthenticated endpoint |
| Search | Amazon OpenSearch Service or an existing endpoint | Self-hosted OpenSearch or an existing endpoint | Self-hosted OpenSearch |
| Queue | Database, RabbitMQ on ECS, Amazon MQ, or an existing RabbitMQ broker | Database, RabbitMQ on GKE, or an existing RabbitMQ broker | Database, RabbitMQ on AKS, or an existing AMQPS broker |

## Keep sessions when cache entries expire

The following service settings run two stores on Kubernetes. They also give local Compose distinct cache and session containers. With a managed service, choose its instance size and check its supported engine and eviction settings. For example, the GCP starter selects Memorystore 8.0 when an Adobe recommendation names Valkey 8.1. [Memorystore versions](https://docs.cloud.google.com/memorystore/docs/valkey/supported-versions)

```php
'services' => [
    'cache' => new \MageLift\Service(
        family: 'valkey', version: '8.1', ownership: 'self-hosted',
        evictionPolicy: 'allkeys-lru', memoryMiB: 512,
    ),
    'session' => new \MageLift\Service(
        family: 'valkey', version: '8.1', ownership: 'self-hosted',
        evictionPolicy: 'noeviction', memoryMiB: 512, storageGiB: 4,
    ),
],
```

Kubernetes self-hosted MariaDB, cache and session stores have one replica each. A persistent volume keeps data across pod restarts. Each provisioned Kubernetes store has its own password, and dedicated session stores use an append-only log. Rehearse recovery from a lost volume and set memory limits that cover the session workload. AKS RabbitMQ and the ECS RabbitMQ task also have one writer. The ECS broker uses EFS. A high-availability web tier can use these services, and `magelift config validate` names the single-replica dependencies. Choose a redundant database or broker where the shop requires failover.

For a provisioned RDS database or self-hosted MariaDB, `services.database.parameters` accepts `character_set_server`, `collation_server` and `max_allowed_packet` in bytes. Self-hosted database, search, broker and store bindings accept `storageGiB` where the provider provisions their volumes. GCP and AKS can keep MariaDB in the cluster when an existing dump depends on MariaDB behavior. Moving the dump to a MySQL service needs a separate rehearsal.

## Connect to existing services

Set `ownership` to `existing`, supply an endpoint reachable from the workload and grant the destination access to the referenced secret. AWS uses Secrets Manager for credentials. GCP uses Secret Manager version references. On AKS, an attached database uses port 3306 and a trusted TLS certificate. An attached RabbitMQ broker uses AMQPS. Both read the password from the `password` entry of a Secret in the Magento namespace. Give previews their own endpoints and credentials when they use attachments.

```php
'queue' => new \MageLift\Service(
    family: 'rabbitmq', ownership: 'existing',
    endpoint: 'amqps://broker.staging.example.com:5671',
    username: 'magento', secret: 'kubernetes://broker-login',
),
```

An AKS deployment exposes a load balancer address by default. Set up ingress, DNS and TLS for the shop hostname. Custom binaries can register an adapter for another service or a provider module that manages its infrastructure. [Azure setup](bootstrap.md#azure-aks) · [Extending MageLift](adding-a-provider.md)

## Run ElasticSuite on OpenSearch

Smile ElasticSuite is a Magento extension for search and merchandising. Use the `opensearch` service family. MageLift supplies the endpoint to Magento and ElasticSuite and preserves the shop's selected search adapter. Check it with `bin/magento config:show catalog/search/engine`. For a new local installation, `magelift dev --search-engine elasticsuite` selects ElasticSuite at install time. [Smile compatibility table](https://github.com/Smile-SA/elasticsuite#which-version-should-i-use-) · [Installation guide](https://github.com/Smile-SA/elasticsuite/wiki/ModuleInstall)

A locked ElasticSuite package adds `analysis-icu` and `analysis-phonetic` to `services.search.plugins`. Amazon's provisioned OpenSearch domains include these analyzers. Self-hosted OpenSearch on GKE, AKS and local Compose checks the installed plugins at startup. It downloads missing plugins, so allow outbound access or provide an image with the matching versions already installed. When moving a custom search image, check `GET /_cat/plugins?v` and reindex a representative catalog. [OpenSearch plugins](https://docs.opensearch.org/latest/install-and-configure/additional-plugins/index/) · [Amazon OpenSearch plugins](https://docs.aws.amazon.com/opensearch-service/latest/developerguide/supported-plugins.html)
