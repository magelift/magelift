## Purpose

Gives a Magento project one explicit, inspectable contract for the PHP runtime,
required extensions, Composer version, edition, release line, and credentials
needed to build and certify the application.

## Requirements

### Requirement: Runtime requirements are project inputs

MageLift MUST accept and preserve the project PHP version, required PHP
extensions, Composer version, Magento release line, edition, and Composer
credential reference as typed inputs. Omitted optional values MUST resolve from
the selected release and preset, and the resolved values MUST be visible in
plan and evidence output.

#### Scenario: Project selects PHP and Composer explicitly

- **WHEN** a project declares PHP 8.5, a list of extensions, and Composer 2.10
- **THEN** the build request and certification identity contain those exact
  resolved values

### Requirement: Requirements are validated before mutation

MageLift MUST validate the requested runtime contract against the Adobe release
catalog, the selected artifact, and the target provider capability before
preview or apply registers or mutates infrastructure. A missing extension,
invalid Composer version, unavailable provider service, or missing credential
reference MUST produce an actionable error.

#### Scenario: Required extension is absent from the artifact contract

- **WHEN** the project requires `intl` and the selected builder cannot provide
  it
- **THEN** validation fails before infrastructure mutation and names `intl`

#### Scenario: Composer credentials are missing for Adobe Commerce

- **WHEN** the edition is Adobe Commerce and no supported secret reference is
  configured
- **THEN** the plan is rejected and no Adobe Commerce certification record is
  generated

### Requirement: Runtime requirements are verified in the isolated build

The build MUST prove the effective PHP version, required extension set, and
Composer version inside the isolated builder or runtime preparation step. A
successful host-side parse MUST NOT substitute for the isolated check.

#### Scenario: Host PHP differs from the builder

- **WHEN** the host has PHP 8.3 but the project requires PHP 8.5
- **THEN** the isolated builder checks PHP 8.5 and the host version is not used
  as certification evidence

### Requirement: PaaS imports preserve runtime intent

ACC and Upsun imports MUST map supported PHP extension and Composer declarations
into the portable runtime contract. Unsupported free-form hooks or provider
settings MUST remain visible as unmapped migration data and MUST NOT be silently
executed.

#### Scenario: ACC declares an extension and Composer dependency

- **WHEN** an ACC project contains runtime extension and Composer declarations
- **THEN** the imported MageLift project contains the equivalent typed fields
  and records the source metadata

### Requirement: Evidence records the effective contract

Every live certification record MUST include the resolved PHP version,
extension set, Composer version, edition, release line, credential scheme, and
immutable artifact digest without including secret values.

#### Scenario: Certification evidence is reviewed after a pass

- **WHEN** a live cell is marked PASS
- **THEN** a reviewer can identify the exact runtime contract and artifact
  without reading a secret or relying on an unpinned tag
