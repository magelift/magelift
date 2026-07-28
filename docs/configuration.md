# Configuration reference

`magelift.yaml` uses schema version 1. Unknown fields are rejected.

When `target.aws` is present, the resolver applies the selected preset's bounded
topology defaults and the service versions from the Magento compatibility catalog.
Project and environment values override them. The defaults do not create secrets,
certificates, existing-resource references, or benchmark-selected instance sizes.
Use `config effective` to inspect the values and their provenance.

`target.provider: gcp` with `runtime: gke-autopilot` is an **experimental** first-party
target (ADR 0007 / 0008). It requires `target.gcp.project` and keeps GCP topology under
`target.gcp` only. Do not reuse `target.aws` fields for GCP.

`target.provider: ovh` / `runtime: mks` and `target.provider: scaleway` / `runtime: kapsule`
are also experimental. See [ovh-experimental.md](ovh-experimental.md) and
[scaleway-experimental.md](scaleway-experimental.md). Scaleway requires
`cacheMode: redis` (no managed Valkey yet).

Deployments require `target.aws.encryptionKeySecretArn` to reference a stable
Secrets Manager value. MageLift injects it at task start as the Magento encryption
key. The key must remain stable for the lifetime of encrypted Magento data.

`target.aws.existing.network` enables an existing VPC. When it is set, provide one
public, private, and data subnet ID for every configured availability zone. MageLift
does not create NAT gateways, route tables, or VPC endpoints in this mode, so the
imported network must already provide the required egress and private AWS service
access. The VPC CIDR is still required for security-group rules.

