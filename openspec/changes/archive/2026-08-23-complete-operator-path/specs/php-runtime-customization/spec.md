## MODIFIED Requirements

### Requirement: php.ini customization is explicit and Magento-oriented

MageLift MUST support Magento-optimized PHP presets and operator `php.ini` overrides. Local development MUST write a generated ini file mounted into the app container. Cloud runtimes MUST apply the equivalent settings to the selected PHP runtime. Invalid or unknown ini directives that the runtime would ignore silently MUST fail validation when MageLift can detect them. Settings that change request body or admin upload behavior MUST stay consistent with the Magento-safe WAF body-inspection limit.

#### Scenario: Local ini is generated

- **WHEN** a user runs `magelift local init` with verified PHP settings
- **THEN** MageLift writes a generated ini file under the local state directory, mounts it into the app container, and does not require hand-edited container images

### Requirement: nginx-fpm is the primary web runtime

`application.webRuntime` MUST be `nginx-fpm` only. FrankenPHP and Apache MUST NOT appear in the schema, recipes, local Compose, or cloud catalogs. There is no public release: omitted values are invalid YAML, not a migrate command.

#### Scenario: FrankenPHP image is unavailable

- **WHEN** YAML sets `application.webRuntime` to `frankenphp-classic`
- **THEN** configuration validation fails before any image pull and names `nginx-fpm` as the allowed value; MageLift does not start nginx-fpm under the FrankenPHP name

#### Scenario: nginx-fpm is accepted

- **WHEN** a project omits `application.webRuntime` or sets `nginx-fpm`
- **THEN** local and cloud planning use nginx with PHP-FPM

#### Scenario: FrankenPHP is not a valid value

- **WHEN** YAML sets `application.webRuntime` to `frankenphp-classic`, `frankenphp-worker`, or `php-apache`
- **THEN** configuration validation fails before build or provision and names `nginx-fpm` as the allowed value
