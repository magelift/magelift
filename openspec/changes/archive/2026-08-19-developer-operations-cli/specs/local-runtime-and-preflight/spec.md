## ADDED Requirements

### Requirement: Dependency checks are capability-specific and actionable

The CLI MUST check external executables before the command that needs them
creates local volumes, temporary credentials, or provider sessions. Each result
MUST identify the executable, capability, required or optional state, detected
version when available, and an install hint for the current platform. A cloud
command MUST NOT require Docker when Docker is not part of its execution path.

#### Scenario: A required dependency is missing

- **WHEN** an operator runs a command whose required executable is unavailable
- **THEN** the command fails before mutation and reports the missing executable,
  the capability it serves, and a verified package-manager or official install
  path

#### Scenario: An optional dependency is missing

- **WHEN** an operator runs `doctor` for a target whose optional capability is
  unavailable locally
- **THEN** the report marks that capability unavailable, keeps unrelated checks
  usable, and explains which command would require the dependency

### Requirement: Dependency installation is explicit and constrained

The CLI MUST NOT silently install software or execute arbitrary shell strings.
Any opt-in install flow MUST require confirmation, use an allowlisted package
manager and package identifier, preserve argv boundaries, and report the
resulting version or failure.

#### Scenario: A user confirms a Homebrew or Scoop installation

- **WHEN** the detected platform has an allowlisted package manager and the
  operator explicitly confirms installation
- **THEN** the CLI invokes only the matching package-manager argv and reruns the
  dependency probe before continuing

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
  outside its verified baseline, and `dev init` writes the sorted PHP settings
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

### Requirement: Compatibility rows are source-dated and version-specific

The compatibility catalog MUST keep the exact Magento release or release line,
PHP and Composer constraints, service family, service version, local image
reference, cloud mapping, and verification status together. A newer service
version MUST NOT be inferred from a previous row or from a provider default.
The source snapshot MUST be refreshable from the current Adobe system
requirements page, and historical certification evidence MUST NOT be treated
as current compatibility evidence.

#### Scenario: The source catalog changes a service major

- **WHEN** the current Adobe requirements list a different service major than
  a historical MageLift row
- **THEN** the source catalog and generated local plan use the current row,
  while historical evidence remains labeled with its original service major
  and cannot certify the new row

#### Scenario: A service family has no verified local image contract

- **WHEN** a project selects MariaDB, MySQL, Redis, Valkey, OpenSearch,
  Elasticsearch, RabbitMQ, Artemis, Varnish, nginx, or a provider-only
  substitute for which the catalog lacks a pinned image and health contract
- **THEN** local planning fails before image build or volume creation and names
  the nearest verified family or the missing verification work
