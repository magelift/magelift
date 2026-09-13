## Purpose

Defines the Adobe Commerce release and service combinations that MageLift may validate, reject, or describe as compatibility-only behavior.

## Requirements

### Requirement: Current release-line catalog

The compatibility catalog MUST contain one authoritative record for Adobe Commerce 2.4.6-p15, 2.4.7-p10, 2.4.8-p5, and 2.4.9, including the source revision and retrieval date for the Adobe system requirements.

#### Scenario: Catalog exposes the current patch lines

- **WHEN** a contributor lists the compatibility catalog
- **THEN** the four release-line records and their source metadata are present

### Requirement: Explicit compatibility status

Every database, search, cache, queue, web-cache, PHP, Composer, and edge combination in the catalog MUST have one of these statuses: `adobe-supported`, `magelift-compatible`, `unsupported`, or `unavailable`.

#### Scenario: Unsupported service is visible

- **WHEN** a release line does not list a requested service version
- **THEN** validation reports that status instead of silently treating the service as supported

### Requirement: Validate before infrastructure mutation

MageLift MUST reject an `adobe-supported` certification cell that selects a version or service combination outside the catalog before Pulumi registers or mutates resources.

#### Scenario: Latest 2.4.6 rejects unsupported MySQL and Redis

- **WHEN** a 2.4.6-p15 cell selects MySQL or Redis
- **THEN** configuration validation fails before preview or apply and names the unsupported choice

#### Scenario: 2.4.8 accepts listed search engines

- **WHEN** a 2.4.8-p5 cell selects Elasticsearch 8 or OpenSearch 3
- **THEN** the cell passes compatibility validation when the selected provider can supply that service

#### Scenario: 2.4.9 rejects Elasticsearch

- **WHEN** a 2.4.9 cell selects Elasticsearch
- **THEN** validation fails because the current Adobe table lists OpenSearch 3 instead

### Requirement: Provider capability intersection

The effective cell MUST be the intersection of Adobe compatibility, MageLift target capability, and provider product availability. A service that Adobe supports but a target cannot provide MUST be reported as unavailable on that target.

#### Scenario: Provider lacks a managed service

- **WHEN** a target has no implementation for a required search or queue service
- **THEN** the cell is marked unavailable and cannot be reported as certified

### Requirement: Edition is part of cell identity

Certification records MUST include the Magento Open Source or Adobe Commerce edition used to run the cell. An Adobe Commerce claim MUST require a pullable, licensed artifact and composer credentials.

#### Scenario: Only Open Source credentials are available

- **WHEN** a live run has no Adobe Commerce artifact credentials
- **THEN** Open Source results may be recorded, but no Adobe Commerce certification row is generated

### Requirement: Catalog names default stacks and an expansion rule

The compatibility catalog MUST name a default recommended stack per supported Magento release line (PHP, Composer, web server, cache, database, search, queue). It MUST list unsupported combinations instead of implying that every permutation works. Adding a new Magento patch line, PHP version, or service major MUST require a source date from Adobe system requirements plus a verification path (local image contract, cloud adapter, or explicit unavailable). Historical certified cells MUST NOT be treated as current compatibility for a newer row.

#### Scenario: Default stack for 2.4.9

- **WHEN** a user omits optional service versions on Adobe Commerce or Magento Open Source 2.4.9
- **THEN** effective configuration resolves the documented 2.4.9 default stack and records provenance for each service

#### Scenario: New service major needs a dated row

- **WHEN** Adobe lists a new OpenSearch or Valkey major that MageLift has not verified
- **THEN** validation reports the row as unsupported or unavailable until a source-dated catalog entry and verification path exist

### Requirement: Runtime families are enumerated even when not all cells exist

The catalog MUST have a place for PHP, Composer, nginx, Varnish, Redis, Valkey, OpenSearch, MySQL, MariaDB, RabbitMQ, and ActiveMQ Artemis, and for managed provider substitutes (Aurora, ElastiCache, OpenSearch Service, Amazon MQ, Cloud SQL, Memorystore, and documented Scaleway or OVHcloud equivalents). A family without a verified cell MUST be `unsupported` or `unavailable`, not omitted.

#### Scenario: MariaDB has no verified local image

- **WHEN** a project selects MariaDB and the local catalog has no pinned image and health contract
- **THEN** local planning fails before Compose is written and names the nearest verified database family

### Requirement: Adobe-unsupported combinations fail closed unless allowUnsupported

A combination Adobe system requirements mark unsupported MUST fail configuration validation before preview, local Compose mutation, or cloud mutation. `compatibility.allowUnsupported` MAY proceed past that Adobe gate only. It MUST NOT recertify the cell, MUST NOT hide the Adobe-unsupported status in provenance, and MUST NOT apply to MageLift-experimental or provider-unavailable cells. Validation errors MUST name which authority rejected the combination: Adobe, MageLift, or the selected provider.

#### Scenario: Adobe-unsupported MySQL fails closed

- **WHEN** a 2.4.6-p15 project selects MySQL and `compatibility.allowUnsupported` is unset
- **THEN** validation exits before plan or mutate, names Adobe as the rejecting authority, and names the supported database family

#### Scenario: allowUnsupported is an Adobe hatch only

- **WHEN** the same Adobe-unsupported combination is selected with `compatibility.allowUnsupported: true`
- **THEN** planning may proceed with Adobe-unsupported recorded in provenance, and the cell MUST NOT be reported as `adobe-supported` or certified

### Requirement: MageLift-experimental cells warn and do not block

Selecting a MageLift-experimental or otherwise uncertified-but-implemented cell MUST print a warning that names MageLift as the authority, MUST record experimental in provenance, and MUST NOT fail validation or block plan or apply. The warning MUST NOT be silent. The cell MUST NOT be reported as certified. `compatibility.allowUnsupported` MUST NOT be required and MUST NOT recertify the cell. Provider-unavailable cells (no adapter or no SKU) MUST still fail closed.

#### Scenario: Experimental MageLift cells warn and proceed

- **WHEN** YAML selects a MageLift-experimental cell such as ECS Managed Instances
- **THEN** `config validate`, plan, and mutate succeed, a warning names MageLift and experimental, provenance records experimental, and the cell is not claimed certified

#### Scenario: Unavailable stays blocking

- **WHEN** YAML selects a provider-unavailable cell with no adapter or no SKU
- **THEN** validation fails before mutate, names the provider as the rejecting authority, and does not treat the cell as experimental-warn

### Requirement: nginx is the only catalog web family

Catalog rows for Magento 2.4.x web runtimes MUST list nginx only. Apache and FrankenPHP MUST be absent from the catalog, schema, and recipes.

#### Scenario: nginx is the only web family

- **WHEN** a catalog row is listed for Magento 2.4.x web runtimes
- **THEN** the Adobe-aligned web family is nginx, and Apache and FrankenPHP are absent from the row

### Requirement: SQS and Pub/Sub are never Adobe-supported Magento brokers

Magento core message queue remains database or AMQP. SQS and Pub/Sub MUST NOT appear as `adobe-supported` catalog brokers. They MAY be provisioned only as an optional Magento-module transport when a named Composer Magento package is locked. Without that package, planning MUST fail closed. Core Magento consumers MUST stay on db or AMQP.

#### Scenario: SQS without a Magento module is refused

- **WHEN** YAML requests SQS or Pub/Sub and the project lock does not contain the declared Magento module
- **THEN** validation fails before provision and names the module contract
