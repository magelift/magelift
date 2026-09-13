## MODIFIED Requirements

### Requirement: nginx-fpm is the default Adobe-aligned web runtime

`application.webRuntime` MUST default to the first-party `nginx-fpm` plugin. Additional HTTP frontends MUST load as web-runtime plugins (`web-runtime-plugins`). `frankenphp-classic` and `php-apache` MUST fail closed on 2.4.8-p3+ / 2.4.9 without `compatibility.allowUnsupported`. Omitted values MUST mean nginx-fpm, not an implicit migrate command. FrankenPHP worker MUST remain unregistered.

#### Scenario: nginx-fpm is accepted

- **WHEN** a project omits `application.webRuntime` or sets `nginx-fpm`
- **THEN** local and cloud planning use nginx with PHP-FPM

#### Scenario: FrankenPHP classic without hatch is refused

- **WHEN** Magento 2.4.9 YAML sets `application.webRuntime` to `frankenphp-classic` without `compatibility.allowUnsupported`
- **THEN** validation fails closed and names the Adobe hatch; MageLift does not start nginx under the FrankenPHP name

#### Scenario: FrankenPHP classic with hatch warns

- **WHEN** Magento 2.4.9 YAML sets `application.webRuntime` to `frankenphp-classic` and `compatibility.allowUnsupported: true` and the plugin is registered
- **THEN** planning proceeds with Adobe-unsupported and MageLift-community warnings and does not mark the cell certified

#### Scenario: Worker mode is not a valid plugin

- **WHEN** YAML sets `application.webRuntime` to `frankenphp-worker`
- **THEN** configuration validation fails before build or provision

#### Scenario: Apache without hatch is refused

- **WHEN** Magento 2.4.9 YAML sets `application.webRuntime` to `php-apache` without `compatibility.allowUnsupported`
- **THEN** validation fails closed and names the Adobe hatch
