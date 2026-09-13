## Purpose

Defines Magento HTTP frontends as independently versioned plugins, parallel to cloud-provider extensions, so nginx, FrankenPHP classic, Apache, and later community servers can ship on their own cadence without claiming Adobe support they do not have.

## ADDED Requirements

### Requirement: Web runtimes are versioned extensions

Every Magento HTTP frontend MUST be a registered web-runtime extension with its own identity, semantic version, SDK API version, source, digest when loaded out-of-process, Adobe-support declaration, MageLift certification tier, Magento release range, local Compose contract, and cloud image contract. `application.webRuntime` MUST be that extension ID. The empty value MUST resolve to the first-party `nginx-fpm` plugin. An unregistered ID MUST fail closed before image pull or mutate and MUST name the plugin to install. Web-runtime extensions MUST NOT be required to declare stack `CoreOutputKeys`. Unsigned files in the working directory MUST NOT execute.

#### Scenario: Default YAML uses nginx

- **WHEN** a project omits `application.webRuntime`
- **THEN** planning uses the first-party `nginx-fpm` plugin and records Adobe-supported nginx on the Magento compatibility row

#### Scenario: Unknown runtime ID is refused

- **WHEN** YAML sets `application.webRuntime` to an ID that is not registered
- **THEN** validation fails before build or provision and names the missing web-runtime plugin

### Requirement: Adobe row and MageLift plugin status are separate

Each plugin MUST declare whether the selected Magento release lists that HTTP server on Adobe's current system-requirements table. Adobe-unsupported combinations, including FrankenPHP classic and Apache on 2.4.8-p3+ / 2.4.9, MUST fail closed unless `compatibility.allowUnsupported` records the Adobe hatch. Docs MUST state that Adobe lists nginx only on that train and that FrankenPHP has no Adobe row. Hatch warnings MUST name Adobe-unsupported and MageLift-community and MUST NOT call the cell Adobe-supported or MageLift-certified. Provider-unavailable images MUST still fail closed.

#### Scenario: FrankenPHP on 2.4.9 needs the Adobe hatch

- **WHEN** Magento 2.4.9 YAML sets `application.webRuntime: frankenphp-classic` without `compatibility.allowUnsupported`
- **THEN** validation fails closed because Adobe's current 2.4.9 table lists nginx and does not list FrankenPHP

#### Scenario: FrankenPHP hatch is explicit

- **WHEN** Magento 2.4.9 YAML sets `application.webRuntime: frankenphp-classic` and `compatibility.allowUnsupported: true` and the FrankenPHP classic plugin is registered
- **THEN** planning proceeds with warnings that name both Adobe-unsupported and MageLift-community, and the matrix does not mark FrankenPHP certified

#### Scenario: Apache on 2.4.9 needs the Adobe hatch

- **WHEN** Magento 2.4.9 YAML sets `application.webRuntime: php-apache` without `compatibility.allowUnsupported`
- **THEN** validation fails closed because Adobe's current 2.4.9 table lists nginx and does not list Apache

#### Scenario: Apache hatch is explicit

- **WHEN** Magento 2.4.9 YAML sets `application.webRuntime: php-apache` and `compatibility.allowUnsupported: true` and the Apache plugin is registered
- **THEN** planning proceeds with warnings that name both Adobe-unsupported and MageLift-community, and the matrix does not mark Apache certified

### Requirement: First-party plugins version independently of the CLI

The default binary MAY compile in first-party web-runtime plugins (`nginx-fpm`, `frankenphp-classic`, `php-apache`). Each plugin MUST still expose independent version and capability metadata. A plugin bugfix MUST be allowed without a core CLI version bump when the web-runtime SDK API version matches. Community web-runtime plugins MUST load through the same signed HashiCorp go-plugin path as community cloud providers. FrankenPHP worker mode MUST remain unregistered until a Magento storefront contract exists.

#### Scenario: Extensions list web runtimes

- **WHEN** a user runs the documented extensions list command
- **THEN** each web-runtime plugin reports ID, version, Adobe-support flag, MageLift tier, and Magento releases it claims

#### Scenario: Worker mode stays out

- **WHEN** YAML sets `application.webRuntime: frankenphp-worker`
- **THEN** validation fails because no plugin registers that ID

### Requirement: Local and cloud use the same plugin contract

The selected plugin MUST supply the local Compose web service and the cloud HTTP process or sidecar used for Magento. Switching plugin ID MUST NOT silently substitute nginx under another name. Health MUST use the plugin's documented Magento health path.

#### Scenario: Local Compose follows the plugin

- **WHEN** `magelift local init` runs with `application.webRuntime: frankenphp-classic` and `compatibility.allowUnsupported: true`
- **THEN** generated Compose uses the FrankenPHP classic image contract and does not start nginx under that name
