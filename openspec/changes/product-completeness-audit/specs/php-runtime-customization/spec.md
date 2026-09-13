## Purpose

Specifies PHP versions, `php.ini`, extensions, and Magento-oriented presets, and keeps those contracts aligned across local containers, builders, and cloud runtimes.

## ADDED Requirements

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

- **WHEN** a user runs `magelift dev init` with verified PHP settings
- **THEN** MageLift writes a generated ini file under the local state directory, mounts it into the app container, and does not require hand-edited container images

### Requirement: nginx-fpm is the primary web runtime

`application.webRuntime` MUST support `nginx-fpm` as the primary Magento path. `frankenphp-classic` and `frankenphp-worker` MAY be selected where a certified or experimental cell exists. FrankenPHP worker mode MUST remain P3 and MUST NOT block nginx-fpm implementation or certification. A missing FrankenPHP image MUST fail closed rather than silently falling back to nginx-fpm.

#### Scenario: FrankenPHP image is unavailable

- **WHEN** a project selects `frankenphp-classic` and the matching image digest cannot be pulled
- **THEN** local or cloud planning fails and names the missing image; it does not start nginx-fpm under the FrankenPHP name

#### Scenario: FrankenPHP worker is selectable without changing nginx-fpm

- **WHEN** a project selects `application.webRuntime: frankenphp-worker`
- **THEN** the cloud runtime starts FrankenPHP with `--worker` and nginx-fpm remains the default for projects that omit worker
