## Purpose

Defines the Adobe Commerce release and service combinations that MageLift may validate, reject, or describe as compatibility-only behavior.

## ADDED Requirements

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
