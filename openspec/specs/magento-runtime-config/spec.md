# Magento Runtime Config Specification

## Purpose

Gives Magento PHP teams a YAML control plane for Magento runtime configuration they used to keep in env.php, ACC variables, and PaaS overlays, without putting secret values in the file and without vendoring Adobe tools.

## Requirements

### Requirement: Magento runtime overlays are first-class YAML

`magelift.yaml` MUST accept Magento runtime overlays that map to `env.php` structure and to Magento `CONFIG__*` / injected `MAGENTO_DC_*` keys. Values that are secrets MUST be secret references. The effective configuration MUST retain provenance for every overlay. MageLift MUST NOT require operators to SSH and edit `env.php` on the happy path.

#### Scenario: Cookie domain is set in YAML

- **WHEN** an environment overlay sets the Magento cookie domain
- **THEN** deploy writes that value through the Magento runtime contract, does not print secret material, and `config` effective output names the overlay as the source

#### Scenario: Plaintext secret in an overlay is refused

- **WHEN** an overlay would store a database password or crypt key as a literal string
- **THEN** validation fails before mutate and names the secret-reference contract

### Requirement: Consumers and cron are explicit

YAML MUST declare whether Magento message consumers run from `cron:run`, from dedicated consumer processes, or both, including named consumers when the project does not use Magento defaults. SQS or Pub/Sub MUST NOT silently replace Magento db or AMQP consumers.

#### Scenario: Named consumers are listed

- **WHEN** the project lists Magento consumers in YAML
- **THEN** deploy applies that runner configuration and `queue-status` observes those consumers rather than an undocumented default set

### Requirement: Store, website, and store-view URLs are YAML

Base URLs, cookie scope, and admin `frontName` MUST be configurable per environment and, when present, per website or store view. Admin WAF exclusions MUST use the configured `frontName`, not a baked `/admin` path.

#### Scenario: Custom admin frontName

- **WHEN** YAML sets a custom Magento admin frontName
- **THEN** Magento-safe WAF and docs use that path, and a request to `/admin` is not assumed to be the admin panel

### Requirement: Headless CORS and dual hostnames are YAML

When `application.mode` is `headless`, YAML MUST accept the Magento/API hostname, optional unused storefront hostname, and CORS origins. MageLift MUST NOT deploy a JavaScript storefront.

#### Scenario: Dual hostname headless

- **WHEN** headless YAML sets an API hostname and a distinct storefront origin for CORS
- **THEN** Magento GraphQL and REST accept that origin and MageLift does not provision a Next.js or PWA Studio app

### Requirement: Quality Patch IDs have one control plane

After ACC or Upsun import, Quality Patch IDs MUST live only in `magelift.yaml`. Build MUST NOT read a second live control plane from `.magento.env.yaml` once those IDs are present in Magelift YAML. Import MUST map `.magento.env.yaml` `QUALITY_PATCHES` into `magelift.yaml` and report unmapped IDs.

#### Scenario: Import maps patch IDs

- **WHEN** the user imports a project whose `.magento.env.yaml` lists Quality Patch IDs
- **THEN** `magelift.yaml` contains those IDs in order and later builds read only `magelift.yaml`

#### Scenario: Dual sources disagree

- **WHEN** `magelift.yaml` lists patch IDs and `.magento.env.yaml` still lists a different set
- **THEN** build uses `magelift.yaml` only and does not merge the two lists
