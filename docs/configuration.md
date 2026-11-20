# Configuration reference

Use `magelift.php` to choose a cloud, set service versions, and name environments such
as staging and production. MageLift reads schema version 1 and reports unknown fields.
Start with [Getting started](getting-started.md) if you are setting up a shop.

A preset fills in common service and capacity settings. Values in the project file
override the preset, and environment settings override project settings. Run
`magelift config effective --env staging` to see the resulting values and where
they came from. Set an optional environment value to `null` to remove an inherited
value. Keep the cloud-specific settings under `target.aws`, `target.gcp`, or
`target.azure`, according to `target.provider`.

## Build and email

`build.extensions` lists the PHP extensions your shop needs. The builder checks
that it loads them and that its Composer version matches your request before it
installs packages. Set `build.composer.version` to a branch such as `2.10`, an
exact release such as `2.11.2`, a minimum such as `2.9.8+`, or a patch range
such as `~2.9.8`. Set `build.composer.image` to a digest-pinned image if you
need another Composer binary. [Composer build settings](builds.md#composer)

Use `email` to configure a shop's cloud mail relay. Provide a secret reference in
`email.credential` for authenticated SMTP. On AWS, use `email.mode: ses` with
`email.managed.domain` and `email.managed.hostedZoneId` to let MageLift create the
sender identity, SMTP credentials and DKIM records. Managed SES takes care of
the host, port and login values. Preview environments use disabled email by
default. Use `local.email` for Docker Compose and pass a password through the
variable named by `local.email.credentialEnv`.

## Cloud settings

On AWS, `target.aws.encryptionKeySecretArn` refers to the Secrets Manager value
for the Magento encryption key. Keep that value for as long as you keep the
encrypted data. If you use an existing VPC, supply one public, private and data
subnet ID per selected zone. The network must already have egress and access to
the AWS services your shop uses. Set `target.aws.existing.database` to adopt an
existing RDS instance. [Connect existing resources](brownfield-attach.md)

GKE Autopilot and GKE Standard require `target.gcp.project`. Choose Standard for
workloads such as multi-node OpenSearch that need control over the worker nodes.
Set `memorystoreMode`, `memorystoreShardCount`, and `memorystoreReplicas` to
select the cache configuration. Cluster mode disabled supports one shard.
Choose cluster mode enabled for multiple shards.

AWS and Google Cloud select backup retention and deletion protection from the
preset and environment class. Preview defaults limit the resources retained
after teardown. Check the effective configuration before deployment. Explicit
`false` and `0` values are preserved. [Service settings](service-bindings.md)

## Edge and telemetry

Set `observability.nativeProvider` to `cloudwatch` on AWS or
`google-cloud-operations` on Google Cloud. To send signals to an external
adapter, use `observability.externalProvider` and put vendor-specific settings
under `extensions.<vendor>`. `observability.nativeReference` identifies an
existing destination, such as a Google Cloud Monitoring service. Store
credentials as secret references.

For an external edge adapter, set `edge.health.expectedCname` to the DNS
target the provider gave you. For standalone `magelift edge apply`, set
`edge.health.originUrl` to the origin's health endpoint. A deployment can use
the `applicationURL` output to find that endpoint. Set `edge.health.originHost`
if the certificate uses a different hostname. With `edge.tlsMode: external`,
you supply and renew the certificate. [Operate a shop](operations.md)

## Fields

| Field | Type | Required | Accepted values | Description |
| --- | --- | --- | --- | --- |
| `services` | object | no |  | Magento service families, versions, ownership and connection settings |
| `services.*.parameters` | object | no |  | Database server defaults for charset, collation and packet size |
| `services.*.adapter` | string | no |  | Explicitly linked service connection adapter ID |
| `services.*.family` | string | yes |  | Service engine or implementation |
| `services.*.version` | string | no |  | Selected engine version |
| `services.*.ownership` | string or null | no | managed, self-hosted, existing, external | Provisioning ownership |
| `services.*.endpoint` | string | no |  | Hostname or URL of an existing service |
| `services.*.secret` | string | no |  | Provider secret reference for a connection |
| `services.*.image` | string | no |  | Shop-selected service container image |
| `services.*.port` | integer | no |  | Service connection port |
| `services.*.username` | string | no |  | Service connection username |
| `services.*.evictionPolicy` | string | no |  | Eviction policy for the cache or session store |
| `services.*.plugins` | array | no |  | OpenSearch plugins required by the shop |
| `services.*.memoryMiB` | integer | no |  | Redis or Valkey eviction threshold in MiB |
| `services.*.storageGiB` | integer | no |  | Persistent volume size in GiB for a self-hosted service |
| `schemaVersion` | integer | yes | 1 | MageLift configuration schema version |
| `project` | object | yes |  | Project identity |
| `project.name` | string | yes |  | Project name |
| `application` | object | yes |  | Magento application settings |
| `application.edition` | string | yes | open-source, commerce | Magento edition |
| `application.version` | string | yes |  | Exact Magento release |
| `application.mode` | string | yes | integrated, headless | Application mode |
| `application.webRuntime` | string | no | nginx-fpm | HTTP runtime (nginx and PHP-FPM) |
| `application.magento` | object | no |  | Magento runtime overlays |
| `application.magento.frontName` | string | no |  | Magento admin frontName |
| `application.magento.cookieDomain` | string | no |  | Magento cookie domain |
| `application.magento.unsecureBaseUrl` | string | no |  | Unsecure Magento base URL |
| `application.magento.secureBaseUrl` | string | no |  | Secure Magento base URL |
| `application.magento.corsOrigins` | array | no |  | Allowed CORS origins for the Magento API |
| `application.magento.storefrontOrigin` | string | no |  | Headless storefront origin allowed by CORS |
| `application.magento.consumers` | object | no |  | Magento message consumer runners |
| `application.magento.consumers.mode` | string or null | no | cron, processes, both, disabled | Consumer runner |
| `application.magento.consumers.names` | array | no |  | Named Magento consumers |
| `application.magento.queueTransport` | string or null | no | sqs, pubsub | Optional Magento-module queue transport |
| `application.magento.queueModule` | string or null | no |  | Locked Composer Magento package providing SQS or Pub/Sub transport |
| `application.magento.variables` | object | no |  | CONFIG__* and MAGENTO_DC_* overlays |
| `application.cron` | array | no |  | Portable Magento cron schedule entries |
| `build` | object | yes |  | Application build settings |
| `build.php` | string | yes |  | Exact PHP branch or patch version |
| `build.extensions` | array | no |  | Required PHP extensions |
| `build.composer` | object | no |  | Composer settings |
| `build.composer.image` | string or null | no |  | Digest-pinned Composer image |
| `build.composer.version` | string or null | no |  | Required Composer 2 major, minor, or patch version |
| `build.composer.credentials` | string | no |  | Composer credentials secret reference |
| `build.staticContent` | object | no |  | Static content deployment settings |
| `build.staticContent.matrix` | object | no |  | Locales to build for each Magento theme |
| `build.staticContent.locales` | array | no |  | Locales passed to setup:static-content:deploy --language |
| `build.staticContent.themes` | array | no |  | Themes passed to setup:static-content:deploy --theme |
| `build.staticContent.strategy` | string or null | no | quick, standard, compact | Static content deploy strategy (-s) |
| `build.staticContent.threads` | integer or null | no |  | Static content deploy thread count (-j) |
| `build.qualityPatches` | array | no |  | Quality Patch IDs applied at build |
| `build.cloudPatches` | boolean | no |  | Opt in to upstream Cloud Patches apply during build |
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
| `build.cache` | object or null | no |  | Optional user-owned BuildKit registry cache |
| `build.cache.from` | array | no |  | Registry references imported before the build |
| `build.cache.to` | string or null | no |  | Registry reference receiving exported cache layers |
| `build.cache.mode` | string or null | no | min, max | BuildKit cache export mode |
| `build.cache.ignoreExportErrors` | boolean | no |  | Continue when cache export fails |
| `local` | object | no |  | Local runtime service and email choices |
| `local.session` | object | no |  | Separate cache engine for Magento sessions |
| `local.session.parameters` | object | no |  | Local database server defaults |
| `local.session.memoryMiB` | integer | no |  | Local cache or session memory limit in MiB |
| `local.session.evictionPolicy` | string | no |  | Valkey or Redis eviction policy |
| `local.session.plugins` | array | no |  | OpenSearch plugins to load |
| `local.session.image` | string or null | no |  | Container image selected by the project for local development |
| `local.session.family` | string or null | no |  | Service family |
| `local.session.version` | string or null | no |  | Service version |
| `local.database` | object | no |  | Local database family and version |
| `local.database.parameters` | object | no |  | Local database server defaults |
| `local.database.memoryMiB` | integer | no |  | Local cache or session memory limit in MiB |
| `local.database.evictionPolicy` | string | no |  | Valkey or Redis eviction policy |
| `local.database.plugins` | array | no |  | OpenSearch plugins to load |
| `local.database.image` | string or null | no |  | Container image selected by the project for local development |
| `local.database.family` | string or null | no |  | Service family |
| `local.database.version` | string or null | no |  | Service version |
| `local.cache` | object | no |  | Local cache family and version |
| `local.cache.parameters` | object | no |  | Local database server defaults |
| `local.cache.memoryMiB` | integer | no |  | Local cache or session memory limit in MiB |
| `local.cache.evictionPolicy` | string | no |  | Valkey or Redis eviction policy |
| `local.cache.plugins` | array | no |  | OpenSearch plugins to load |
| `local.cache.image` | string or null | no |  | Container image selected by the project for local development |
| `local.cache.family` | string or null | no |  | Service family |
| `local.cache.version` | string or null | no |  | Service version |
| `local.search` | object | no |  | Local search family and version |
| `local.search.parameters` | object | no |  | Local database server defaults |
| `local.search.memoryMiB` | integer | no |  | Local cache or session memory limit in MiB |
| `local.search.evictionPolicy` | string | no |  | Valkey or Redis eviction policy |
| `local.search.plugins` | array | no |  | OpenSearch plugins to load |
| `local.search.image` | string or null | no |  | Container image selected by the project for local development |
| `local.search.family` | string or null | no |  | Service family |
| `local.search.version` | string or null | no |  | Service version |
| `local.queue` | object | no |  | Local queue family and version |
| `local.queue.parameters` | object | no |  | Local database server defaults |
| `local.queue.memoryMiB` | integer | no |  | Local cache or session memory limit in MiB |
| `local.queue.evictionPolicy` | string | no |  | Valkey or Redis eviction policy |
| `local.queue.plugins` | array | no |  | OpenSearch plugins to load |
| `local.queue.image` | string or null | no |  | Container image selected by the project for local development |
| `local.queue.family` | string or null | no |  | Service family |
| `local.queue.version` | string or null | no |  | Service version |
| `local.webServer` | object | no |  | Local web server family and version |
| `local.webServer.parameters` | object | no |  | Local database server defaults |
| `local.webServer.memoryMiB` | integer | no |  | Local cache or session memory limit in MiB |
| `local.webServer.evictionPolicy` | string | no |  | Valkey or Redis eviction policy |
| `local.webServer.plugins` | array | no |  | OpenSearch plugins to load |
| `local.webServer.image` | string or null | no |  | Container image selected by the project for local development |
| `local.webServer.family` | string or null | no |  | Service family |
| `local.webServer.version` | string or null | no |  | Service version |
| `local.webCache` | object | no |  | Local web cache family and version |
| `local.webCache.parameters` | object | no |  | Local database server defaults |
| `local.webCache.memoryMiB` | integer | no |  | Local cache or session memory limit in MiB |
| `local.webCache.evictionPolicy` | string | no |  | Valkey or Redis eviction policy |
| `local.webCache.plugins` | array | no |  | OpenSearch plugins to load |
| `local.webCache.image` | string or null | no |  | Container image selected by the project for local development |
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
| `target.provider` | string | yes | aws, gcp, azure | Infrastructure provider |
| `target.runtime` | string | yes | ecs-fargate, eks, gke-autopilot, gke-standard, aks | Application runtime |
| `target.aws` | object or null | no |  | AWS deployment inputs |
| `target.aws.previewPool` | object or null | no |  | Shared preview compute in an existing VPC |
| `target.aws.previewPool.clusterArn` | string | yes |  | ECS Fargate cluster owned outside the preview stack |
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
| `target.aws.natTopology` | string or null | no | single-az, multi-az | NAT placement across AWS availability zones. Defaults to one zone for preview and multiple zones for standard or high availability |
| `target.aws.natReplacementMode` | string or null | no | none, auto-scaling | How AWS replaces fck-nat instances. Defaults to automatic replacement for multi-AZ fck-nat |
| `target.aws.natInstanceType` | string or null | no |  | ARM64 instance type for fck-nat. Defaults to t4g.nano to reduce cost |
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
| `target.aws.catalog` | object | no |  | AWS runtime and service settings |
| `target.aws.catalog.version` | string or null | no |  | AWS service settings version |
| `target.aws.catalog.databaseEngine` | string or null | no | aurora-mysql, rds-mysql, rds-mariadb | Database engine shape |
| `target.aws.catalog.searchMode` | string or null | no | serverless, provisioned, disabled | OpenSearch provisioning mode |
| `target.aws.catalog.queueMode` | string or null | no | db, amazon-mq, ecs-rabbitmq, ecs-artemis | Magento messaging broker mode |
| `target.aws.catalog.databaseBackupWindow` | string or null | no |  | AWS RDS/Aurora automated backup window in UTC |
| `target.aws.catalog.databaseMaintenanceWindow` | string or null | no |  | AWS RDS/Aurora maintenance window in UTC |
| `target.aws.catalog.databaseDeletionProtection` | boolean or null | no |  | Enable AWS RDS/Aurora deletion protection |
| `target.aws.catalog.databaseDeleteAutomatedBackups` | boolean or null | no |  | Delete AWS RDS/Aurora automated backups when the database is destroyed |
| `target.aws.catalog.cacheSnapshotRetentionLimit` | integer or null | no |  | Days to retain ElastiCache Valkey snapshots. Set to 0 to disable automatic snapshots |
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
| `target.gcp.previewPool` | object or null | no |  | Existing cluster and network for preview environments |
| `target.gcp.previewPool.cluster` | string | yes |  | Existing regional GKE cluster name |
| `target.gcp.previewPool.network` | string | yes |  | Existing VPC self link or resource ID |
| `target.gcp.previewPool.subnet` | string | yes |  | Existing regional subnet self link or resource ID |
| `target.gcp.previewPool.networkPolicyEnforced` | boolean | yes |  | Confirm the shared cluster enforces Kubernetes NetworkPolicy |
| `target.gcp.previewPool.ingressCidrs` | array | yes |  | Source CIDRs reaching preview web pods, including load balancer and health checks |
| `target.gcp.project` | string | yes |  | GCP project ID |
| `target.gcp.region` | string or null | no |  | GCP region |
| `target.gcp.networkCidr` | string or null | no |  | VPC IPv4 CIDR |
| `target.gcp.zones` | array or null | no |  | GCP zones |
| `target.gcp.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `target.gcp.databaseName` | string or null | no |  | Magento database name |
| `target.gcp.masterUsername` | string or null | no |  | Cloud SQL master username |
| `target.gcp.encryptionKeySecret` | string or null | no |  | Secret Manager secret ID for Magento encryption key |
| `target.gcp.cloudSqlDatabaseVersion` | string | no |  | Cloud SQL MySQL API engine version |
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
| `target.gcp.memorystoreEngineVersion` | string or null | no |  | Memorystore for Valkey API engine version |
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
| `target.azure` | object or null | no |  | Azure deployment inputs |
| `target.azure.encryptionKeySecret` | string | no |  | Versioned Key Vault secret URL for the source Magento encryption key |
| `target.azure.encryptionKeySecretName` | string | no |  | Existing Kubernetes Secret containing the Magento encryption key |
| `target.azure.subscriptionId` | string | yes |  | Azure subscription ID |
| `target.azure.resourceGroup` | string | yes |  | Azure resource group name |
| `target.azure.tenantId` | string | no |  | Microsoft Entra tenant ID |
| `target.azure.imageDigest` | string | no |  | Immutable application image |
| `target.azure.stateBackendUrl` | string | no |  | Existing Azure Blob Pulumi state backend |
| `target.azure.imagePullSecret` | string | no |  | Existing Kubernetes secret for private registry access |
| `target.azure.acrResourceId` | string | no |  | Azure Container Registry resource ID for AKS pull access |
| `target.azure.ingressClass` | string | no |  | Existing Kubernetes ingress class |
| `target.azure.tlsSecret` | string | no |  | Existing Kubernetes TLS secret in the environment namespace |
| `target.azure.monitorWorkspaceId` | string | no |  | Existing Azure Log Analytics workspace resource ID |
| `target.azure.previewPool` | object or null | no |  | Existing AKS cluster and private network for preview environments |
| `target.azure.previewPool.clusterName` | string | yes |  | Existing AKS cluster name in the configured resource group |
| `target.azure.previewPool.nodeSubnetId` | string | yes |  | Existing AKS node subnet resource ID |
| `target.azure.previewPool.mysqlSubnetId` | string | yes |  | Existing delegated MySQL subnet resource ID |
| `target.azure.previewPool.mysqlDnsZoneId` | string | yes |  | Existing private DNS zone linked to the cluster network |
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
| `email.mode` | string or null | no | disabled, smtp, ses | Cloud email mode |
| `email.host` | string or null | no |  | SMTP host |
| `email.port` | integer or null | no |  | SMTP port |
| `email.username` | string or null | no |  | SMTP username |
| `email.from` | string or null | no |  | Default sender address |
| `email.credential` | string or null | no |  | SES credential secret reference |
| `email.managed` | object or null | no |  | Managed email provisioning inputs |
| `email.managed.domain` | string or null | no |  | Verified sender domain |
| `email.managed.hostedZoneId` | string or null | no |  | Route 53 hosted zone ID for DKIM records (SES only) |
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
| `environments` | object | yes |  | Named deployment environments |
| `environments.*.services` | object | no |  | Magento service families, versions, ownership and connection settings |
| `environments.*.services.*.parameters` | object | no |  | Database server defaults for charset, collation and packet size |
| `environments.*.services.*.adapter` | string | no |  | Explicitly linked service connection adapter ID |
| `environments.*.services.*.family` | string | yes |  | Service engine or implementation |
| `environments.*.services.*.version` | string | no |  | Selected engine version |
| `environments.*.services.*.ownership` | string or null | no | managed, self-hosted, existing, external | Provisioning ownership |
| `environments.*.services.*.endpoint` | string | no |  | Hostname or URL of an existing service |
| `environments.*.services.*.secret` | string | no |  | Provider secret reference for a connection |
| `environments.*.services.*.image` | string | no |  | Shop-selected service container image |
| `environments.*.services.*.port` | integer | no |  | Service connection port |
| `environments.*.services.*.username` | string | no |  | Service connection username |
| `environments.*.services.*.evictionPolicy` | string | no |  | Eviction policy for the cache or session store |
| `environments.*.services.*.plugins` | array | no |  | OpenSearch plugins required by the shop |
| `environments.*.services.*.memoryMiB` | integer | no |  | Redis or Valkey eviction threshold in MiB |
| `environments.*.services.*.storageGiB` | integer | no |  | Persistent volume size in GiB for a self-hosted service |
| `environments.*.inherits` | string | no |  | Parent environment |
| `environments.*.account` | string or null | no |  | AWS account ID |
| `environments.*.class` | string or null | no |  | Environment class |
| `environments.*.preset` | string or null | no | preview, standard, high-availability | Infrastructure preset |
| `environments.*.domain` | string or null | no |  | Environment domain |
| `environments.*.protection` | boolean or null | no |  | Protect against destructive commands |
| `environments.*.expiresAt` | string or null | no |  | Preview expiration as RFC3339 |
| `environments.*.previewTtl` | string or null | no |  | Preview lifetime from the triggering event, for example 168h |
| `environments.*.monthlyBudgetCents` | integer or null | no |  | Monthly budget planning input in cents (alerts only, never enforced) |
| `environments.*.branches` | array or null | no |  | Git branches mapped to this environment |
| `environments.*.seedDump` | string or null | no |  | Local SQL dump path for an explicit env import-dump operation |
| `environments.*.project` | object or null | no |  |  |
| `environments.*.project.name` | string | no |  | Project name |
| `environments.*.application` | object or null | no |  |  |
| `environments.*.application.edition` | string | no | open-source, commerce | Magento edition |
| `environments.*.application.version` | string | no |  | Exact Magento release |
| `environments.*.application.mode` | string | no | integrated, headless | Application mode |
| `environments.*.application.webRuntime` | string | no | nginx-fpm | HTTP runtime (nginx and PHP-FPM) |
| `environments.*.application.magento` | object | no |  | Magento runtime overlays |
| `environments.*.application.magento.frontName` | string | no |  | Magento admin frontName |
| `environments.*.application.magento.cookieDomain` | string | no |  | Magento cookie domain |
| `environments.*.application.magento.unsecureBaseUrl` | string | no |  | Unsecure Magento base URL |
| `environments.*.application.magento.secureBaseUrl` | string | no |  | Secure Magento base URL |
| `environments.*.application.magento.corsOrigins` | array | no |  | Allowed CORS origins for the Magento API |
| `environments.*.application.magento.storefrontOrigin` | string | no |  | Headless storefront origin allowed by CORS |
| `environments.*.application.magento.consumers` | object | no |  | Magento message consumer runners |
| `environments.*.application.magento.consumers.mode` | string or null | no | cron, processes, both, disabled | Consumer runner |
| `environments.*.application.magento.consumers.names` | array | no |  | Named Magento consumers |
| `environments.*.application.magento.queueTransport` | string or null | no | sqs, pubsub | Optional Magento-module queue transport |
| `environments.*.application.magento.queueModule` | string or null | no |  | Locked Composer Magento package providing SQS or Pub/Sub transport |
| `environments.*.application.magento.variables` | object | no |  | CONFIG__* and MAGENTO_DC_* overlays |
| `environments.*.application.cron` | array | no |  | Portable Magento cron schedule entries |
| `environments.*.build` | object or null | no |  |  |
| `environments.*.build.php` | string | no |  | Exact PHP branch or patch version |
| `environments.*.build.extensions` | array | no |  | Required PHP extensions |
| `environments.*.build.composer` | object | no |  | Composer settings |
| `environments.*.build.composer.image` | string or null | no |  | Digest-pinned Composer image |
| `environments.*.build.composer.version` | string or null | no |  | Required Composer 2 major, minor, or patch version |
| `environments.*.build.composer.credentials` | string | no |  | Composer credentials secret reference |
| `environments.*.build.staticContent` | object | no |  | Static content deployment settings |
| `environments.*.build.staticContent.matrix` | object | no |  | Locales to build for each Magento theme |
| `environments.*.build.staticContent.locales` | array | no |  | Locales passed to setup:static-content:deploy --language |
| `environments.*.build.staticContent.themes` | array | no |  | Themes passed to setup:static-content:deploy --theme |
| `environments.*.build.staticContent.strategy` | string or null | no | quick, standard, compact | Static content deploy strategy (-s) |
| `environments.*.build.staticContent.threads` | integer or null | no |  | Static content deploy thread count (-j) |
| `environments.*.build.qualityPatches` | array | no |  | Quality Patch IDs applied at build |
| `environments.*.build.cloudPatches` | boolean | no |  | Opt in to upstream Cloud Patches apply during build |
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
| `environments.*.build.cache` | object or null | no |  | Optional user-owned BuildKit registry cache |
| `environments.*.build.cache.from` | array | no |  | Registry references imported before the build |
| `environments.*.build.cache.to` | string or null | no |  | Registry reference receiving exported cache layers |
| `environments.*.build.cache.mode` | string or null | no | min, max | BuildKit cache export mode |
| `environments.*.build.cache.ignoreExportErrors` | boolean | no |  | Continue when cache export fails |
| `environments.*.target` | object or null | no |  |  |
| `environments.*.target.provider` | string | no | aws, gcp, azure | Infrastructure provider |
| `environments.*.target.runtime` | string | no | ecs-fargate, eks, gke-autopilot, gke-standard, aks | Application runtime |
| `environments.*.target.aws` | object or null | no |  | AWS deployment inputs |
| `environments.*.target.aws.previewPool` | object or null | no |  | Shared preview compute in an existing VPC |
| `environments.*.target.aws.previewPool.clusterArn` | string | no |  | ECS Fargate cluster owned outside the preview stack |
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
| `environments.*.target.aws.natTopology` | string or null | no | single-az, multi-az | NAT placement across AWS availability zones. Defaults to one zone for preview and multiple zones for standard or high availability |
| `environments.*.target.aws.natReplacementMode` | string or null | no | none, auto-scaling | How AWS replaces fck-nat instances. Defaults to automatic replacement for multi-AZ fck-nat |
| `environments.*.target.aws.natInstanceType` | string or null | no |  | ARM64 instance type for fck-nat. Defaults to t4g.nano to reduce cost |
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
| `environments.*.target.aws.catalog` | object | no |  | AWS runtime and service settings |
| `environments.*.target.aws.catalog.version` | string or null | no |  | AWS service settings version |
| `environments.*.target.aws.catalog.databaseEngine` | string or null | no | aurora-mysql, rds-mysql, rds-mariadb | Database engine shape |
| `environments.*.target.aws.catalog.searchMode` | string or null | no | serverless, provisioned, disabled | OpenSearch provisioning mode |
| `environments.*.target.aws.catalog.queueMode` | string or null | no | db, amazon-mq, ecs-rabbitmq, ecs-artemis | Magento messaging broker mode |
| `environments.*.target.aws.catalog.databaseBackupWindow` | string or null | no |  | AWS RDS/Aurora automated backup window in UTC |
| `environments.*.target.aws.catalog.databaseMaintenanceWindow` | string or null | no |  | AWS RDS/Aurora maintenance window in UTC |
| `environments.*.target.aws.catalog.databaseDeletionProtection` | boolean or null | no |  | Enable AWS RDS/Aurora deletion protection |
| `environments.*.target.aws.catalog.databaseDeleteAutomatedBackups` | boolean or null | no |  | Delete AWS RDS/Aurora automated backups when the database is destroyed |
| `environments.*.target.aws.catalog.cacheSnapshotRetentionLimit` | integer or null | no |  | Days to retain ElastiCache Valkey snapshots. Set to 0 to disable automatic snapshots |
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
| `environments.*.target.gcp.previewPool` | object or null | no |  | Existing cluster and network for preview environments |
| `environments.*.target.gcp.previewPool.cluster` | string | no |  | Existing regional GKE cluster name |
| `environments.*.target.gcp.previewPool.network` | string | no |  | Existing VPC self link or resource ID |
| `environments.*.target.gcp.previewPool.subnet` | string | no |  | Existing regional subnet self link or resource ID |
| `environments.*.target.gcp.previewPool.networkPolicyEnforced` | boolean | no |  | Confirm the shared cluster enforces Kubernetes NetworkPolicy |
| `environments.*.target.gcp.previewPool.ingressCidrs` | array | no |  | Source CIDRs reaching preview web pods, including load balancer and health checks |
| `environments.*.target.gcp.project` | string | no |  | GCP project ID |
| `environments.*.target.gcp.region` | string or null | no |  | GCP region |
| `environments.*.target.gcp.networkCidr` | string or null | no |  | VPC IPv4 CIDR |
| `environments.*.target.gcp.zones` | array or null | no |  | GCP zones |
| `environments.*.target.gcp.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `environments.*.target.gcp.databaseName` | string or null | no |  | Magento database name |
| `environments.*.target.gcp.masterUsername` | string or null | no |  | Cloud SQL master username |
| `environments.*.target.gcp.encryptionKeySecret` | string or null | no |  | Secret Manager secret ID for Magento encryption key |
| `environments.*.target.gcp.cloudSqlDatabaseVersion` | string | no |  | Cloud SQL MySQL API engine version |
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
| `environments.*.target.gcp.memorystoreEngineVersion` | string or null | no |  | Memorystore for Valkey API engine version |
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
| `environments.*.target.azure` | object or null | no |  | Azure deployment inputs |
| `environments.*.target.azure.encryptionKeySecret` | string | no |  | Versioned Key Vault secret URL for the source Magento encryption key |
| `environments.*.target.azure.encryptionKeySecretName` | string | no |  | Existing Kubernetes Secret containing the Magento encryption key |
| `environments.*.target.azure.subscriptionId` | string | no |  | Azure subscription ID |
| `environments.*.target.azure.resourceGroup` | string | no |  | Azure resource group name |
| `environments.*.target.azure.tenantId` | string | no |  | Microsoft Entra tenant ID |
| `environments.*.target.azure.imageDigest` | string | no |  | Immutable application image |
| `environments.*.target.azure.stateBackendUrl` | string | no |  | Existing Azure Blob Pulumi state backend |
| `environments.*.target.azure.imagePullSecret` | string | no |  | Existing Kubernetes secret for private registry access |
| `environments.*.target.azure.acrResourceId` | string | no |  | Azure Container Registry resource ID for AKS pull access |
| `environments.*.target.azure.ingressClass` | string | no |  | Existing Kubernetes ingress class |
| `environments.*.target.azure.tlsSecret` | string | no |  | Existing Kubernetes TLS secret in the environment namespace |
| `environments.*.target.azure.monitorWorkspaceId` | string | no |  | Existing Azure Log Analytics workspace resource ID |
| `environments.*.target.azure.previewPool` | object or null | no |  | Existing AKS cluster and private network for preview environments |
| `environments.*.target.azure.previewPool.clusterName` | string | no |  | Existing AKS cluster name in the configured resource group |
| `environments.*.target.azure.previewPool.nodeSubnetId` | string | no |  | Existing AKS node subnet resource ID |
| `environments.*.target.azure.previewPool.mysqlSubnetId` | string | no |  | Existing delegated MySQL subnet resource ID |
| `environments.*.target.azure.previewPool.mysqlDnsZoneId` | string | no |  | Existing private DNS zone linked to the cluster network |
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
| `environments.*.email.mode` | string or null | no | disabled, smtp, ses | Cloud email mode |
| `environments.*.email.host` | string or null | no |  | SMTP host |
| `environments.*.email.port` | integer or null | no |  | SMTP port |
| `environments.*.email.username` | string or null | no |  | SMTP username |
| `environments.*.email.from` | string or null | no |  | Default sender address |
| `environments.*.email.credential` | string or null | no |  | SES credential secret reference |
| `environments.*.email.managed` | object or null | no |  | Managed email provisioning inputs |
| `environments.*.email.managed.domain` | string or null | no |  | Verified sender domain |
| `environments.*.email.managed.hostedZoneId` | string or null | no |  | Route 53 hosted zone ID for DKIM records (SES only) |
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
| `environments.*.extensions` | object | no |  |  |
| `extensions` | object | no |  | Namespaced extension settings |