| Field | Type | Required | Accepted values | Description |
| --- | --- | --- | --- | --- |
| `schemaVersion` | integer | yes | 1 | MageLift configuration schema version |
| `project` | object | yes |  | Project identity |
| `project.name` | string | yes |  | Project name |
| `application` | object | yes |  | Magento application settings |
| `application.edition` | string | yes | open-source, commerce | Magento edition |
| `application.version` | string | yes |  | Exact Magento release |
| `application.mode` | string | yes | integrated, headless | Application mode |
| `application.webRuntime` | string | no | nginx-fpm, frankenphp-classic | HTTP application runtime |
| `build` | object | yes |  | Application build settings |
| `build.php` | string | yes |  | Exact PHP branch or patch version |
| `build.composer` | object | no |  | Composer settings |
| `build.composer.credentials` | string | no |  | Composer credentials secret reference |
| `build.staticContent` | object | no |  | Static content deployment settings |
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
| `target` | object | yes |  | Deployment target |
| `target.provider` | string | yes | aws, gcp, ovh, scaleway | Infrastructure provider |
| `target.runtime` | string | yes | ecs-fargate, eks-autopilot, gke-autopilot, mks, kapsule | Application runtime |
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
| `target.aws.existing` | object or null | no |  | Existing AWS resources |
| `target.aws.existing.network` | object or null | no |  | Existing VPC reference |
| `target.aws.existing.network.provider` | string | yes | aws | Resource provider |
| `target.aws.existing.network.kind` | string | yes |  | Resource kind |
| `target.aws.existing.network.externalId` | string | yes |  | Provider resource identifier |
| `target.aws.existing.publicSubnetIds` | array or null | no |  | Existing public subnet IDs |
| `target.aws.existing.privateSubnetIds` | array or null | no |  | Existing private subnet IDs |
| `target.aws.existing.dataSubnetIds` | array or null | no |  | Existing data subnet IDs |
| `target.aws.catalog` | object | no |  | Benchmark-selected service catalog |
| `target.aws.catalog.version` | string or null | no |  | Benchmark catalog version |
| `target.aws.catalog.databaseEngine` | string or null | no | aurora-mysql, rds-mysql | MySQL engine shape |
| `target.aws.catalog.searchMode` | string or null | no | serverless, provisioned, disabled | OpenSearch provisioning mode |
| `target.aws.catalog.queueMode` | string or null | no | db, amazon-mq, ecs-rabbitmq, ecs-artemis | Magento messaging broker mode |
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
| `target.aws.catalog.fargate.cpu` | integer | no |  | Fargate task CPU |
| `target.aws.catalog.fargate.memoryMiB` | integer | no |  | Fargate task memory |
| `target.aws.catalog.fargate.desiredCount` | integer | no |  | Fargate desired task count |
| `target.aws.catalog.eks` | object or null | no |  | EKS Autopilot capacity (eks-autopilot) |
| `target.aws.catalog.eks.cpuRequest` | string or null | no |  | Web/cron CPU request |
| `target.aws.catalog.eks.memoryRequest` | string or null | no |  | Web/cron memory request |
| `target.aws.catalog.eks.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `target.aws.catalog.eks.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `target.aws.catalog.retention` | object | no |  | Retention policy |
| `target.aws.catalog.retention.logDays` | integer | no |  | CloudWatch log retention days |
| `target.aws.catalog.retention.backupDays` | integer | no |  | Database backup retention days |
| `target.aws.catalog.retention.artifactDays` | integer | no |  | Artifact retention days |
| `target.aws.catalog.versions` | object | no |  | Managed service versions |
| `target.aws.catalog.versions.auroraMysql` | string | no |  | Aurora MySQL engine version |
| `target.aws.catalog.versions.mysql` | string | no |  | RDS MySQL engine version |
| `target.aws.catalog.versions.valkey` | string | no |  | Valkey engine version |
| `target.aws.catalog.versions.openSearch` | string | no |  | OpenSearch engine version |
| `target.aws.catalog.versions.rabbitMq` | string | no |  | RabbitMQ engine version |
| `target.gcp` | object or null | no |  | GCP deployment inputs (experimental) |
| `target.gcp.project` | string | yes |  | GCP project ID |
| `target.gcp.region` | string or null | no |  | GCP region |
| `target.gcp.networkCidr` | string or null | no |  | VPC IPv4 CIDR |
| `target.gcp.zones` | array or null | no |  | GCP zones |
| `target.gcp.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `target.gcp.databaseName` | string or null | no |  | Magento database name |
| `target.gcp.masterUsername` | string or null | no |  | Cloud SQL master username |
| `target.gcp.encryptionKeySecret` | string or null | no |  | Secret Manager secret ID for Magento encryption key |
| `target.gcp.cloudSqlTier` | string or null | no |  | Cloud SQL machine tier |
| `target.gcp.memorystoreNodeType` | string or null | no |  | Memorystore for Valkey node type |
| `target.gcp.autopilotCpuRequest` | string or null | no |  | GKE Autopilot CPU request |
| `target.gcp.autopilotMemoryRequest` | string or null | no |  | GKE Autopilot memory request |
| `target.gcp.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `target.gcp.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `target.gcp.labels` | object or null | no |  | Resource labels |
| `target.ovh` | object or null | no |  | OVHcloud deployment inputs (experimental) |
| `target.ovh.serviceName` | string | yes |  | OVH Public Cloud project service name |
| `target.ovh.region` | string or null | no |  | OVH Public Cloud region (e.g. GRA9) |
| `target.ovh.networkCidr` | string or null | no |  | Private network IPv4 CIDR |
| `target.ovh.zones` | array or null | no |  | OVH regions used as network availability zones |
| `target.ovh.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `target.ovh.databaseName` | string or null | no |  | Magento database name |
| `target.ovh.masterUsername` | string or null | no |  | Managed MySQL master username |
| `target.ovh.encryptionKeySecret` | string or null | no |  | Secret reference for Magento encryption key |
| `target.ovh.databaseFlavor` | string or null | no |  | OVH managed MySQL flavor |
| `target.ovh.databasePlan` | string or null | no |  | OVH managed MySQL plan |
| `target.ovh.valkeyFlavor` | string or null | no |  | OVH managed Valkey flavor |
| `target.ovh.valkeyPlan` | string or null | no |  | OVH managed Valkey plan |
| `target.ovh.nodeFlavor` | string or null | no |  | MKS node pool flavor |
| `target.ovh.nodeCount` | integer or null | no |  | MKS node pool size |
| `target.ovh.cpuRequest` | string or null | no |  | Kubernetes CPU request |
| `target.ovh.memoryRequest` | string or null | no |  | Kubernetes memory request |
| `target.ovh.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `target.ovh.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
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
| `target.scaleway.redisNodeType` | string or null | no |  | Scaleway Redis node type |
| `target.scaleway.cacheMode` | string or null | no | redis | Cache engine escape hatch |
| `target.scaleway.cpuRequest` | string or null | no |  | Kubernetes CPU request |
| `target.scaleway.memoryRequest` | string or null | no |  | Kubernetes memory request |
| `target.scaleway.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `target.scaleway.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `target.scaleway.kapsuleVersion` | string or null | no |  | Kapsule Kubernetes version |
| `target.scaleway.nodeType` | string or null | no |  | Kapsule pool node type |
| `target.scaleway.nodeCount` | integer or null | no |  | Kapsule pool size |
| `target.scaleway.labels` | object or null | no |  | Resource labels |
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
| `environments.*.monthlyBudgetCents` | integer or null | no |  | Maximum monthly AWS budget in cents |
| `environments.*.branches` | array or null | no |  | Git branches mapped to this environment |
| `environments.*.project` | object or null | no |  |  |
| `environments.*.project.name` | string | no |  | Project name |
| `environments.*.application` | object or null | no |  |  |
| `environments.*.application.edition` | string | no | open-source, commerce | Magento edition |
| `environments.*.application.version` | string | no |  | Exact Magento release |
| `environments.*.application.mode` | string | no | integrated, headless | Application mode |
| `environments.*.application.webRuntime` | string | no | nginx-fpm, frankenphp-classic | HTTP application runtime |
| `environments.*.build` | object or null | no |  |  |
| `environments.*.build.php` | string | no |  | Exact PHP branch or patch version |
| `environments.*.build.composer` | object | no |  | Composer settings |
| `environments.*.build.composer.credentials` | string | no |  | Composer credentials secret reference |
| `environments.*.build.staticContent` | object | no |  | Static content deployment settings |
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
| `environments.*.target.runtime` | string | no | ecs-fargate, eks-autopilot, gke-autopilot, mks, kapsule | Application runtime |
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
| `environments.*.target.aws.existing` | object or null | no |  | Existing AWS resources |
| `environments.*.target.aws.existing.network` | object or null | no |  | Existing VPC reference |
| `environments.*.target.aws.existing.network.provider` | string | no | aws | Resource provider |
| `environments.*.target.aws.existing.network.kind` | string | no |  | Resource kind |
| `environments.*.target.aws.existing.network.externalId` | string | no |  | Provider resource identifier |
| `environments.*.target.aws.existing.publicSubnetIds` | array or null | no |  | Existing public subnet IDs |
| `environments.*.target.aws.existing.privateSubnetIds` | array or null | no |  | Existing private subnet IDs |
| `environments.*.target.aws.existing.dataSubnetIds` | array or null | no |  | Existing data subnet IDs |
| `environments.*.target.aws.catalog` | object | no |  | Benchmark-selected service catalog |
| `environments.*.target.aws.catalog.version` | string or null | no |  | Benchmark catalog version |
| `environments.*.target.aws.catalog.databaseEngine` | string or null | no | aurora-mysql, rds-mysql | MySQL engine shape |
| `environments.*.target.aws.catalog.searchMode` | string or null | no | serverless, provisioned, disabled | OpenSearch provisioning mode |
| `environments.*.target.aws.catalog.queueMode` | string or null | no | db, amazon-mq, ecs-rabbitmq, ecs-artemis | Magento messaging broker mode |
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
| `environments.*.target.aws.catalog.fargate.cpu` | integer | no |  | Fargate task CPU |
| `environments.*.target.aws.catalog.fargate.memoryMiB` | integer | no |  | Fargate task memory |
| `environments.*.target.aws.catalog.fargate.desiredCount` | integer | no |  | Fargate desired task count |
| `environments.*.target.aws.catalog.eks` | object or null | no |  | EKS Autopilot capacity (eks-autopilot) |
| `environments.*.target.aws.catalog.eks.cpuRequest` | string or null | no |  | Web/cron CPU request |
| `environments.*.target.aws.catalog.eks.memoryRequest` | string or null | no |  | Web/cron memory request |
| `environments.*.target.aws.catalog.eks.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `environments.*.target.aws.catalog.eks.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `environments.*.target.aws.catalog.retention` | object | no |  | Retention policy |
| `environments.*.target.aws.catalog.retention.logDays` | integer | no |  | CloudWatch log retention days |
| `environments.*.target.aws.catalog.retention.backupDays` | integer | no |  | Database backup retention days |
| `environments.*.target.aws.catalog.retention.artifactDays` | integer | no |  | Artifact retention days |
| `environments.*.target.aws.catalog.versions` | object | no |  | Managed service versions |
| `environments.*.target.aws.catalog.versions.auroraMysql` | string | no |  | Aurora MySQL engine version |
| `environments.*.target.aws.catalog.versions.mysql` | string | no |  | RDS MySQL engine version |
| `environments.*.target.aws.catalog.versions.valkey` | string | no |  | Valkey engine version |
| `environments.*.target.aws.catalog.versions.openSearch` | string | no |  | OpenSearch engine version |
| `environments.*.target.aws.catalog.versions.rabbitMq` | string | no |  | RabbitMQ engine version |
| `environments.*.target.gcp` | object or null | no |  | GCP deployment inputs (experimental) |
| `environments.*.target.gcp.project` | string | no |  | GCP project ID |
| `environments.*.target.gcp.region` | string or null | no |  | GCP region |
| `environments.*.target.gcp.networkCidr` | string or null | no |  | VPC IPv4 CIDR |
| `environments.*.target.gcp.zones` | array or null | no |  | GCP zones |
| `environments.*.target.gcp.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `environments.*.target.gcp.databaseName` | string or null | no |  | Magento database name |
| `environments.*.target.gcp.masterUsername` | string or null | no |  | Cloud SQL master username |
| `environments.*.target.gcp.encryptionKeySecret` | string or null | no |  | Secret Manager secret ID for Magento encryption key |
| `environments.*.target.gcp.cloudSqlTier` | string or null | no |  | Cloud SQL machine tier |
| `environments.*.target.gcp.memorystoreNodeType` | string or null | no |  | Memorystore for Valkey node type |
| `environments.*.target.gcp.autopilotCpuRequest` | string or null | no |  | GKE Autopilot CPU request |
| `environments.*.target.gcp.autopilotMemoryRequest` | string or null | no |  | GKE Autopilot memory request |
| `environments.*.target.gcp.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `environments.*.target.gcp.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `environments.*.target.gcp.labels` | object or null | no |  | Resource labels |
| `environments.*.target.ovh` | object or null | no |  | OVHcloud deployment inputs (experimental) |
| `environments.*.target.ovh.serviceName` | string | no |  | OVH Public Cloud project service name |
| `environments.*.target.ovh.region` | string or null | no |  | OVH Public Cloud region (e.g. GRA9) |
| `environments.*.target.ovh.networkCidr` | string or null | no |  | Private network IPv4 CIDR |
| `environments.*.target.ovh.zones` | array or null | no |  | OVH regions used as network availability zones |
| `environments.*.target.ovh.imageDigest` | string or null | no |  | Signed immutable OCI image digest |
| `environments.*.target.ovh.databaseName` | string or null | no |  | Magento database name |
| `environments.*.target.ovh.masterUsername` | string or null | no |  | Managed MySQL master username |
| `environments.*.target.ovh.encryptionKeySecret` | string or null | no |  | Secret reference for Magento encryption key |
| `environments.*.target.ovh.databaseFlavor` | string or null | no |  | OVH managed MySQL flavor |
| `environments.*.target.ovh.databasePlan` | string or null | no |  | OVH managed MySQL plan |
| `environments.*.target.ovh.valkeyFlavor` | string or null | no |  | OVH managed Valkey flavor |
| `environments.*.target.ovh.valkeyPlan` | string or null | no |  | OVH managed Valkey plan |
| `environments.*.target.ovh.nodeFlavor` | string or null | no |  | MKS node pool flavor |
| `environments.*.target.ovh.nodeCount` | integer or null | no |  | MKS node pool size |
| `environments.*.target.ovh.cpuRequest` | string or null | no |  | Kubernetes CPU request |
| `environments.*.target.ovh.memoryRequest` | string or null | no |  | Kubernetes memory request |
| `environments.*.target.ovh.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `environments.*.target.ovh.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
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
| `environments.*.target.scaleway.redisNodeType` | string or null | no |  | Scaleway Redis node type |
| `environments.*.target.scaleway.cacheMode` | string or null | no | redis | Cache engine escape hatch |
| `environments.*.target.scaleway.cpuRequest` | string or null | no |  | Kubernetes CPU request |
| `environments.*.target.scaleway.memoryRequest` | string or null | no |  | Kubernetes memory request |
| `environments.*.target.scaleway.desiredWebReplicas` | integer or null | no |  | Desired web Deployment replicas |
| `environments.*.target.scaleway.queueConsumerCount` | integer or null | no |  | Queue consumer Deployment replicas |
| `environments.*.target.scaleway.kapsuleVersion` | string or null | no |  | Kapsule Kubernetes version |
| `environments.*.target.scaleway.nodeType` | string or null | no |  | Kapsule pool node type |
| `environments.*.target.scaleway.nodeCount` | integer or null | no |  | Kapsule pool size |
| `environments.*.target.scaleway.labels` | object or null | no |  | Resource labels |
| `environments.*.defaults` | object or null | no |  |  |
| `environments.*.defaults.region` | string | no |  | Default AWS region |
| `environments.*.defaults.preset` | string | no | preview, standard, high-availability | Default infrastructure preset |
| `environments.*.compatibility` | object or null | no |  |  |
| `environments.*.compatibility.allowUnsupported` | boolean | no |  | Allow an unsupported service combination |
| `environments.*.extensions` | object | no |  |  |
| `extensions` | object | no |  | Namespaced extension settings |

Environment values override project values. A `null` environment value removes an optional inherited value. Entries under `extensions` may use namespaced fields that MageLift does not inspect.
