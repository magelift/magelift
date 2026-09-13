## Purpose

Specifies the Magento-aware build and deploy lifecycle analogous in purpose to `ece-tools`, using upstream Magento CLI and project-installed patch tools rather than a vendored Adobe clone.

## Requirements

### Requirement: Build produces an immutable Magento artifact

`magelift build` MUST run a Magento-aware prepare path that covers Composer authentication by secret reference, Composer install, the patch lifecycle, typed build hooks, generated code, static-content deploy when a locale and theme matrix is configured, and dependency-injection compilation. Composer credentials MUST NOT enter the image, the artifact manifest, or logs. The isolated builder's PHP branch, Composer version, and extensions MUST match the requested contract before the application image is created. The resulting artifact MUST be identified by digest. MageLift MUST NOT reimplement Magento CLI commands that upstream already provides.

#### Scenario: A catalog-backed build succeeds

- **WHEN** a project with a supported Magento release, PHP, Composer, and extension set runs `magelift build`
- **THEN** prepare completes the documented phases, the image is bound to a digest, and Composer credentials are absent from the image and manifest

#### Scenario: Builder PHP does not match the request

- **WHEN** the isolated builder reports a PHP branch, Composer version, or extension set that does not satisfy the build contract
- **THEN** the build fails before image creation and names the mismatched input without printing credentials

### Requirement: Typed hooks extend the DAG without shell strings

Build configuration MAY add typed hooks whose executable is Composer or `bin/magento`. Free-form shell hook strings MUST be rejected. Importers MUST report PaaS shell hooks as unmapped. Deploy-time typed hooks MAY exist as a later provider-extension contract; until that contract exists, YAML MUST NOT accept deploy shell scripts.

#### Scenario: A shell hook is rejected

- **WHEN** configuration supplies a hook command as a shell string
- **THEN** validation fails before build or deploy and names the typed-hook contract

### Requirement: Deploy upgrades Magento then validates

Deploy MUST apply environment configuration, run `app:config:import` and `setup:upgrade --keep-generated` (or the documented Magento equivalents for that release), clean or flush cache as specified, and run post-deploy validation including runtime health. Maintenance mode MUST be used when the selected strategy requires it, and MUST be cleared on success. Deploy MUST fail closed if Magento CLI or health validation fails. Application rollback MUST deploy a previously signed digest as a new release; it MUST NOT claim to reverse irreversible schema or data migrations.

#### Scenario: Schema upgrade fails

- **WHEN** Magento `setup:upgrade` exits non-zero during deploy
- **THEN** the command fails, leaves the previous healthy release serving if a candidate cutover has not completed, and does not report success

#### Scenario: Rollback after a data migration

- **WHEN** an operator requests rollback to a previous digest after a release that ran irreversible schema or data migrations
- **THEN** MageLift deploys that digest only if the documented data-compatibility check passes, or it refuses and names the required restore path

### Requirement: Local, CI, and cloud builds apply the same Magento contract

Patch selection, Composer install flags, PHP extensions, and SCD inputs MUST be identical for local builds, CI builds, and cloud builds of the same configuration digest. A success in one environment MUST NOT depend on a silently skipped patch or hook in another.

#### Scenario: CI omits a required patch tool

- **WHEN** CI runs a build whose configuration selects Quality Patches and the project-installed patch tool is missing
- **THEN** the build fails closed with the same class of error as a local build

### Requirement: Integrated PHP storefront is a certified path

When `application.mode` is `integrated`, build and deploy MUST apply the locale×theme static-content matrix for storefront themes, Magento base URLs and cookies on the shop hostname, static and media, and Varnish FPC when the catalog selects Varnish. Hyvä or other Node-built Magento theme compile MUST remain out of band until a typed Node hook exists. Free-form shell hooks MUST stay rejected. JavaScript storefronts MUST stay outside the CLI.

#### Scenario: Storefront themes are deployed

- **WHEN** an integrated project lists storefront themes and locales in `build.staticContent`
- **THEN** prepare runs `setup:static-content:deploy` for that matrix and the cloud runtime serves those static files through nginx-fpm

#### Scenario: Hyvä Node compile is not a Magelift hook

- **WHEN** a project needs a Node or Vite compile for a Magento theme
- **THEN** Magelift does not accept a shell hook for that compile and docs name it as an out-of-band step
