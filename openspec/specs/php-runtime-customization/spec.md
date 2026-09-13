## Purpose

Specifies PHP versions, `php.ini`, extensions, and Magento-oriented presets, and keeps those contracts aligned across local containers, builders, and cloud runtimes.

## Requirements

### Requirement: PHP version and extensions come from the catalog

Supported PHP versions MUST be the versions listed for the selected Magento release in the compatibility catalog. `build.php` and `build.extensions` MUST be validated before image build or cloud mutation. Unknown extension names MUST fail validation. Display names such as `Zend OPcache` MUST normalize to a stable identifier. The same extension set MUST be required of the isolated builder, the local app image, and the cloud runtime image.

#### Scenario: An unsupported PHP is rejected

- **WHEN** a 2.4.9 project requests a PHP version the catalog does not list for that release
- **THEN** validation fails before build or provision and names the supported PHP versions

#### Scenario: Cloud runtime missing an extension

- **WHEN** the selected runtime image does not load a requested extension
- **THEN** deploy or build fails before serving traffic and names the missing extension

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
