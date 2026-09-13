## MODIFIED Requirements

### Requirement: Apply Quality Patches from the standard build configuration

The build MUST read Quality Patches IDs from `magelift.yaml`. IDs MUST be validated as non-empty patch identifiers and MUST preserve the configured order. Duplicate IDs or malformed configuration MUST fail before Composer or patch execution. ACC and Upsun import MUST map `.magento.env.yaml` `stage.build.QUALITY_PATCHES` into `magelift.yaml`. After that mapping exists, build MUST NOT read `.magento.env.yaml` as a second live control plane.

#### Scenario: Standalone Quality Patches Tool is locked

- **GIVEN** `magelift.yaml` contains an ordered Quality Patch ID list
- **AND** the production Composer lock contains the Quality Patches package but not the Cloud Patches lifecycle package
- **WHEN** the build commands execute after Composer install
- **THEN** MageLift invokes `php vendor/bin/magento-patches apply` with the configured IDs in order
- **AND** it applies any validated `m2-hotfixes` files only after the Quality Patches command succeeds

#### Scenario: Quality Patch configuration has no applying package

- **GIVEN** `magelift.yaml` contains one or more Quality Patch IDs
- **AND** neither supported upstream patch package is locked for production
- **WHEN** MageLift prepares the build
- **THEN** it fails with an actionable error naming the missing package/tool requirement
- **AND** it does not proceed to Composer install or custom patch application

### Requirement: Keep patch selection consistent across build environments

Local, CI, and cloud preparation MUST use the same patch selection and ordering rules from `magelift.yaml` and the production Composer lock. Environment-specific credentials or deployment settings MUST NOT change whether configured patches are selected, skipped, or duplicated. After import has mapped `.magento.env.yaml` into `magelift.yaml`, leftover `.magento.env.yaml` MUST NOT be a live input to patch selection.

#### Scenario: The same locked source is prepared in two environments

- **GIVEN** two preparations receive the same source revision, lock file, and `magelift.yaml` Quality Patch IDs
- **WHEN** they construct their build command sequences
- **THEN** the patch command sequence and ordering are identical
- **AND** a failure in either environment remains a build failure rather than a warning-only skip
