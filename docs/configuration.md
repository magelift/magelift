# Configuration reference

This page is generated from the MageLift configuration schema. Do not edit it by hand.

`magelift.yaml` uses schema version 1. Unknown fields are rejected.

When a provider target block is present, the resolver applies the selected preset's
safe provider shape defaults and the service versions from the Magento compatibility catalog.
Project and environment values override them. The defaults do not create secrets,
certificates, or existing-resource references; provider and preset catalogs do
materialize named capacity shapes so advanced YAML can override them deliberately
instead of inheriting an undocumented SDK default.
Use `config effective` to inspect the values and their provenance.

See the [advanced AWS example](https://github.com/magelift/magelift/blob/main/examples/advanced-aws-fck-nat.yaml) for a complete
typed configuration that composes multi-AZ fck-nat, native and external edge,
native and external observability, recovery policy, and a community extension.

Configuration has four intentional levels: the project/defaults/preset layer is the
easy path for most users; environment overlays specialize it without duplicating the
whole file; provider target fields select supported semantic infrastructure choices;
and namespaced `extensions.<vendor>` fields hold provider-owned integration options.
Provider target fields are validated semantic escape hatches, not an unversioned dump
of raw provider SDK arguments. Omitted values resolve to named defaults and appear in
the effective configuration, provenance, plan, and architecture fingerprint.

Cloud transactional email is `email` on the project or an environment overlay.
`ses` requires a provider-matching secret reference in `email.credential`;
plaintext is rejected before Magento SMTP is written. Validation success is not certified delivery.
Modes `tem` and `ovh`, and `ses` with a `managed` block, provision the sender identity, credentials, and DNS through the provider adapter; explicit host, port, username, and credential values are rejected alongside `managed`.
`local.email` remains the workstation overlay and uses `credentialEnv`. A preview environment that
omits `email` resolves to `disabled` and does not inherit production SES credentials.

Resilience projection targets are optional advanced settings: use `resilience.projection.runtime: ecs` with an ECS `cluster`,
`service` (preferred) or pinned `task`, and `container`; use `runtime: kubernetes` with `namespace`, `workload`, and an optional
`container`. These are workload identities only; credentials and application verifiers stay provider-owned.

`target.provider: gcp` with `runtime: gke-autopilot` is the certified first-party Magento path for evidenced cells. `runtime: gke-standard` is experimental and remains the node-capable path for workloads such as multi-node OpenSearch.
Historical 2.4.6-p15 and 2.4.9 topology/runtime passes remain scoped evidence. The complete Adobe cache intersection and the rest of the release matrix stay open.
Both require `target.gcp.project` and keep GCP topology under
`target.gcp` only. Do not reuse `target.aws` fields for GCP.

GCP Memorystore capacity and failure-domain choices are explicit advanced fields: `memorystoreShardCount`,
`memorystoreReplicas`, `memorystoreMode`, and `memorystoreZoneDistributionMode`/`memorystoreZone`.
Cluster Mode Disabled accepts one shard; Cluster Mode Enabled is the path for multiple shards, and
the selected mode is translated to the current Memorystore API rather than left to an SDK default.

`target.provider: ovh` / `runtime: mks` and `target.provider: scaleway` / `runtime: kapsule`
are also experimental. See [ovh-experimental.md](ovh-experimental.md) and
[scaleway-experimental.md](scaleway-experimental.md). Scaleway requires
`cacheMode: redis` (no managed Valkey yet); `databaseHighAvailability` and `redisClusterSize`
select its supported RDB/Redis failure-domain shapes. Redis cluster mode (3-6 nodes) remains
blocked until the Magento connector carries cluster discovery endpoints. OVH exposes
`mksPlan`, `zones`, `nodeCount`, `databaseNodeCount`, and `valkeyNodeCount`; `zones` are MKS
availability zones inside the selected OVH region, not database regions. Free MKS is single-zone;
Standard multi-zone MKS creates one worker pool per listed zone and requires at least one worker per zone.
The preview preset selects the current low-cost discovery one-node shape; standard and high-availability select the current production two-node shape. Advanced YAML can choose discovery/essential, business/production, or enterprise/advanced plans plus explicit flavors and node counts, and provider admission checks the selected region before mutation.
On Scaleway, `zone` selects the managed-cache zone and `zones`
selects the Kapsule worker zones; multi-zone plans create one worker pool per listed zone
and require at least one configured node per zone. The preview preset may use one zone;
standard requires two and high-availability requires three, so an underprovisioned list
is rejected before Pulumi mutation.

Managed-service durability is customizable at the provider boundary while keeping the core contract portable.
AWS exposes RDS/Aurora backup and maintenance windows, database deletion-protection and automated-backup deletion policy, and ElastiCache Valkey snapshot retention/window choices. Preview materializes zero Valkey snapshot-retention days; standard and high-availability materialize seven days. Database deletion protection and automated-backup cleanup are materialized from the environment class, with production retaining protection and backups by default. Explicit `false` and `0` values are preserved as intentional advanced choices.
The AWS ECS presets materialize task CPU/memory, capacity mode, database/cache/search capacity, and queue shape;
the EKS presets materialize compute mode, Kubernetes version, requests, workload replicas, and the shared
managed database/cache durability shape.
GCP exposes Cloud SQL automated-backup, binary-log, retained-backup-count, transaction-log, schedule,
location, and deletion-protection choices plus Memorystore deletion protection and the PSC connection limit.
The standard and high-availability GCP presets name eight retained automated backups and seven days of
transaction-log retention; the preview preset disables Cloud SQL automated backups and binary logging.
Cloud Armor is materialized for durable presets and for production environments, including production
environments that deliberately select preview capacity; an explicit `enableCloudArmor` value overrides it.
Scaleway exposes RDB backup enablement, frequency, retention, same-region placement, and encryption at rest;
standard and high-availability presets name daily backups (24 hours), seven-day retention, same-region
placement, and encryption at rest; preview explicitly disables automated backups to avoid orphaned
backup cost during disposable environments.
OVH exposes managed MySQL/Valkey backup time, destination regions, and deletion protection. Retention is
provider-native: GCP's retained-backup count, Scaleway's retention days, and OVH's plan-controlled policy
are not silently converted into the generic recovery policy. Cache authentication or transport-encryption
modes are only exposed once the Magento connection, secret, certificate, health, and recovery contracts
can honor them end to end; otherwise they fail as typed unsupported choices before mutation.

Deployments require `target.aws.encryptionKeySecretArn` to reference a stable
Secrets Manager value. MageLift injects it at task start as the Magento encryption
key. The key must remain stable for the lifetime of encrypted Magento data.

`target.aws.existing.network` enables an existing VPC. When it is set, provide one
public, private, and data subnet ID for every configured availability zone. MageLift
does not create NAT gateways, route tables, or VPC endpoints in this mode, so the
imported network must already provide the required egress and private AWS service
access. The VPC CIDR is still required for security-group rules.

For an owned AWS network, omitting `target.aws.vpcCidr` and
`target.aws.availabilityZones` selects the named preview/standard/high-availability
network defaults (`10.42.0.0/16` and two or three zones derived from the configured
region). The effective configuration shows these values and their provenance;
override them when the account's available zones or address plan differs.

`target.aws.existing.database` adopts an existing AWS RDS MySQL instance. When it is
set, provide `provider: aws`, `kind: database`, `externalId` (instance ID or ARN),
`secretArn` (Secrets Manager master-user secret ARN; never an inline password),
and `endpoint` (writer hostname). MageLift does not create RDS when this block is
set. GCP managed Cloud SQL settings, when used, belong under `target.gcp`; they are
not overloaded into the AWS existing-resource shape.

For AWS fck-nat, `natMode`, `natTopology`, and `natReplacementMode` form one
explicit network boundary. Selecting fck-nat materializes the preview or durable-preset
topology, replacement policy, and one cost-optimized ARM64 NAT instance in effective YAML;
multi-AZ fck-nat uses one failure-domain target per selected zone and automatic
replacement through an Auto Scaling group unless `none` is explicitly selected.
`target.aws.natInstanceType` overrides the ARM64 Graviton size; it defaults to the
cost-optimized `t4g.nano`, and x86 instance types are rejected for the first-party
ARM64 fck-nat AMI.
A high-availability fck-nat plan requires multi-AZ plus automatic replacement; a
single instance is never labeled highly available.

`build.staticContent.strategy` and `build.staticContent.threads` map PaaS
`SCD_STRATEGY` / `SCD_THREADS` on import and become Magento SCD `-s` / `-j`.
See [ece-tools parity](ece-parity.md) for the closed vs intentional-gap matrix.

`build.extensions` declares the PHP extensions that the selected builder must load.
`build.composer.version` accepts a Composer 2 major.minor or patch version; append `+` for a minimum. Both
requirements are checked inside the isolated builder before dependency installation; an
unsupported extension or Composer version fails the build instead of being ignored.

`observability.nativeProvider` selects a matching cloud telemetry destination and
`observability.externalProvider` selects an extension-owned adapter without putting vendor credentials
in the portable configuration. `observability.nativeReference` is an opaque identity
owned by that native provider; it is not a credential, selector, or provider SDK
object. For example, OVHcloud uses it to identify an existing Logs Data Platform
stream. `cloudwatch`, `google-cloud-operations`, `scaleway-cockpit`, and
`ovh-logs-data-platform` are first-party native adapters for their matching clouds.
New Relic, Datadog, OTLP, and other external providers are extension-owned. Put
their credentials and provider-specific options under `extensions.<vendor>`.
The core preserves an extension namespace as opaque YAML; its registered adapter
must strict-decode and validate that payload before planning or mutation. A
first-party adapter rejects signals it cannot configure rather than silently
dropping them.

The first-release schema has no singular `edge.provider` or
`observability.provider` field and has no compatibility alias for either one.
Use `nativeProvider` and `externalProvider` to compose destinations;
`provider` on `target` and existing-resource identities remains identity data.

External edge apply requires a real `edge.health` policy. Set
`edge.health.expectedCname` to the provider's DNS target and, for a standalone
`edge apply`, set `edge.health.originUrl` to the origin health endpoint. A normal
deployment may derive the origin URL from the newly provisioned
`applicationURL` output. If the endpoint uses a generated load-balancer hostname with a certificate
for the application domain, set `edge.health.originHost` for TLS SNI and HTTP Host.
When one edge domain is configured, the first-party Fastly adapter uses that
domain as the default origin host; set `originHost` when the certificate differs.
An `originHealthRef` is a reference for evidence and
never substitutes for an HTTP health proof. `tlsMode: external` means that
certificate ownership and renewal stay outside MageLift; the Fastly adapter
maps it to customer-provided TLS and does not create a TLS subscription.

| Field | Type | Required | Accepted values | Description |
| --- | --- | --- | --- | --- |
| `schemaVersion` | integer | yes | 1 | MageLift configuration schema version |
| `project` | object | yes |  | Project identity |
| `project.name` | string | yes |  | Project name |
| `application` | object | yes |  | Magento application settings |
| `application.edition` | string | yes | open-source, commerce | Magento edition |
| `application.version` | string | yes |  | Exact Magento release |
| `application.mode` | string | yes | integrated, headless | Application mode |
| `application.webRuntime` | string | no | nginx-fpm, frankenphp-classic, php-apache | HTTP application runtime |
| `application.magento` | object | no |  | Magento runtime overlays |
| `application.magento.frontName` | string | no |  | Magento admin frontName |
| `application.magento.cookieDomain` | string | no |  | Magento cookie domain |
| `application.magento.unsecureBaseUrl` | string | no |  | Unsecure Magento base URL |
| `application.magento.secureBaseUrl` | string | no |  | Secure Magento base URL |
| `application.magento.corsOrigins` | array | no |  | Allowed CORS origins for the Magento API |
| `application.magento.storefrontOrigin` | string | no |  | Headless storefront origin allowed by CORS |
| `application.magento.consumers` | object | no |  | Magento message consumer runners |
| `application.magento.consumers.mode` | string or null | no | cron, processes, both | Consumer runner |
| `application.magento.consumers.names` | array | no |  | Named Magento consumers |
| `application.magento.queueTransport` | string or null | no | sqs, pubsub | Optional Magento-module queue transport |
| `application.magento.queueModule` | string or null | no |  | Locked Composer Magento package providing SQS or Pub/Sub transport |
| `application.magento.variables` | object | no |  | CONFIG__* and MAGENTO_DC_* overlays |
| `application.cron` | array | no |  | Portable Magento cron schedule entries |
| `build` | object | yes |  | Application build settings |
| `build.php` | string | yes |  | Exact PHP branch or patch version |
| `build.extensions` | array | no |  | Required PHP extensions |
| `build.composer` | object | no |  | Composer settings |
| `build.composer.version` | string or null | no |  | Required Composer 2 major, minor, or patch version |
| `build.composer.credentials` | string | no |  | Composer credentials secret reference |
| `build.staticContent` | object | no |  | Static content deployment settings |
| `build.staticContent.locales` | array | no |  | Locales passed to setup:static-content:deploy --language |
| `build.staticContent.themes` | array | no |  | Themes passed to setup:static-content:deploy --theme |
| `build.staticContent.strategy` | string or null | no | quick, standard, compact | Static content deploy strategy (-s) |
| `build.staticContent.threads` | integer or null | no |  | Static content deploy thread count (-j) |
| `build.qualityPatches` | array | no |  | Quality Patch IDs applied at build |
| `build.hooks` | object | no |  | Build lifecycle hooks |
| `build.hooks.*.phase` | string | yes | validate, build, package | Preparation lifecycle phase |
| `build.hooks.*.relationship` | string | yes | before, after, replace, disable | Relationship to the target step |
| `build.hooks.*.target` | string | yes |  | Stable target step ID |
| `build.hooks.*.command` | object or null | no |  | Validated command vector |
| `build.hooks.*.command.executable` | string | yes | composer, magento | Allowed executable |
| `build.hooks.*.command.arguments` | array | no |  | Argument vector |
| `build.hooks.*.dependencies` | array | no |  | Additional stable step dependencies |
| `build.hooks.*.timeoutSeconds` | integer | no |  | Step timeout in seconds |
| `build.hooks.*.retries` | object | no |  | Retry policy |
| `build.hooks.*.retries.maxAttempts` | integer | no |  | Maximum attempts |
| `build.hooks.*.retries.delaySeconds` | integer | no |  | Retry delay in seconds |
| `build.hooks.*.retries.idempotent` | boolean | no |  | Whether retrying is safe |
| `build.hooks.*.failure` | string | no | abort, continue | Failure action |
| `local` | object | no |  | Local runtime service and email choices |
| `local.database` | object | no |  | Local database family and version |
| `local.database.family` | string or null | no |  | Service family |
| `local.database.version` | string or null | no |  | Service version |
| `local.cache` | object | no |  | Local cache family and version |
| `local.cache.family` | string or null | no |  | Service family |
| `local.cache.version` | string or null | no |  | Service version |
| `local.search` | object | no |  | Local search family and version |
| `local.search.family` | string or null | no |  | Service family |
| `local.search.version` | string or null | no |  | Service version |
| `local.queue` | object | no |  | Local queue family and version |
| `local.queue.family` | string or null | no |  | Service family |
| `local.queue.version` | string or null | no |  | Service version |
| `local.webServer` | object | no |  | Local web server family and version |
| `local.webServer.family` | string or null | no |  | Service family |
| `local.webServer.version` | string or null | no |  | Service version |
| `local.webCache` | object | no |  | Local web cache family and version |
| `local.webCache.family` | string or null | no |  | Service family |
| `local.webCache.version` | string or null | no |  | Service version |
| `local.phpSettings` | object or null | no |  | Local PHP ini settings |
| `local.email` | object | no |  | Local email delivery mode |
| `local.email.mode` | string or null | no | disabled, smtp, ses, mailpit | Email mode |
| `local.email.host` | string or null | no |  | SMTP or local email host |
| `local.email.port` | integer or null | no |  | SMTP or local email port |
| `local.email.username` | string or null | no |  | SMTP username |
| `local.email.from` | string or null | no |  | Default sender address |
| `local.email.credentialEnv` | string or null | no |  | Environment variable containing the email credential |
| `target` | object | yes |  | Deployment target |
| `target.provider` | string | yes | aws, gcp, ovh, scaleway | Infrastructure provider |
| `target.runtime` | string | yes | ecs-fargate, eks, gke-autopilot, gke-standard, mks, kapsule | Application runtime |
| `target.aws` | object or null | no |  | AWS deployment inputs |
| `target.aws.kmsKeyArn` | string or null | no |  | Customer-managed KMS key ARN |
| `target.aws.hostedZoneId` | string or null | no |  | Route 53 hosted zone ID |
| `target.aws.cloudFrontCertificateArn` | string or null | no |  | us-east-1 ACM certificate ARN |
| `target.aws.albCertificateArn` | string or null | no |  | Regional ACM certificate ARN |
| `target.aws.snsTopicArn` | string or null | no |  | SNS notification topic ARN |
| `target.aws.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `target.aws.cacheSecretArn` | string or null | no |  | Valkey auth token secret ARN |
| `target.aws.sessionSecretArn` | string or null | no |  | Session auth token secret ARN |
| `target.aws.queueSecretArn` | string or null | no |  | RabbitMQ password secret ARN |
| `target.aws.encryptionKeySecretArn` | string or null | no |  | Magento encryption key secret ARN |
| `target.aws.databaseName` | string or null | no |  | Magento database name |
| `target.aws.masterUsername` | string or null | no |  | Database master username |
| `target.aws.vpcCidr` | string or null | no |  | Canonical VPC IPv4 CIDR |
| `target.aws.availabilityZones` | array or null | no |  | AWS availability zones |
| `target.aws.mediaDomain` | string or null | no |  | Media delivery domain |
| `target.aws.natMode` | string or null | no | nat-gateway, fck-nat | Private subnet egress mode |
| `target.aws.natTopology` | string or null | no | single-az, multi-az | AWS NAT failure-domain topology; omitted defaults to single-AZ for preview and multi-AZ for standard/high-availability |
| `target.aws.natReplacementMode` | string or null | no | none, auto-scaling | AWS fck-nat replacement mode; omitted defaults to automatic replacement for multi-AZ fck-nat |
| `target.aws.natInstanceType` | string or null | no |  | ARM64 fck-nat instance type; omitted defaults to cost-optimized t4g.nano |
| `target.aws.labels` | object or null | no |  | Resource labels |
| `target.aws.existing` | object or null | no |  | Existing AWS resources |
| `target.aws.existing.network` | object or null | no |  | Existing VPC reference |
| `target.aws.existing.network.provider` | string | yes | aws | Resource provider |
| `target.aws.existing.network.kind` | string | yes |  | Resource kind |
| `target.aws.existing.network.externalId` | string | yes |  | Provider resource identifier |
| `target.aws.existing.publicSubnetIds` | array or null | no |  | Existing public subnet IDs |
| `target.aws.existing.privateSubnetIds` | array or null | no |  | Existing private subnet IDs |
| `target.aws.existing.dataSubnetIds` | array or null | no |  | Existing data subnet IDs |
| `target.aws.existing.database` | object or null | no |  | Existing RDS MySQL reference |
| `target.aws.existing.database.provider` | string | yes | aws | Resource provider |
| `target.aws.existing.database.kind` | string | yes |  | Resource kind |
| `target.aws.existing.database.externalId` | string | yes |  | RDS instance identifier or ARN |
| `target.aws.existing.database.secretArn` | string | yes |  | Secrets Manager master-user secret ARN |
| `target.aws.existing.database.endpoint` | string | yes |  | RDS writer endpoint hostname |
| `target.aws.catalog` | object | no |  | Benchmark-selected service catalog |
| `target.aws.catalog.version` | string or null | no |  | Benchmark catalog version |
| `target.aws.catalog.databaseEngine` | string or null | no | aurora-mysql, rds-mysql, rds-mariadb | Database engine shape |
| `target.aws.catalog.searchMode` | string or null | no | serverless, provisioned, disabled | OpenSearch provisioning mode |
| `target.aws.catalog.queueMode` | string or null | no | db, amazon-mq, ecs-rabbitmq, ecs-artemis | Magento messaging broker mode |
| `target.aws.catalog.databaseBackupWindow` | string or null | no |  | AWS RDS/Aurora automated backup window in UTC |
| `target.aws.catalog.databaseMaintenanceWindow` | string or null | no |  | AWS RDS/Aurora maintenance window in UTC |
| `target.aws.catalog.databaseDeletionProtection` | boolean or null | no |  | Enable AWS RDS/Aurora deletion protection |
| `target.aws.catalog.databaseDeleteAutomatedBackups` | boolean or null | no |  | Delete AWS RDS/Aurora automated backups when the database is destroyed |
| `target.aws.catalog.cacheSnapshotRetentionLimit` | integer or null | no |  | AWS ElastiCache Valkey automatic snapshot retention in days; zero disables snapshots |
| `target.aws.catalog.cacheSnapshotWindow` | string or null | no |  | AWS ElastiCache Valkey daily snapshot window in UTC |
| `target.aws.catalog.aurora` | object | no |  | Aurora or RDS MySQL capacity |
| `target.aws.catalog.aurora.minimumAcu` | number | no |  | Minimum Aurora Serverless v2 capacity |
| `target.aws.catalog.aurora.maximumAcu` | number | no |  | Maximum Aurora Serverless v2 capacity |
| `target.aws.catalog.aurora.autoPauseSeconds` | integer | no |  | Aurora auto-pause duration |
| `target.aws.catalog.aurora.engineSupportsAutoPause` | boolean | no |  | Whether the selected engine supports auto-pause |
| `target.aws.catalog.aurora.instanceClass` | string | no |  | Provisioned Aurora or RDS MySQL instance class |
| `target.aws.catalog.aurora.instanceCount` | integer | no |  | Provisioned Aurora instance count |
| `target.aws.catalog.valkey` | object | no |  | Valkey capacity |
| `target.aws.catalog.valkey.nodeType` | string | no |  | Valkey node type |
| `target.aws.catalog.valkey.replicaCount` | integer | no |  | Valkey replica count |
| `target.aws.catalog.search` | object | no |  | OpenSearch capacity |
| `target.aws.catalog.search.maximumIndexingOcu` | number | no |  | OpenSearch Serverless indexing limit |
| `target.aws.catalog.search.maximumSearchOcu` | number | no |  | OpenSearch Serverless search limit |
| `target.aws.catalog.search.acceptColdStarts` | boolean | no |  | Accept OpenSearch Serverless cold starts |
| `target.aws.catalog.search.instanceType` | string | no |  | Provisioned OpenSearch instance type |
| `target.aws.catalog.search.instanceCount` | integer | no |  | Provisioned OpenSearch data node count |
| `target.aws.catalog.search.dedicatedMasterType` | string | no |  | OpenSearch dedicated master type |
| `target.aws.catalog.search.dedicatedMasterCount` | integer | no |  | OpenSearch dedicated master count |
| `target.aws.catalog.search.ebsVolumeType` | string | no |  | OpenSearch EBS volume type |
| `target.aws.catalog.search.ebsVolumeSizeGiB` | integer | no |  | OpenSearch EBS volume size |
| `target.aws.catalog.rabbitMq` | object | no |  | Amazon MQ RabbitMQ capacity (queueMode amazon-mq) |
| `target.aws.catalog.rabbitMq.instanceType` | string | no |  | RabbitMQ broker instance type |
| `target.aws.catalog.fargate` | object | no |  | Fargate capacity (ecs-fargate) |
| `target.aws.catalog.fargate.computeMode` | string or null | no | fargate, fargate-spot, ec2-asg, managed-instances | ECS capacity mode |
| `target.aws.catalog.fargate.cpu` | integer | no |  | Fargate task CPU |
| `target.aws.catalog.fargate.memoryMiB` | integer | no |  | Fargate task memory |
| `target.aws.catalog.fargate.desiredCount` | integer | no |  | Fargate desired task count |
| `target.aws.catalog.fargate.instanceType` | string or null | no |  | EC2 or managed-instance capacity type |
| `target.aws.catalog.fargate.instanceAmi` | string or null | no |  | Pinned AMI for ECS EC2 capacity |
| `target.aws.catalog.fargate.minCapacity` | integer or null | no |  | Minimum host capacity |
| `target.aws.catalog.fargate.maxCapacity` | integer or null | no |  | Maximum host capacity |
| `target.aws.catalog.eks` | object or null | no |  | EKS Autopilot capacity (eks) |
| `target.aws.catalog.eks.computeMode` | string or null | no | auto-mode, managed-node-groups, self-managed, fargate | EKS compute mode |
| `target.aws.catalog.eks.kubernetesVersion` | string or null | no |  | EKS Kubernetes minor version |
| `target.aws.catalog.eks.cpuRequest` | string or null | no |  | Web/cron CPU request |
| `target.aws.catalog.eks.memoryRequest` | string or null | no |  | Web/cron memory request |
| `target.aws.catalog.eks.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `target.aws.catalog.eks.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `target.aws.catalog.eks.searchMode` | string or null | no | opensearch, disabled | EKS OpenSearch workload mode |
| `target.aws.catalog.eks.searchReplicas` | integer or null | no |  | OpenSearch StatefulSet replicas |
| `target.aws.catalog.eks.queueMode` | string or null | no | database, rabbitmq | EKS queue workload mode |
| `target.aws.catalog.eks.queueReplicas` | integer or null | no |  | RabbitMQ StatefulSet replicas |
| `target.aws.catalog.eks.nodeInstanceType` | string or null | no |  | EKS node instance type |
| `target.aws.catalog.eks.nodeAmi` | string or null | no |  | Pinned AMI for self-managed EKS nodes |
| `target.aws.catalog.eks.nodeMinSize` | integer or null | no |  | Minimum EKS node count |
| `target.aws.catalog.eks.nodeDesiredSize` | integer or null | no |  | Desired EKS node count |
| `target.aws.catalog.eks.nodeMaxSize` | integer or null | no |  | Maximum EKS node count |
| `target.aws.catalog.eks.fargateNamespaces` | array or null | no |  | Namespaces scheduled on EKS Fargate |
| `target.aws.catalog.retention` | object | no |  | Retention policy |
| `target.aws.catalog.retention.logDays` | integer | no |  | CloudWatch log retention days |
| `target.aws.catalog.retention.backupDays` | integer | no |  | Database backup retention days |
| `target.aws.catalog.retention.artifactDays` | integer | no |  | Artifact retention days |
| `target.aws.catalog.versions` | object | no |  | Managed service versions |
| `target.aws.catalog.versions.auroraMysql` | string | no |  | Aurora MySQL engine version |
| `target.aws.catalog.versions.mysql` | string | no |  | RDS MySQL engine version |
| `target.aws.catalog.versions.mariaDb` | string | no |  | RDS MariaDB engine version |
| `target.aws.catalog.versions.valkey` | string | no |  | Valkey engine version |
| `target.aws.catalog.versions.openSearch` | string | no |  | OpenSearch engine version |
| `target.aws.catalog.versions.rabbitMq` | string | no |  | RabbitMQ engine version |
| `target.gcp` | object or null | no |  | GCP deployment inputs |
| `target.gcp.project` | string | yes |  | GCP project ID |
| `target.gcp.region` | string or null | no |  | GCP region |
| `target.gcp.networkCidr` | string or null | no |  | VPC IPv4 CIDR |
| `target.gcp.zones` | array or null | no |  | GCP zones |
| `target.gcp.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `target.gcp.databaseName` | string or null | no |  | Magento database name |
| `target.gcp.masterUsername` | string or null | no |  | Cloud SQL master username |
| `target.gcp.encryptionKeySecret` | string or null | no |  | Secret Manager secret ID for Magento encryption key |
| `target.gcp.cloudSqlTier` | string or null | no |  | Cloud SQL machine tier |
| `target.gcp.cloudSqlAvailability` | string or null | no | ZONAL, REGIONAL | Cloud SQL availability type |
| `target.gcp.cloudSqlBackupEnabled` | boolean or null | no |  | Enable Cloud SQL automated backups |
| `target.gcp.cloudSqlBinaryLogEnabled` | boolean or null | no |  | Enable Cloud SQL MySQL binary logging for point-in-time recovery |
| `target.gcp.cloudSqlBackupRetentionCount` | integer or null | no |  | Cloud SQL retained automated backup count |
| `target.gcp.cloudSqlTransactionLogRetentionDays` | integer or null | no |  | Cloud SQL MySQL transaction log retention in days (1-7) |
| `target.gcp.cloudSqlBackupStartTime` | string or null | no |  | Cloud SQL automated backup start time (HH:MM) |
| `target.gcp.cloudSqlBackupLocation` | string or null | no |  | Cloud SQL automated backup storage location |
| `target.gcp.cloudSqlDeletionProtection` | boolean or null | no |  | Enable Cloud SQL service and IaC deletion protection |
| `target.gcp.memorystoreNodeType` | string or null | no |  | Memorystore for Valkey node type |
| `target.gcp.memorystoreShardCount` | integer or null | no |  | Memorystore for Valkey shard count |
| `target.gcp.memorystoreReplicas` | integer or null | no |  | Memorystore for Valkey replica count per shard (0-5) |
| `target.gcp.memorystoreEngineVersion` | string or null | no | VALKEY_8_0, VALKEY_9_0, VALKEY_9_1 | Memorystore for Valkey engine version (VALKEY_9_0 GA or VALKEY_9_1 Preview) |
| `target.gcp.memorystoreMode` | string or null | no | CLUSTER, CLUSTER_DISABLED | Memorystore for Valkey cluster mode |
| `target.gcp.memorystoreZoneDistributionMode` | string or null | no | MULTI_ZONE, SINGLE_ZONE | Memorystore for Valkey zone distribution mode |
| `target.gcp.memorystoreZone` | string or null | no |  | Memorystore for Valkey single-zone placement |
| `target.gcp.memorystoreDeletionProtection` | boolean or null | no |  | Enable Memorystore for Valkey deletion protection |
| `target.gcp.memorystorePscConnectionLimit` | integer or null | no |  | Maximum automatic Memorystore PSC connections |
| `target.gcp.openSearchMode` | string or null | no | opensearch, disabled | GKE OpenSearch workload mode |
| `target.gcp.openSearchReplicas` | integer or null | no |  | GKE OpenSearch replica count |
| `target.gcp.openSearchImage` | string or null | no |  | GKE OpenSearch image reference |
| `target.gcp.queueMode` | string or null | no | database, rabbitmq | GKE queue workload mode |
| `target.gcp.queueReplicas` | integer or null | no |  | GKE RabbitMQ replica count |
| `target.gcp.rabbitMqImage` | string or null | no |  | GKE RabbitMQ image reference |
| `target.gcp.enableCloudArmor` | boolean or null | no |  | Create a GCP Cloud Armor security policy |
| `target.gcp.kubernetesVersion` | string or null | no |  | GKE Kubernetes version |
| `target.gcp.releaseChannel` | string or null | no | RAPID, REGULAR, STABLE | GKE release channel |
| `target.gcp.clusterIpv4Cidr` | string or null | no |  | GKE cluster pod IPv4 CIDR |
| `target.gcp.servicesIpv4Cidr` | string or null | no |  | GKE services IPv4 CIDR |
| `target.gcp.standardNodeType` | string or null | no |  | GKE Standard node machine type |
| `target.gcp.standardNodeCount` | integer or null | no |  | GKE Standard initial node count per zone |
| `target.gcp.standardNodeMinCount` | integer or null | no |  | GKE Standard minimum nodes per zone |
| `target.gcp.standardNodeMaxCount` | integer or null | no |  | GKE Standard maximum nodes per zone |
| `target.gcp.standardNodeDiskType` | string or null | no |  | GKE Standard node boot disk type |
| `target.gcp.standardNodeDiskSizeGiB` | integer or null | no |  | GKE Standard node boot disk size |
| `target.gcp.standardNodeImageType` | string or null | no |  | GKE Standard node image type |
| `target.gcp.standardNodeSpot` | boolean | no |  | Use Spot VMs for GKE Standard nodes |
| `target.gcp.autopilotCpuRequest` | string or null | no |  | GKE Autopilot CPU request |
| `target.gcp.autopilotMemoryRequest` | string or null | no |  | GKE Autopilot memory request |
| `target.gcp.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `target.gcp.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `target.gcp.labels` | object or null | no |  | Resource labels |
| `target.ovh` | object or null | no |  | OVHcloud deployment inputs (experimental) |
| `target.ovh.serviceName` | string | yes |  | OVH Public Cloud project service name |
| `target.ovh.apiEndpoint` | string or null | no | ovh-eu, ovh-ca, ovh-us | OVHcloud API endpoint alias (ovh-eu, ovh-ca, or ovh-us) |
| `target.ovh.region` | string or null | no |  | OVH Public Cloud region (for example EU-WEST-PAR or GRA11) |
| `target.ovh.networkCidr` | string or null | no |  | Private network IPv4 CIDR |
| `target.ovh.zones` | array or null | no |  | OVH MKS availability zones within the selected region |
| `target.ovh.attachFloatingIps` | boolean | no |  | Attach OVH floating IPs to MKS nodes |
| `target.ovh.privateNetworkRoutingAsDefault` | boolean | no |  | Route MKS egress through OVH's documented private-subnet DHCP gateway; custom gateway routing is not inferred |
| `target.ovh.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `target.ovh.databaseName` | string or null | no |  | Magento database name |
| `target.ovh.masterUsername` | string or null | no |  | Managed MySQL master username |
| `target.ovh.encryptionKeySecret` | string or null | no |  | Secret reference for Magento encryption key |
| `target.ovh.databaseFlavor` | string or null | no |  | OVH managed MySQL flavor |
| `target.ovh.databasePlan` | string or null | no | discovery, essential, business, production, enterprise, advanced | OVH managed MySQL plan (discovery/essential, business/production, or enterprise/advanced) |
| `target.ovh.databaseVersion` | string or null | no | 8.0, 8.4 | OVH managed MySQL engine version (8.0 or 8.4) |
| `target.ovh.databaseNodeCount` | integer or null | no |  | OVH managed MySQL node count |
| `target.ovh.databaseBackupTime` | string or null | no |  | OVH managed MySQL daily backup start time (HH:MM) |
| `target.ovh.databaseBackupRegions` | array or null | no |  | OVH managed MySQL backup regions |
| `target.ovh.databaseDeletionProtection` | boolean or null | no |  | Enable OVH managed MySQL deletion protection |
| `target.ovh.valkeyFlavor` | string or null | no |  | OVH managed Valkey flavor |
| `target.ovh.valkeyPlan` | string or null | no | discovery, essential, business, production | OVH managed Valkey plan (discovery/essential or business/production) |
| `target.ovh.valkeyVersion` | string or null | no | 7.2, 8.0, 8.1, 9.0, 9.1 | OVH managed Valkey engine version (7.2, 8.0, 8.1, 9.0, or 9.1; default 8.1) |
| `target.ovh.valkeyNodeCount` | integer or null | no |  | OVH managed Valkey node count |
| `target.ovh.valkeyBackupTime` | string or null | no |  | OVH managed Valkey daily backup start time (HH:MM) |
| `target.ovh.valkeyBackupRegions` | array or null | no |  | OVH managed Valkey backup regions |
| `target.ovh.valkeyDeletionProtection` | boolean or null | no |  | Enable OVH managed Valkey deletion protection |
| `target.ovh.mksPlan` | string or null | no | free, standard | OVH Managed Kubernetes plan |
| `target.ovh.nodeFlavor` | string or null | no |  | MKS node pool flavor |
| `target.ovh.nodeCount` | integer or null | no |  | MKS node pool size |
| `target.ovh.cpuRequest` | string or null | no |  | Kubernetes CPU request |
| `target.ovh.memoryRequest` | string or null | no |  | Kubernetes memory request |
| `target.ovh.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `target.ovh.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `target.ovh.stateBucket` | string or null | no |  | S3-compatible DIY state bucket name |
| `target.ovh.stateEndpoint` | string or null | no |  | Loopback S3-compatible endpoint override (Floci) |
| `target.ovh.stateRegion` | string or null | no |  | S3-compatible region code for DIY state |
| `target.ovh.labels` | object or null | no |  | Resource labels |
| `target.scaleway` | object or null | no |  | Scaleway deployment inputs (experimental) |
| `target.scaleway.projectId` | string | yes |  | Scaleway project ID |
| `target.scaleway.region` | string or null | no |  | Scaleway region (e.g. fr-par) |
| `target.scaleway.zone` | string or null | no |  | Scaleway availability zone (e.g. fr-par-1) |
| `target.scaleway.networkCidr` | string or null | no |  | Private network IPv4 CIDR |
| `target.scaleway.zones` | array or null | no |  | Scaleway zones |
| `target.scaleway.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `target.scaleway.databaseName` | string or null | no |  | Magento database name |
| `target.scaleway.masterUsername` | string or null | no |  | Managed MySQL master username |
| `target.scaleway.encryptionKeySecret` | string or null | no |  | Secret reference for Magento encryption key |
| `target.scaleway.databaseNodeType` | string or null | no |  | Scaleway RDB node type |
| `target.scaleway.databaseHighAvailability` | boolean | no |  | Enable Scaleway RDB high availability |
| `target.scaleway.databaseBackupEnabled` | boolean or null | no |  | Enable Scaleway RDB automated backups |
| `target.scaleway.databaseBackupFrequencyHours` | integer or null | no |  | Scaleway RDB automated backup frequency in hours |
| `target.scaleway.databaseBackupRetentionDays` | integer or null | no |  | Scaleway RDB automated backup retention in days |
| `target.scaleway.databaseBackupSameRegion` | boolean or null | no |  | Store Scaleway RDB logical backups in the instance region |
| `target.scaleway.databaseEncryptionAtRest` | boolean or null | no |  | Enable Scaleway RDB encryption at rest |
| `target.scaleway.redisNodeType` | string or null | no |  | Scaleway Redis node type |
| `target.scaleway.redisVersion` | string or null | no |  | Scaleway Redis engine version |
| `target.scaleway.redisClusterSize` | integer or null | no |  | Scaleway Redis node count (1 standalone, 2 HA, 3-6 cluster mode) |
| `target.scaleway.cacheMode` | string or null | no | redis | Cache engine escape hatch |
| `target.scaleway.cpuRequest` | string or null | no |  | Kubernetes CPU request |
| `target.scaleway.memoryRequest` | string or null | no |  | Kubernetes memory request |
| `target.scaleway.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `target.scaleway.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `target.scaleway.kapsuleVersion` | string or null | no |  | Kapsule Kubernetes version |
| `target.scaleway.nodeType` | string or null | no |  | Kapsule pool node type |
| `target.scaleway.nodeCount` | integer or null | no |  | Kapsule pool size |
| `target.scaleway.stateBucket` | string or null | no |  | S3-compatible DIY state bucket name |
| `target.scaleway.stateEndpoint` | string or null | no |  | Loopback S3-compatible endpoint override (Floci) |
| `target.scaleway.stateRegion` | string or null | no |  | S3-compatible region code for DIY state |
| `target.scaleway.labels` | object or null | no |  | Resource labels |
| `edge` | object | no |  | Optional edge delivery provider |
| `edge.externalProvider` | string or null | no |  | External edge delivery provider |
| `edge.mode` | string or null | no | none, native, external, both | Edge composition mode |
| `edge.nativeProvider` | string or null | no |  | Provider-native edge implementation |
| `edge.serviceId` | string or null | no |  | Fastly service identifier |
| `edge.tokenSecret` | string or null | no |  | Fastly API token secret reference |
| `edge.domains` | array or null | no |  | Edge hostnames |
| `edge.tls` | boolean | no |  | Require edge TLS |
| `edge.tlsMode` | string or null | no |  | TLS certificate ownership and renewal mode |
| `edge.dnsMode` | string or null | no |  | DNS ownership and convergence mode |
| `edge.vclRef` | string or null | no |  | Reference to reviewed Fastly VCL or policy artifact |
| `edge.purgeOnDeploy` | boolean | no |  | Purge Fastly content after deploy |
| `edge.originHealthRef` | string or null | no |  | Origin health gate reference |
| `edge.cachePolicyRef` | string or null | no |  | Reviewed cache-key and bypass policy reference |
| `edge.purgePolicyRef` | string or null | no |  | Purge verification policy reference |
| `edge.wafPolicyRef` | string or null | no |  | WAF or security-header policy reference |
| `edge.failoverPolicyRef` | string or null | no |  | Origin failover and failback policy reference |
| `edge.ownershipMarker` | string or null | no |  | Edge resource ownership marker |
| `edge.health` | object or null | no |  | External edge health verification policy |
| `edge.health.originUrl` | string or null | no |  | Origin health URL checked before edge mutation |
| `edge.health.originHost` | string or null | no |  | TLS SNI and HTTP Host used for the origin health URL |
| `edge.health.expectedCname` | string or null | no |  | Expected edge DNS CNAME |
| `edge.health.routePath` | string or null | no |  | Path used for post-mutation edge verification |
| `edge.health.expectedStatus` | integer or null | no |  | Expected post-mutation HTTP status |
| `edge.health.routeTimeoutSeconds` | integer or null | no |  | Maximum edge route convergence time |
| `edge.health.routePollSeconds` | integer or null | no |  | Edge route convergence polling interval |
| `observability` | object | no |  | Optional observability provider |
| `observability.nativeProvider` | string or null | no |  | Provider-native observability destination |
| `observability.nativeReference` | string or null | no |  | Opaque provider-owned native destination reference |
| `observability.externalProvider` | string or null | no |  | External observability destination |
| `observability.credentialReferences` | array or null | no |  | Secret references resolved by the observability extension |
| `observability.endpoint` | string or null | no |  | Optional telemetry endpoint |
| `observability.serviceName` | string or null | no |  | Telemetry service name |
| `observability.environment` | string or null | no |  | Telemetry environment name |
| `observability.logs` | boolean | no |  | Export logs |
| `observability.metrics` | boolean | no |  | Export metrics |
| `observability.traces` | boolean | no |  | Export traces |
| `observability.signals` | array or null | no |  | Portable telemetry signal names |
| `observability.retentionDays` | integer or null | no |  | Telemetry retention days |
| `observability.samplingRatio` | number or null | no |  | Trace sampling ratio |
| `observability.redactionPolicyRef` | string or null | no |  | Telemetry redaction policy reference |
| `observability.alertReferences` | array or null | no |  | Alert and dashboard references |
| `observability.labels` | object or null | no |  | Additional resource attributes |
| `observability.dataResidency` | string or null | no |  | Declared telemetry data residency |
| `observability.alerts` | array or null | no |  | Actionable alert policies |
| `observability.dashboards` | array or null | no |  | Operational dashboards |
| `observability.slos` | array or null | no |  | Service-level objectives |
| `email` | object | no |  | Cloud transactional email |
| `email.mode` | string or null | no | disabled, smtp, ses, tem, ovh | Cloud email mode |
| `email.host` | string or null | no |  | SMTP host |
| `email.port` | integer or null | no |  | SMTP port |
| `email.username` | string or null | no |  | SMTP username |
| `email.from` | string or null | no |  | Default sender address |
| `email.credential` | string or null | no |  | SES credential secret reference |
| `email.managed` | object or null | no |  | Managed email provisioning inputs |
| `email.managed.domain` | string or null | no |  | Verified sender domain |
| `email.managed.hostedZoneId` | string or null | no |  | Route 53 hosted zone ID for DKIM records (SES only) |
| `email.managed.account` | string or null | no |  | Mailbox account name (OVH only) |
| `resilience` | object | no |  | Availability, backup, and disaster-recovery policy |
| `resilience.profileId` | string or null | no |  | Named resilience profile |
| `resilience.availabilityTarget` | string or null | no |  | Availability target |
| `resilience.rpoSeconds` | integer or null | no |  | Maximum recovery point objective in seconds |
| `resilience.rtoSeconds` | integer or null | no |  | Maximum recovery time objective in seconds |
| `resilience.retentionDays` | integer or null | no |  | Backup retention days |
| `resilience.recoveryScope` | string or null | no |  | Recovery scope |
| `resilience.failoverOwner` | string or null | no |  | Failover operator |
| `resilience.fencingPolicy` | string or null | no |  | Single-writer and split-brain fencing policy |
| `resilience.dataRegion` | string or null | no |  | Optional region residency for Magento database, media, and backups |
| `resilience.recoveryDestinations` | array or null | no |  | Allowed recovery destinations |
| `resilience.projection` | object or null | no |  | Search and cache rebuild workload target |
| `resilience.projection.runtime` | string | yes | ecs, kubernetes | Projection runtime transport |
| `resilience.projection.cluster` | string or null | no |  | ECS cluster identity |
| `resilience.projection.service` | string or null | no |  | ECS service used to select a running task |
| `resilience.projection.task` | string or null | no |  | Optional pinned ECS task identity |
| `resilience.projection.namespace` | string or null | no |  | Kubernetes workload namespace |
| `resilience.projection.workload` | string or null | no |  | Kubernetes workload app label |
| `resilience.projection.container` | string or null | no |  | Container receiving projection commands |
| `resilience.dataClasses` | array or null | no |  | Independent data-class recovery policies |
| `defaults` | object | yes |  | Project defaults |
| `defaults.region` | string | no |  | Default AWS region |
| `defaults.preset` | string | no | preview, standard, high-availability | Default infrastructure preset |
| `compatibility` | object | no |  | Compatibility policy |
| `compatibility.allowUnsupported` | boolean | no |  | Allow an unsupported service combination |
| `environments` | object | yes |  | Named deployment environments |
| `environments.*.inherits` | string | no |  | Parent environment |
| `environments.*.account` | string or null | no |  | AWS account ID |
| `environments.*.class` | string or null | no |  | Environment class |
| `environments.*.preset` | string or null | no | preview, standard, high-availability | Infrastructure preset |
| `environments.*.domain` | string or null | no |  | Environment domain |
| `environments.*.protection` | boolean or null | no |  | Protect against destructive commands |
| `environments.*.expiresAt` | string or null | no |  | Preview expiration as RFC3339 |
| `environments.*.monthlyBudgetCents` | integer or null | no |  | Monthly budget planning input in cents (alerts only, never enforced) |
| `environments.*.branches` | array or null | no |  | Git branches mapped to this environment |
| `environments.*.seedDump` | string or null | no |  | Local MySQL dump path seeded after first deploy |
| `environments.*.project` | object or null | no |  |  |
| `environments.*.project.name` | string | no |  | Project name |
| `environments.*.application` | object or null | no |  |  |
| `environments.*.application.edition` | string | no | open-source, commerce | Magento edition |
| `environments.*.application.version` | string | no |  | Exact Magento release |
| `environments.*.application.mode` | string | no | integrated, headless | Application mode |
| `environments.*.application.webRuntime` | string | no | nginx-fpm, frankenphp-classic, php-apache | HTTP application runtime |
| `environments.*.application.magento` | object | no |  | Magento runtime overlays |
| `environments.*.application.magento.frontName` | string | no |  | Magento admin frontName |
| `environments.*.application.magento.cookieDomain` | string | no |  | Magento cookie domain |
| `environments.*.application.magento.unsecureBaseUrl` | string | no |  | Unsecure Magento base URL |
| `environments.*.application.magento.secureBaseUrl` | string | no |  | Secure Magento base URL |
| `environments.*.application.magento.corsOrigins` | array | no |  | Allowed CORS origins for the Magento API |
| `environments.*.application.magento.storefrontOrigin` | string | no |  | Headless storefront origin allowed by CORS |
| `environments.*.application.magento.consumers` | object | no |  | Magento message consumer runners |
| `environments.*.application.magento.consumers.mode` | string or null | no | cron, processes, both | Consumer runner |
| `environments.*.application.magento.consumers.names` | array | no |  | Named Magento consumers |
| `environments.*.application.magento.queueTransport` | string or null | no | sqs, pubsub | Optional Magento-module queue transport |
| `environments.*.application.magento.queueModule` | string or null | no |  | Locked Composer Magento package providing SQS or Pub/Sub transport |
| `environments.*.application.magento.variables` | object | no |  | CONFIG__* and MAGENTO_DC_* overlays |
| `environments.*.application.cron` | array | no |  | Portable Magento cron schedule entries |
| `environments.*.build` | object or null | no |  |  |
| `environments.*.build.php` | string | no |  | Exact PHP branch or patch version |
| `environments.*.build.extensions` | array | no |  | Required PHP extensions |
| `environments.*.build.composer` | object | no |  | Composer settings |
| `environments.*.build.composer.version` | string or null | no |  | Required Composer 2 major, minor, or patch version |
| `environments.*.build.composer.credentials` | string | no |  | Composer credentials secret reference |
| `environments.*.build.staticContent` | object | no |  | Static content deployment settings |
| `environments.*.build.staticContent.locales` | array | no |  | Locales passed to setup:static-content:deploy --language |
| `environments.*.build.staticContent.themes` | array | no |  | Themes passed to setup:static-content:deploy --theme |
| `environments.*.build.staticContent.strategy` | string or null | no | quick, standard, compact | Static content deploy strategy (-s) |
| `environments.*.build.staticContent.threads` | integer or null | no |  | Static content deploy thread count (-j) |
| `environments.*.build.qualityPatches` | array | no |  | Quality Patch IDs applied at build |
| `environments.*.build.hooks` | object | no |  | Build lifecycle hooks |
| `environments.*.build.hooks.*.phase` | string | no | validate, build, package | Preparation lifecycle phase |
| `environments.*.build.hooks.*.relationship` | string | no | before, after, replace, disable | Relationship to the target step |
| `environments.*.build.hooks.*.target` | string | no |  | Stable target step ID |
| `environments.*.build.hooks.*.command` | object or null | no |  | Validated command vector |
| `environments.*.build.hooks.*.command.executable` | string | no | composer, magento | Allowed executable |
| `environments.*.build.hooks.*.command.arguments` | array | no |  | Argument vector |
| `environments.*.build.hooks.*.dependencies` | array | no |  | Additional stable step dependencies |
| `environments.*.build.hooks.*.timeoutSeconds` | integer | no |  | Step timeout in seconds |
| `environments.*.build.hooks.*.retries` | object | no |  | Retry policy |
| `environments.*.build.hooks.*.retries.maxAttempts` | integer | no |  | Maximum attempts |
| `environments.*.build.hooks.*.retries.delaySeconds` | integer | no |  | Retry delay in seconds |
| `environments.*.build.hooks.*.retries.idempotent` | boolean | no |  | Whether retrying is safe |
| `environments.*.build.hooks.*.failure` | string | no | abort, continue | Failure action |
| `environments.*.target` | object or null | no |  |  |
| `environments.*.target.provider` | string | no | aws, gcp, ovh, scaleway | Infrastructure provider |
| `environments.*.target.runtime` | string | no | ecs-fargate, eks, gke-autopilot, gke-standard, mks, kapsule | Application runtime |
| `environments.*.target.aws` | object or null | no |  | AWS deployment inputs |
| `environments.*.target.aws.kmsKeyArn` | string or null | no |  | Customer-managed KMS key ARN |
| `environments.*.target.aws.hostedZoneId` | string or null | no |  | Route 53 hosted zone ID |
| `environments.*.target.aws.cloudFrontCertificateArn` | string or null | no |  | us-east-1 ACM certificate ARN |
| `environments.*.target.aws.albCertificateArn` | string or null | no |  | Regional ACM certificate ARN |
| `environments.*.target.aws.snsTopicArn` | string or null | no |  | SNS notification topic ARN |
| `environments.*.target.aws.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `environments.*.target.aws.cacheSecretArn` | string or null | no |  | Valkey auth token secret ARN |
| `environments.*.target.aws.sessionSecretArn` | string or null | no |  | Session auth token secret ARN |
| `environments.*.target.aws.queueSecretArn` | string or null | no |  | RabbitMQ password secret ARN |
| `environments.*.target.aws.encryptionKeySecretArn` | string or null | no |  | Magento encryption key secret ARN |
| `environments.*.target.aws.databaseName` | string or null | no |  | Magento database name |
| `environments.*.target.aws.masterUsername` | string or null | no |  | Database master username |
| `environments.*.target.aws.vpcCidr` | string or null | no |  | Canonical VPC IPv4 CIDR |
| `environments.*.target.aws.availabilityZones` | array or null | no |  | AWS availability zones |
| `environments.*.target.aws.mediaDomain` | string or null | no |  | Media delivery domain |
| `environments.*.target.aws.natMode` | string or null | no | nat-gateway, fck-nat | Private subnet egress mode |
| `environments.*.target.aws.natTopology` | string or null | no | single-az, multi-az | AWS NAT failure-domain topology; omitted defaults to single-AZ for preview and multi-AZ for standard/high-availability |
| `environments.*.target.aws.natReplacementMode` | string or null | no | none, auto-scaling | AWS fck-nat replacement mode; omitted defaults to automatic replacement for multi-AZ fck-nat |
| `environments.*.target.aws.natInstanceType` | string or null | no |  | ARM64 fck-nat instance type; omitted defaults to cost-optimized t4g.nano |
| `environments.*.target.aws.labels` | object or null | no |  | Resource labels |
| `environments.*.target.aws.existing` | object or null | no |  | Existing AWS resources |
| `environments.*.target.aws.existing.network` | object or null | no |  | Existing VPC reference |
| `environments.*.target.aws.existing.network.provider` | string | no | aws | Resource provider |
| `environments.*.target.aws.existing.network.kind` | string | no |  | Resource kind |
| `environments.*.target.aws.existing.network.externalId` | string | no |  | Provider resource identifier |
| `environments.*.target.aws.existing.publicSubnetIds` | array or null | no |  | Existing public subnet IDs |
| `environments.*.target.aws.existing.privateSubnetIds` | array or null | no |  | Existing private subnet IDs |
| `environments.*.target.aws.existing.dataSubnetIds` | array or null | no |  | Existing data subnet IDs |
| `environments.*.target.aws.existing.database` | object or null | no |  | Existing RDS MySQL reference |
| `environments.*.target.aws.existing.database.provider` | string | no | aws | Resource provider |
| `environments.*.target.aws.existing.database.kind` | string | no |  | Resource kind |
| `environments.*.target.aws.existing.database.externalId` | string | no |  | RDS instance identifier or ARN |
| `environments.*.target.aws.existing.database.secretArn` | string | no |  | Secrets Manager master-user secret ARN |
| `environments.*.target.aws.existing.database.endpoint` | string | no |  | RDS writer endpoint hostname |
| `environments.*.target.aws.catalog` | object | no |  | Benchmark-selected service catalog |
| `environments.*.target.aws.catalog.version` | string or null | no |  | Benchmark catalog version |
| `environments.*.target.aws.catalog.databaseEngine` | string or null | no | aurora-mysql, rds-mysql, rds-mariadb | Database engine shape |
| `environments.*.target.aws.catalog.searchMode` | string or null | no | serverless, provisioned, disabled | OpenSearch provisioning mode |
| `environments.*.target.aws.catalog.queueMode` | string or null | no | db, amazon-mq, ecs-rabbitmq, ecs-artemis | Magento messaging broker mode |
| `environments.*.target.aws.catalog.databaseBackupWindow` | string or null | no |  | AWS RDS/Aurora automated backup window in UTC |
| `environments.*.target.aws.catalog.databaseMaintenanceWindow` | string or null | no |  | AWS RDS/Aurora maintenance window in UTC |
| `environments.*.target.aws.catalog.databaseDeletionProtection` | boolean or null | no |  | Enable AWS RDS/Aurora deletion protection |
| `environments.*.target.aws.catalog.databaseDeleteAutomatedBackups` | boolean or null | no |  | Delete AWS RDS/Aurora automated backups when the database is destroyed |
| `environments.*.target.aws.catalog.cacheSnapshotRetentionLimit` | integer or null | no |  | AWS ElastiCache Valkey automatic snapshot retention in days; zero disables snapshots |
| `environments.*.target.aws.catalog.cacheSnapshotWindow` | string or null | no |  | AWS ElastiCache Valkey daily snapshot window in UTC |
| `environments.*.target.aws.catalog.aurora` | object | no |  | Aurora or RDS MySQL capacity |
| `environments.*.target.aws.catalog.aurora.minimumAcu` | number | no |  | Minimum Aurora Serverless v2 capacity |
| `environments.*.target.aws.catalog.aurora.maximumAcu` | number | no |  | Maximum Aurora Serverless v2 capacity |
| `environments.*.target.aws.catalog.aurora.autoPauseSeconds` | integer | no |  | Aurora auto-pause duration |
| `environments.*.target.aws.catalog.aurora.engineSupportsAutoPause` | boolean | no |  | Whether the selected engine supports auto-pause |
| `environments.*.target.aws.catalog.aurora.instanceClass` | string | no |  | Provisioned Aurora or RDS MySQL instance class |
| `environments.*.target.aws.catalog.aurora.instanceCount` | integer | no |  | Provisioned Aurora instance count |
| `environments.*.target.aws.catalog.valkey` | object | no |  | Valkey capacity |
| `environments.*.target.aws.catalog.valkey.nodeType` | string | no |  | Valkey node type |
| `environments.*.target.aws.catalog.valkey.replicaCount` | integer | no |  | Valkey replica count |
| `environments.*.target.aws.catalog.search` | object | no |  | OpenSearch capacity |
| `environments.*.target.aws.catalog.search.maximumIndexingOcu` | number | no |  | OpenSearch Serverless indexing limit |
| `environments.*.target.aws.catalog.search.maximumSearchOcu` | number | no |  | OpenSearch Serverless search limit |
| `environments.*.target.aws.catalog.search.acceptColdStarts` | boolean | no |  | Accept OpenSearch Serverless cold starts |
| `environments.*.target.aws.catalog.search.instanceType` | string | no |  | Provisioned OpenSearch instance type |
| `environments.*.target.aws.catalog.search.instanceCount` | integer | no |  | Provisioned OpenSearch data node count |
| `environments.*.target.aws.catalog.search.dedicatedMasterType` | string | no |  | OpenSearch dedicated master type |
| `environments.*.target.aws.catalog.search.dedicatedMasterCount` | integer | no |  | OpenSearch dedicated master count |
| `environments.*.target.aws.catalog.search.ebsVolumeType` | string | no |  | OpenSearch EBS volume type |
| `environments.*.target.aws.catalog.search.ebsVolumeSizeGiB` | integer | no |  | OpenSearch EBS volume size |
| `environments.*.target.aws.catalog.rabbitMq` | object | no |  | Amazon MQ RabbitMQ capacity (queueMode amazon-mq) |
| `environments.*.target.aws.catalog.rabbitMq.instanceType` | string | no |  | RabbitMQ broker instance type |
| `environments.*.target.aws.catalog.fargate` | object | no |  | Fargate capacity (ecs-fargate) |
| `environments.*.target.aws.catalog.fargate.computeMode` | string or null | no | fargate, fargate-spot, ec2-asg, managed-instances | ECS capacity mode |
| `environments.*.target.aws.catalog.fargate.cpu` | integer | no |  | Fargate task CPU |
| `environments.*.target.aws.catalog.fargate.memoryMiB` | integer | no |  | Fargate task memory |
| `environments.*.target.aws.catalog.fargate.desiredCount` | integer | no |  | Fargate desired task count |
| `environments.*.target.aws.catalog.fargate.instanceType` | string or null | no |  | EC2 or managed-instance capacity type |
| `environments.*.target.aws.catalog.fargate.instanceAmi` | string or null | no |  | Pinned AMI for ECS EC2 capacity |
| `environments.*.target.aws.catalog.fargate.minCapacity` | integer or null | no |  | Minimum host capacity |
| `environments.*.target.aws.catalog.fargate.maxCapacity` | integer or null | no |  | Maximum host capacity |
| `environments.*.target.aws.catalog.eks` | object or null | no |  | EKS Autopilot capacity (eks) |
| `environments.*.target.aws.catalog.eks.computeMode` | string or null | no | auto-mode, managed-node-groups, self-managed, fargate | EKS compute mode |
| `environments.*.target.aws.catalog.eks.kubernetesVersion` | string or null | no |  | EKS Kubernetes minor version |
| `environments.*.target.aws.catalog.eks.cpuRequest` | string or null | no |  | Web/cron CPU request |
| `environments.*.target.aws.catalog.eks.memoryRequest` | string or null | no |  | Web/cron memory request |
| `environments.*.target.aws.catalog.eks.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `environments.*.target.aws.catalog.eks.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `environments.*.target.aws.catalog.eks.searchMode` | string or null | no | opensearch, disabled | EKS OpenSearch workload mode |
| `environments.*.target.aws.catalog.eks.searchReplicas` | integer or null | no |  | OpenSearch StatefulSet replicas |
| `environments.*.target.aws.catalog.eks.queueMode` | string or null | no | database, rabbitmq | EKS queue workload mode |
| `environments.*.target.aws.catalog.eks.queueReplicas` | integer or null | no |  | RabbitMQ StatefulSet replicas |
| `environments.*.target.aws.catalog.eks.nodeInstanceType` | string or null | no |  | EKS node instance type |
| `environments.*.target.aws.catalog.eks.nodeAmi` | string or null | no |  | Pinned AMI for self-managed EKS nodes |
| `environments.*.target.aws.catalog.eks.nodeMinSize` | integer or null | no |  | Minimum EKS node count |
| `environments.*.target.aws.catalog.eks.nodeDesiredSize` | integer or null | no |  | Desired EKS node count |
| `environments.*.target.aws.catalog.eks.nodeMaxSize` | integer or null | no |  | Maximum EKS node count |
| `environments.*.target.aws.catalog.eks.fargateNamespaces` | array or null | no |  | Namespaces scheduled on EKS Fargate |
| `environments.*.target.aws.catalog.retention` | object | no |  | Retention policy |
| `environments.*.target.aws.catalog.retention.logDays` | integer | no |  | CloudWatch log retention days |
| `environments.*.target.aws.catalog.retention.backupDays` | integer | no |  | Database backup retention days |
| `environments.*.target.aws.catalog.retention.artifactDays` | integer | no |  | Artifact retention days |
| `environments.*.target.aws.catalog.versions` | object | no |  | Managed service versions |
| `environments.*.target.aws.catalog.versions.auroraMysql` | string | no |  | Aurora MySQL engine version |
| `environments.*.target.aws.catalog.versions.mysql` | string | no |  | RDS MySQL engine version |
| `environments.*.target.aws.catalog.versions.mariaDb` | string | no |  | RDS MariaDB engine version |
| `environments.*.target.aws.catalog.versions.valkey` | string | no |  | Valkey engine version |
| `environments.*.target.aws.catalog.versions.openSearch` | string | no |  | OpenSearch engine version |
| `environments.*.target.aws.catalog.versions.rabbitMq` | string | no |  | RabbitMQ engine version |
| `environments.*.target.gcp` | object or null | no |  | GCP deployment inputs |
| `environments.*.target.gcp.project` | string | no |  | GCP project ID |
| `environments.*.target.gcp.region` | string or null | no |  | GCP region |
| `environments.*.target.gcp.networkCidr` | string or null | no |  | VPC IPv4 CIDR |
| `environments.*.target.gcp.zones` | array or null | no |  | GCP zones |
| `environments.*.target.gcp.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `environments.*.target.gcp.databaseName` | string or null | no |  | Magento database name |
| `environments.*.target.gcp.masterUsername` | string or null | no |  | Cloud SQL master username |
| `environments.*.target.gcp.encryptionKeySecret` | string or null | no |  | Secret Manager secret ID for Magento encryption key |
| `environments.*.target.gcp.cloudSqlTier` | string or null | no |  | Cloud SQL machine tier |
| `environments.*.target.gcp.cloudSqlAvailability` | string or null | no | ZONAL, REGIONAL | Cloud SQL availability type |
| `environments.*.target.gcp.cloudSqlBackupEnabled` | boolean or null | no |  | Enable Cloud SQL automated backups |
| `environments.*.target.gcp.cloudSqlBinaryLogEnabled` | boolean or null | no |  | Enable Cloud SQL MySQL binary logging for point-in-time recovery |
| `environments.*.target.gcp.cloudSqlBackupRetentionCount` | integer or null | no |  | Cloud SQL retained automated backup count |
| `environments.*.target.gcp.cloudSqlTransactionLogRetentionDays` | integer or null | no |  | Cloud SQL MySQL transaction log retention in days (1-7) |
| `environments.*.target.gcp.cloudSqlBackupStartTime` | string or null | no |  | Cloud SQL automated backup start time (HH:MM) |
| `environments.*.target.gcp.cloudSqlBackupLocation` | string or null | no |  | Cloud SQL automated backup storage location |
| `environments.*.target.gcp.cloudSqlDeletionProtection` | boolean or null | no |  | Enable Cloud SQL service and IaC deletion protection |
| `environments.*.target.gcp.memorystoreNodeType` | string or null | no |  | Memorystore for Valkey node type |
| `environments.*.target.gcp.memorystoreShardCount` | integer or null | no |  | Memorystore for Valkey shard count |
| `environments.*.target.gcp.memorystoreReplicas` | integer or null | no |  | Memorystore for Valkey replica count per shard (0-5) |
| `environments.*.target.gcp.memorystoreEngineVersion` | string or null | no | VALKEY_8_0, VALKEY_9_0, VALKEY_9_1 | Memorystore for Valkey engine version (VALKEY_9_0 GA or VALKEY_9_1 Preview) |
| `environments.*.target.gcp.memorystoreMode` | string or null | no | CLUSTER, CLUSTER_DISABLED | Memorystore for Valkey cluster mode |
| `environments.*.target.gcp.memorystoreZoneDistributionMode` | string or null | no | MULTI_ZONE, SINGLE_ZONE | Memorystore for Valkey zone distribution mode |
| `environments.*.target.gcp.memorystoreZone` | string or null | no |  | Memorystore for Valkey single-zone placement |
| `environments.*.target.gcp.memorystoreDeletionProtection` | boolean or null | no |  | Enable Memorystore for Valkey deletion protection |
| `environments.*.target.gcp.memorystorePscConnectionLimit` | integer or null | no |  | Maximum automatic Memorystore PSC connections |
| `environments.*.target.gcp.openSearchMode` | string or null | no | opensearch, disabled | GKE OpenSearch workload mode |
| `environments.*.target.gcp.openSearchReplicas` | integer or null | no |  | GKE OpenSearch replica count |
| `environments.*.target.gcp.openSearchImage` | string or null | no |  | GKE OpenSearch image reference |
| `environments.*.target.gcp.queueMode` | string or null | no | database, rabbitmq | GKE queue workload mode |
| `environments.*.target.gcp.queueReplicas` | integer or null | no |  | GKE RabbitMQ replica count |
| `environments.*.target.gcp.rabbitMqImage` | string or null | no |  | GKE RabbitMQ image reference |
| `environments.*.target.gcp.enableCloudArmor` | boolean or null | no |  | Create a GCP Cloud Armor security policy |
| `environments.*.target.gcp.kubernetesVersion` | string or null | no |  | GKE Kubernetes version |
| `environments.*.target.gcp.releaseChannel` | string or null | no | RAPID, REGULAR, STABLE | GKE release channel |
| `environments.*.target.gcp.clusterIpv4Cidr` | string or null | no |  | GKE cluster pod IPv4 CIDR |
| `environments.*.target.gcp.servicesIpv4Cidr` | string or null | no |  | GKE services IPv4 CIDR |
| `environments.*.target.gcp.standardNodeType` | string or null | no |  | GKE Standard node machine type |
| `environments.*.target.gcp.standardNodeCount` | integer or null | no |  | GKE Standard initial node count per zone |
| `environments.*.target.gcp.standardNodeMinCount` | integer or null | no |  | GKE Standard minimum nodes per zone |
| `environments.*.target.gcp.standardNodeMaxCount` | integer or null | no |  | GKE Standard maximum nodes per zone |
| `environments.*.target.gcp.standardNodeDiskType` | string or null | no |  | GKE Standard node boot disk type |
| `environments.*.target.gcp.standardNodeDiskSizeGiB` | integer or null | no |  | GKE Standard node boot disk size |
| `environments.*.target.gcp.standardNodeImageType` | string or null | no |  | GKE Standard node image type |
| `environments.*.target.gcp.standardNodeSpot` | boolean | no |  | Use Spot VMs for GKE Standard nodes |
| `environments.*.target.gcp.autopilotCpuRequest` | string or null | no |  | GKE Autopilot CPU request |
| `environments.*.target.gcp.autopilotMemoryRequest` | string or null | no |  | GKE Autopilot memory request |
| `environments.*.target.gcp.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `environments.*.target.gcp.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `environments.*.target.gcp.labels` | object or null | no |  | Resource labels |
| `environments.*.target.ovh` | object or null | no |  | OVHcloud deployment inputs (experimental) |
| `environments.*.target.ovh.serviceName` | string | no |  | OVH Public Cloud project service name |
| `environments.*.target.ovh.apiEndpoint` | string or null | no | ovh-eu, ovh-ca, ovh-us | OVHcloud API endpoint alias (ovh-eu, ovh-ca, or ovh-us) |
| `environments.*.target.ovh.region` | string or null | no |  | OVH Public Cloud region (for example EU-WEST-PAR or GRA11) |
| `environments.*.target.ovh.networkCidr` | string or null | no |  | Private network IPv4 CIDR |
| `environments.*.target.ovh.zones` | array or null | no |  | OVH MKS availability zones within the selected region |
| `environments.*.target.ovh.attachFloatingIps` | boolean | no |  | Attach OVH floating IPs to MKS nodes |
| `environments.*.target.ovh.privateNetworkRoutingAsDefault` | boolean | no |  | Route MKS egress through OVH's documented private-subnet DHCP gateway; custom gateway routing is not inferred |
| `environments.*.target.ovh.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `environments.*.target.ovh.databaseName` | string or null | no |  | Magento database name |
| `environments.*.target.ovh.masterUsername` | string or null | no |  | Managed MySQL master username |
| `environments.*.target.ovh.encryptionKeySecret` | string or null | no |  | Secret reference for Magento encryption key |
| `environments.*.target.ovh.databaseFlavor` | string or null | no |  | OVH managed MySQL flavor |
| `environments.*.target.ovh.databasePlan` | string or null | no | discovery, essential, business, production, enterprise, advanced | OVH managed MySQL plan (discovery/essential, business/production, or enterprise/advanced) |
| `environments.*.target.ovh.databaseVersion` | string or null | no | 8.0, 8.4 | OVH managed MySQL engine version (8.0 or 8.4) |
| `environments.*.target.ovh.databaseNodeCount` | integer or null | no |  | OVH managed MySQL node count |
| `environments.*.target.ovh.databaseBackupTime` | string or null | no |  | OVH managed MySQL daily backup start time (HH:MM) |
| `environments.*.target.ovh.databaseBackupRegions` | array or null | no |  | OVH managed MySQL backup regions |
| `environments.*.target.ovh.databaseDeletionProtection` | boolean or null | no |  | Enable OVH managed MySQL deletion protection |
| `environments.*.target.ovh.valkeyFlavor` | string or null | no |  | OVH managed Valkey flavor |
| `environments.*.target.ovh.valkeyPlan` | string or null | no | discovery, essential, business, production | OVH managed Valkey plan (discovery/essential or business/production) |
| `environments.*.target.ovh.valkeyVersion` | string or null | no | 7.2, 8.0, 8.1, 9.0, 9.1 | OVH managed Valkey engine version (7.2, 8.0, 8.1, 9.0, or 9.1; default 8.1) |
| `environments.*.target.ovh.valkeyNodeCount` | integer or null | no |  | OVH managed Valkey node count |
| `environments.*.target.ovh.valkeyBackupTime` | string or null | no |  | OVH managed Valkey daily backup start time (HH:MM) |
| `environments.*.target.ovh.valkeyBackupRegions` | array or null | no |  | OVH managed Valkey backup regions |
| `environments.*.target.ovh.valkeyDeletionProtection` | boolean or null | no |  | Enable OVH managed Valkey deletion protection |
| `environments.*.target.ovh.mksPlan` | string or null | no | free, standard | OVH Managed Kubernetes plan |
| `environments.*.target.ovh.nodeFlavor` | string or null | no |  | MKS node pool flavor |
| `environments.*.target.ovh.nodeCount` | integer or null | no |  | MKS node pool size |
| `environments.*.target.ovh.cpuRequest` | string or null | no |  | Kubernetes CPU request |
| `environments.*.target.ovh.memoryRequest` | string or null | no |  | Kubernetes memory request |
| `environments.*.target.ovh.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `environments.*.target.ovh.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `environments.*.target.ovh.stateBucket` | string or null | no |  | S3-compatible DIY state bucket name |
| `environments.*.target.ovh.stateEndpoint` | string or null | no |  | Loopback S3-compatible endpoint override (Floci) |
| `environments.*.target.ovh.stateRegion` | string or null | no |  | S3-compatible region code for DIY state |
| `environments.*.target.ovh.labels` | object or null | no |  | Resource labels |
| `environments.*.target.scaleway` | object or null | no |  | Scaleway deployment inputs (experimental) |
| `environments.*.target.scaleway.projectId` | string | no |  | Scaleway project ID |
| `environments.*.target.scaleway.region` | string or null | no |  | Scaleway region (e.g. fr-par) |
| `environments.*.target.scaleway.zone` | string or null | no |  | Scaleway availability zone (e.g. fr-par-1) |
| `environments.*.target.scaleway.networkCidr` | string or null | no |  | Private network IPv4 CIDR |
| `environments.*.target.scaleway.zones` | array or null | no |  | Scaleway zones |
| `environments.*.target.scaleway.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `environments.*.target.scaleway.databaseName` | string or null | no |  | Magento database name |
| `environments.*.target.scaleway.masterUsername` | string or null | no |  | Managed MySQL master username |
| `environments.*.target.scaleway.encryptionKeySecret` | string or null | no |  | Secret reference for Magento encryption key |
| `environments.*.target.scaleway.databaseNodeType` | string or null | no |  | Scaleway RDB node type |
| `environments.*.target.scaleway.databaseHighAvailability` | boolean | no |  | Enable Scaleway RDB high availability |
| `environments.*.target.scaleway.databaseBackupEnabled` | boolean or null | no |  | Enable Scaleway RDB automated backups |
| `environments.*.target.scaleway.databaseBackupFrequencyHours` | integer or null | no |  | Scaleway RDB automated backup frequency in hours |
| `environments.*.target.scaleway.databaseBackupRetentionDays` | integer or null | no |  | Scaleway RDB automated backup retention in days |
| `environments.*.target.scaleway.databaseBackupSameRegion` | boolean or null | no |  | Store Scaleway RDB logical backups in the instance region |
| `environments.*.target.scaleway.databaseEncryptionAtRest` | boolean or null | no |  | Enable Scaleway RDB encryption at rest |
| `environments.*.target.scaleway.redisNodeType` | string or null | no |  | Scaleway Redis node type |
| `environments.*.target.scaleway.redisVersion` | string or null | no |  | Scaleway Redis engine version |
| `environments.*.target.scaleway.redisClusterSize` | integer or null | no |  | Scaleway Redis node count (1 standalone, 2 HA, 3-6 cluster mode) |
| `environments.*.target.scaleway.cacheMode` | string or null | no | redis | Cache engine escape hatch |
| `environments.*.target.scaleway.cpuRequest` | string or null | no |  | Kubernetes CPU request |
| `environments.*.target.scaleway.memoryRequest` | string or null | no |  | Kubernetes memory request |
| `environments.*.target.scaleway.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `environments.*.target.scaleway.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `environments.*.target.scaleway.kapsuleVersion` | string or null | no |  | Kapsule Kubernetes version |
| `environments.*.target.scaleway.nodeType` | string or null | no |  | Kapsule pool node type |
| `environments.*.target.scaleway.nodeCount` | integer or null | no |  | Kapsule pool size |
| `environments.*.target.scaleway.stateBucket` | string or null | no |  | S3-compatible DIY state bucket name |
| `environments.*.target.scaleway.stateEndpoint` | string or null | no |  | Loopback S3-compatible endpoint override (Floci) |
| `environments.*.target.scaleway.stateRegion` | string or null | no |  | S3-compatible region code for DIY state |
| `environments.*.target.scaleway.labels` | object or null | no |  | Resource labels |
| `environments.*.edge` | object or null | no |  |  |
| `environments.*.edge.externalProvider` | string or null | no |  | External edge delivery provider |
| `environments.*.edge.mode` | string or null | no | none, native, external, both | Edge composition mode |
| `environments.*.edge.nativeProvider` | string or null | no |  | Provider-native edge implementation |
| `environments.*.edge.serviceId` | string or null | no |  | Fastly service identifier |
| `environments.*.edge.tokenSecret` | string or null | no |  | Fastly API token secret reference |
| `environments.*.edge.domains` | array or null | no |  | Edge hostnames |
| `environments.*.edge.tls` | boolean | no |  | Require edge TLS |
| `environments.*.edge.tlsMode` | string or null | no |  | TLS certificate ownership and renewal mode |
| `environments.*.edge.dnsMode` | string or null | no |  | DNS ownership and convergence mode |
| `environments.*.edge.vclRef` | string or null | no |  | Reference to reviewed Fastly VCL or policy artifact |
| `environments.*.edge.purgeOnDeploy` | boolean | no |  | Purge Fastly content after deploy |
| `environments.*.edge.originHealthRef` | string or null | no |  | Origin health gate reference |
| `environments.*.edge.cachePolicyRef` | string or null | no |  | Reviewed cache-key and bypass policy reference |
| `environments.*.edge.purgePolicyRef` | string or null | no |  | Purge verification policy reference |
| `environments.*.edge.wafPolicyRef` | string or null | no |  | WAF or security-header policy reference |
| `environments.*.edge.failoverPolicyRef` | string or null | no |  | Origin failover and failback policy reference |
| `environments.*.edge.ownershipMarker` | string or null | no |  | Edge resource ownership marker |
| `environments.*.edge.health` | object or null | no |  | External edge health verification policy |
| `environments.*.edge.health.originUrl` | string or null | no |  | Origin health URL checked before edge mutation |
| `environments.*.edge.health.originHost` | string or null | no |  | TLS SNI and HTTP Host used for the origin health URL |
| `environments.*.edge.health.expectedCname` | string or null | no |  | Expected edge DNS CNAME |
| `environments.*.edge.health.routePath` | string or null | no |  | Path used for post-mutation edge verification |
| `environments.*.edge.health.expectedStatus` | integer or null | no |  | Expected post-mutation HTTP status |
| `environments.*.edge.health.routeTimeoutSeconds` | integer or null | no |  | Maximum edge route convergence time |
| `environments.*.edge.health.routePollSeconds` | integer or null | no |  | Edge route convergence polling interval |
| `environments.*.observability` | object or null | no |  |  |
| `environments.*.observability.nativeProvider` | string or null | no |  | Provider-native observability destination |
| `environments.*.observability.nativeReference` | string or null | no |  | Opaque provider-owned native destination reference |
| `environments.*.observability.externalProvider` | string or null | no |  | External observability destination |
| `environments.*.observability.credentialReferences` | array or null | no |  | Secret references resolved by the observability extension |
| `environments.*.observability.endpoint` | string or null | no |  | Optional telemetry endpoint |
| `environments.*.observability.serviceName` | string or null | no |  | Telemetry service name |
| `environments.*.observability.environment` | string or null | no |  | Telemetry environment name |
| `environments.*.observability.logs` | boolean | no |  | Export logs |
| `environments.*.observability.metrics` | boolean | no |  | Export metrics |
| `environments.*.observability.traces` | boolean | no |  | Export traces |
| `environments.*.observability.signals` | array or null | no |  | Portable telemetry signal names |
| `environments.*.observability.retentionDays` | integer or null | no |  | Telemetry retention days |
| `environments.*.observability.samplingRatio` | number or null | no |  | Trace sampling ratio |
| `environments.*.observability.redactionPolicyRef` | string or null | no |  | Telemetry redaction policy reference |
| `environments.*.observability.alertReferences` | array or null | no |  | Alert and dashboard references |
| `environments.*.observability.labels` | object or null | no |  | Additional resource attributes |
| `environments.*.observability.dataResidency` | string or null | no |  | Declared telemetry data residency |
| `environments.*.observability.alerts` | array or null | no |  | Actionable alert policies |
| `environments.*.observability.dashboards` | array or null | no |  | Operational dashboards |
| `environments.*.observability.slos` | array or null | no |  | Service-level objectives |
| `environments.*.email` | object or null | no |  |  |
| `environments.*.email.mode` | string or null | no | disabled, smtp, ses, tem, ovh | Cloud email mode |
| `environments.*.email.host` | string or null | no |  | SMTP host |
| `environments.*.email.port` | integer or null | no |  | SMTP port |
| `environments.*.email.username` | string or null | no |  | SMTP username |
| `environments.*.email.from` | string or null | no |  | Default sender address |
| `environments.*.email.credential` | string or null | no |  | SES credential secret reference |
| `environments.*.email.managed` | object or null | no |  | Managed email provisioning inputs |
| `environments.*.email.managed.domain` | string or null | no |  | Verified sender domain |
| `environments.*.email.managed.hostedZoneId` | string or null | no |  | Route 53 hosted zone ID for DKIM records (SES only) |
| `environments.*.email.managed.account` | string or null | no |  | Mailbox account name (OVH only) |
| `environments.*.resilience` | object or null | no |  |  |
| `environments.*.resilience.profileId` | string or null | no |  | Named resilience profile |
| `environments.*.resilience.availabilityTarget` | string or null | no |  | Availability target |
| `environments.*.resilience.rpoSeconds` | integer or null | no |  | Maximum recovery point objective in seconds |
| `environments.*.resilience.rtoSeconds` | integer or null | no |  | Maximum recovery time objective in seconds |
| `environments.*.resilience.retentionDays` | integer or null | no |  | Backup retention days |
| `environments.*.resilience.recoveryScope` | string or null | no |  | Recovery scope |
| `environments.*.resilience.failoverOwner` | string or null | no |  | Failover operator |
| `environments.*.resilience.fencingPolicy` | string or null | no |  | Single-writer and split-brain fencing policy |
| `environments.*.resilience.dataRegion` | string or null | no |  | Optional region residency for Magento database, media, and backups |
| `environments.*.resilience.recoveryDestinations` | array or null | no |  | Allowed recovery destinations |
| `environments.*.resilience.projection` | object or null | no |  | Search and cache rebuild workload target |
| `environments.*.resilience.projection.runtime` | string | no | ecs, kubernetes | Projection runtime transport |
| `environments.*.resilience.projection.cluster` | string or null | no |  | ECS cluster identity |
| `environments.*.resilience.projection.service` | string or null | no |  | ECS service used to select a running task |
| `environments.*.resilience.projection.task` | string or null | no |  | Optional pinned ECS task identity |
| `environments.*.resilience.projection.namespace` | string or null | no |  | Kubernetes workload namespace |
| `environments.*.resilience.projection.workload` | string or null | no |  | Kubernetes workload app label |
| `environments.*.resilience.projection.container` | string or null | no |  | Container receiving projection commands |
| `environments.*.resilience.dataClasses` | array or null | no |  | Independent data-class recovery policies |
| `environments.*.defaults` | object or null | no |  |  |
| `environments.*.defaults.region` | string | no |  | Default AWS region |
| `environments.*.defaults.preset` | string | no | preview, standard, high-availability | Default infrastructure preset |
| `environments.*.compatibility` | object or null | no |  |  |
| `environments.*.compatibility.allowUnsupported` | boolean | no |  | Allow an unsupported service combination |
| `environments.*.extensions` | object | no |  |  |
| `extensions` | object | no |  | Namespaced extension settings |

Environment values override project values. A `null` environment value removes an optional inherited value. Entries under `extensions` may use namespaced fields that MageLift does not inspect.
