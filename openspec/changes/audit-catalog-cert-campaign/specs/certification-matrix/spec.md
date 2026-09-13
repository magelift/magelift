## ADDED Requirements

### Requirement: Certification is never inferred from a cheap smoke

A cell MUST be marked certified only when `docs/capability-matrix.md` and the current `docs/evidence/` file record live Magento (or declared infra-only) evidence for that exact provider, runtime, Magento release, and service tuple. Preview health on a substitute service MUST NOT certify a denser SKU. Experimental targets MAY gain evidence files without a certified badge.

#### Scenario: Amazon MQ smoke does not certify

- **WHEN** a preview stack uses `queueMode: db` successfully
- **THEN** the matrix does not mark `amazon-mq` certified

### Requirement: Certification tier follows the evidence tuple

`CertificationTier` MUST be computed from provider, runtime, compute mode, Magento release, and catalog services recorded in `docs/evidence/`. A module MUST NOT return certified for every ECS or every Autopilot plan. Preview Fargate Magento MUST NOT certify Managed Instances, EKS, Aurora, Amazon MQ, HA, or provisioned OpenSearch.

#### Scenario: Managed Instances do not inherit Fargate certified

- **WHEN** YAML selects ECS Managed Instances and no Managed Instances Magento evidence file exists
- **THEN** the plan is experimental (or typed unavailable) and the CLI does not omit the experimental warning

### Requirement: AWS remaining credits fund a packed matrix, not Cartesian shops

AWS live cells in this campaign MUST use KEEP, one Magento artifact digest, cold compute families (Fargate, Managed Instances, EKS), and warm catalog transitions (`ecs-rabbitmq`, `amazon-mq`, EKS RabbitMQ, OpenSearch, `aurora-mysql`, HA, CloudWatch, X-Ray after the plugin exists). OVH and Scaleway MUST stay one preview each. Product YAML that omits standard/HA `queueMode` MUST still resolve to `ecs-rabbitmq` or `db`, not Amazon MQ.

#### Scenario: Standard YAML does not spawn Amazon MQ by default

- **WHEN** an operator applies AWS standard-shaped Magento without an explicit `queueMode`
- **THEN** the broker is `ecs-rabbitmq` or `db`, not Amazon MQ

#### Scenario: Campaign Amazon MQ is explicit

- **WHEN** the AWS KEEP profile sets `queueMode: amazon-mq`
- **THEN** evidence records that Amazon MQ cell and does not mark omitted-default YAML as Amazon MQ
