## ADDED Requirements

### Requirement: Audit gaps are named as support or certify limits

The architecture catalog MUST state: AWS standard/HA split Valkey cache and session; GCP/OVH/Scaleway share one cache endpoint; brownfield SQL attach is AWS RDS only; OVH native CDN is fail-closed; Scaleway managed cache is Redis not Valkey; SQS and Pub/Sub are Magento-module transports; unset AWS standard/HA queue defaults to `ecs-rabbitmq` after this change (Amazon MQ remains explicit experimental). A missing attach or SKU MUST be documented rather than implied by a nearby service.

#### Scenario: Operator asks for Cloud SQL attach

- **WHEN** GCP YAML sets an existing Cloud SQL instance identity
- **THEN** MageLift fails closed or typed-unsupported until that attach ships, and docs do not claim AWS RDS attach works on GCP
