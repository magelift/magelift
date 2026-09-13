## Purpose

Applies Magento Cloud Patches, Quality Patches, and `m2-hotfixes` deterministically using the project's installed upstream tools, failing closed on errors.

## Requirements

### Requirement: Use the project-installed upstream patch lifecycle

The build MUST inspect the target project's locked runtime packages before executing build commands. When the project provides the upstream Cloud Patches lifecycle, the build MUST invoke `php vendor/bin/ece-patches apply --no-interaction` after `composer install` and before Magento compilation. The build MUST NOT also emit individual fallback commands for the same `m2-hotfixes` files.

#### Scenario: Cloud lifecycle is locked

- **GIVEN** the target project's production Composer lock contains the upstream Cloud Patches lifecycle package
- **AND** the source contains required Cloud Patches, selected Quality Patches, or `m2-hotfixes`
- **WHEN** MageLift constructs the build commands
- **THEN** it emits Composer install followed by one upstream `ece-patches apply` command and then Magento compilation
- **AND** it does not apply the local patch files a second time

#### Scenario: No patch inputs are present

- **GIVEN** the target project has no Cloud Patches lifecycle package, no selected Quality Patch IDs, and no `m2-hotfixes` files
- **WHEN** MageLift constructs the build commands
- **THEN** it emits no patch command and the build remains a successful no-op for patching

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

### Requirement: Preserve deterministic and safe local patch behavior

When the upstream Cloud Patches lifecycle is unavailable, local patches MUST be discovered only from the project-root `m2-hotfixes` directory, MUST be regular non-symlink files whose paths remain inside that directory, and MUST be applied in lexicographic basename order. A patch that is already applied MUST be treated as an idempotent success; a patch that is neither applicable nor already applied MUST fail.

#### Scenario: Local patch names determine order

- **GIVEN** `m2-hotfixes/b-second.patch` and `m2-hotfixes/a-first.patch` are valid local patches
- **WHEN** MageLift builds the local fallback command list
- **THEN** it applies `a-first.patch` before `b-second.patch`

#### Scenario: Re-running an already-applied local patch

- **GIVEN** a local patch's resulting content is already present in the build workspace
- **WHEN** the fallback evaluates that patch
- **THEN** it reports success without applying the reverse patch or creating a duplicate change

#### Scenario: Unsafe local patch path

- **GIVEN** a local patch is a symlink or resolves outside `m2-hotfixes`
- **WHEN** MageLift discovers local patches
- **THEN** it fails before emitting a patch command

### Requirement: Fail closed on incompatible patch selections

Any non-zero result from the upstream Cloud Patches or Quality Patches command MUST fail the build. MageLift MUST NOT substitute an unavailable patch, ignore an unsupported ID, or continue to compilation after an upstream patch failure. The failure MUST retain the upstream diagnostic and identify the configured patch command or IDs.

#### Scenario: Patch ID is unavailable for the target Magento version

- **GIVEN** the upstream Quality Patches Tool rejects a configured ID for the locked Magento version
- **WHEN** the patch command exits non-zero
- **THEN** the build fails before compilation
- **AND** the surfaced diagnostic includes the upstream failure and the requested patch command/ID context

#### Scenario: A local patch cannot be applied

- **GIVEN** a local patch is neither applicable nor already applied
- **WHEN** the fallback evaluates it
- **THEN** the build fails with the patch filename and patch-tool diagnostic

### Requirement: Keep patch selection consistent across build environments

Local, CI, and cloud preparation MUST use the same patch selection and ordering rules from `magelift.yaml` and the production Composer lock. Environment-specific credentials or deployment settings MUST NOT change whether configured patches are selected, skipped, or duplicated. After import has mapped `.magento.env.yaml` into `magelift.yaml`, leftover `.magento.env.yaml` MUST NOT be a live input to patch selection.

#### Scenario: The same locked source is prepared in two environments

- **GIVEN** two preparations receive the same source revision, lock file, and `magelift.yaml` Quality Patch IDs
- **WHEN** they construct their build command sequences
- **THEN** the patch command sequence and ordering are identical
- **AND** a failure in either environment remains a build failure rather than a warning-only skip

### Requirement: Reuse upstream compatibility and version knowledge

MageLift MUST NOT vendor a Quality Patches database or reimplement Adobe's patch compatibility rules. The locked upstream tools MUST remain authoritative for required Cloud Patches, supported Quality Patch IDs, Magento version compatibility, and patch rollback behavior.

#### Scenario: Upstream package version changes

- **GIVEN** the target project updates its locked upstream patch package to a supported version
- **WHEN** MageLift prepares the project
- **THEN** MageLift invokes the package's installed executable without requiring a MageLift patch-database update
- **AND** any incompatibility reported by that executable fails the build with its diagnostic
