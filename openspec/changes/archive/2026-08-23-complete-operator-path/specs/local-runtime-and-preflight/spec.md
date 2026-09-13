## MODIFIED Requirements

### Requirement: Local runtime inputs are catalog-backed

Docker Compose and the isolated Magento build MUST resolve PHP, Composer,
database, broker, cache, search, email, PHP settings, and PHP extension inputs
from the same compatibility catalog used by cloud planning. A requested
combination outside the catalog MUST fail before image build or volume mutation
and MUST name the nearest supported alternative or the intentional gap.

#### Scenario: A supported Magento service combination is selected

- **WHEN** a project selects a catalog row for a supported Magento release and
  its PHP, Composer, database, broker, cache, search, and email services
- **THEN** local Compose and the build protocol use the pinned row inputs and
  expose the selected values in a deterministic plan

#### Scenario: A provider-only service is selected for local development

- **WHEN** a project selects a managed cloud service with no local equivalent
- **THEN** planning reports the explicit local substitute or intentional gap and
  does not silently replace the production service family

#### Scenario: Local PHP inputs are rendered into the app runtime

- **WHEN** a project selects a verified PHP branch and declares supported PHP
  extensions or PHP ini settings
- **THEN** planning selects the matching local app image, rejects extensions
  outside its verified baseline, and `local init` writes the sorted PHP settings
  to a generated file mounted into the app container

#### Scenario: The isolated builder returns a mismatched runtime contract

- **WHEN** the builder reports a PHP branch, Composer version, or extension set
  that does not satisfy the requested build inputs
- **THEN** the pipeline fails before BuildKit creates the application image and
  identifies the mismatched runtime input without exposing credentials

#### Scenario: Builder extension display names normalize to stable IDs

- **WHEN** the isolated builder reports `Zend OPcache` for a request containing
  the portable `opcache` extension name
- **THEN** the pipeline accepts the alias and records the normalized
  `zend_opcache` identifier in the runtime contract, while equivalent duplicate
  names are rejected before BuildKit

### Requirement: Local commands use the existing `dev` tree

Local Docker workflows MUST be exposed as `magelift local init`, `local up`, `local down`, `local status`, and `local logs` (plus `local seed` and `local exec`). MageLift MUST NOT expose `magelift dev`. Local environments MUST approximate the selected cloud environment: same Magento build contract, nginx-fpm, and catalog service families and majors, with named substitutes when a managed cloud product has no container twin. Local HTTPS MAY use a development CA on a documented loopback port. CloudFront, Cloud Armor, and Fastly MUST NOT be faked locally; doctor MUST say they are cloud-only.

#### Scenario: Operator starts local Magento

- **WHEN** a user runs `magelift local init` then `magelift local up` with a catalog-backed configuration
- **THEN** Compose starts the selected services, `local status` reports them, and `local logs` tails without requiring cloud credentials

#### Scenario: Local HTTPS on loopback

- **WHEN** the local web runtime publishes HTTPS
- **THEN** it listens on the documented loopback port with a development certificate and does not require a public DNS name

### Requirement: Mailpit stays an explicit gap until verified

`local.email.mode: mailpit` MUST use the verified Mailpit image, health check, and Magento SMTP wiring when that contract exists. SMTP, SendGrid, SES, disabled, and Mailpit remain the documented local modes. An unverified Mailpit pin MUST fail before Compose mutation.

#### Scenario: Mailpit selected today

- **WHEN** local email mode is `mailpit` and the pinned Mailpit image contract is missing
- **THEN** the command exits non-zero, names the missing image contract, and does not write Compose services for Mailpit

#### Scenario: Mailpit selected with a verified image

- **WHEN** local email mode is `mailpit` and the pinned Mailpit image contract exists
- **THEN** Compose starts Mailpit, Magento SMTP points at it, and no cloud email credentials are required

## ADDED Requirements

### Requirement: Local substitutes are named, never silent

When the selected cloud environment uses Aurora, Amazon MQ, OpenSearch Serverless, or Memorystore, local planning MUST record the substitute family and major (MySQL same major, RabbitMQ, OpenSearch, Valkey) in provenance. Durable cloud presets that isolate cache and session MUST isolate cache and session locally.

#### Scenario: Aurora environment runs locally on MySQL

- **WHEN** the selected environment catalog is Aurora MySQL 8.4 and the operator runs `magelift local up`
- **THEN** Compose uses MySQL 8.4, effective configuration records the Aurora→MySQL substitute, and Magento still uses nginx-fpm
