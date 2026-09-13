## Purpose

Defines the AWS implemented Magento architecture catalog, the certified subset, and KEEP ownership so agencies can compose field architectures without treating every YAML combination as live-certified.

## ADDED Requirements

### Requirement: AWS catalog lists every implemented architecture family

`docs/capability-matrix.md` plus this spec MUST list every YAML-selectable AWS architecture MageLift can plan or apply: runtime (`ecs-fargate`, `eks`), Fargate compute mode (`fargate`, `fargate-spot`, `ec2-asg`, `managed-instances`), EKS compute mode (`auto-mode`, `managed-node-groups`, `self-managed`, `fargate`), database (`rds-mysql`, `aurora-mysql`, `rds-mariadb`), search (`disabled`, `serverless`, `provisioned`; EKS `opensearch`/`disabled`), queue (`db`, `ecs-rabbitmq`, `amazon-mq`, `ecs-artemis`; EKS `database`/`rabbitmq`), NAT (`nat-gateway`/`fck-nat` with topology), HA/resilience, web runtime, native edge, and observability. Each cell MUST carry Adobe, MageLift-implemented, and certified-or-experimental-or-unavailable status. An omitted combination MUST be typed unavailable or fail closed, not silently missing.

#### Scenario: An agency selects Managed Instances plus Aurora plus provisioned OpenSearch

- **WHEN** YAML sets `runtime: ecs-fargate`, `fargate.computeMode: managed-instances`, `databaseEngine: aurora-mysql`, and `searchMode: provisioned`
- **THEN** the plan accepts or fail-closes with a typed reason, records each dimension, and does not inherit Fargate-preview certified status

### Requirement: Certified AWS architectures are a named subset

Only cells with `docs/capability-matrix.md` plus `docs/evidence/` for that exact provider, runtime, Magento release, and catalog tuple MAY be certified. The certified AWS subset today is ECS Fargate preview Magento (`nginx-fpm`) with the recorded catalog services. Managed Instances, EKS, Aurora, Amazon MQ, provisioned OpenSearch, AOSS Magento data-plane, HA multi-AZ, FrankenPHP, Apache, and X-Ray MUST remain experimental or typed unavailable until that tuple has evidence.

#### Scenario: Fargate preview does not certify EKS

- **WHEN** an operator selects `runtime: eks` after a Fargate Magento KEEP pass
- **THEN** the CLI warns experimental (or typed unavailable) and the matrix does not mark EKS certified

### Requirement: Magento-on-AWS wiring is a MageLift contract, not a private shop

AWS Magento env for RDS and Aurora MUST inject the writer endpoint and Secrets Manager username/password JSON. `searchMode: provisioned` MUST use an in-VPC OpenSearch domain Magento can query (HTTPS 443, no SigV4 sidecar unless Magento signs). `searchMode: serverless` MUST use AOSS plus the SigV4 sidecar. Private sibling Magento shops MAY illustrate one architecture; they MUST NOT be a certification target, the only supported shape, or a substitute for `docs/capability-matrix.md` plus `docs/evidence/`. This spec MUST NOT duplicate the portable Magento overlay contract.

#### Scenario: Provisioned search does not attach a SigV4 sidecar

- **WHEN** the stack applies `searchMode: provisioned`
- **THEN** Magento reaches the domain on HTTPS 443 without a SigV4 sidecar, and `searchMode: serverless` still uses the sidecar

#### Scenario: A private AWS shop does not certify MageLift

- **WHEN** an operator points at a private Magento-on-AWS implementation as proof
- **THEN** the matrix ignores that shop and requires MageLift evidence for the exact cell tuple

### Requirement: AWS KEEP certifies packed main cells, not Cartesian shops

AWS live certification MUST use one KEEP session, one Magento artifact digest, cold baselines when compute family, database engine major, or managed-versus-self-hosted search/queue boundary changes, and warm transitions for compatible catalog service changes. Cartesian live shops per combination are forbidden. Packed-campaign task 6.3 is owned by this capability until archived.

#### Scenario: Queue modes share a Fargate baseline

- **WHEN** a Fargate KEEP baseline exists and YAML changes only `queueMode` among `db`, `ecs-rabbitmq`, and `amazon-mq`
- **THEN** the runner records warm cells on that baseline and does not provision three Magento stacks
