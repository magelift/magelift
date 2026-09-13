# Tasks: Magento patch lifecycle

## 1. Patch input and tool selection

- [x] 1.1 Add strict parsing for `.magento.env.yaml` `stage.build.QUALITY_PATCHES`, including ordered IDs, duplicate rejection, and actionable malformed-input errors.
- [x] 1.2 Read production Composer package metadata and select Cloud lifecycle, standalone Quality Patches, or custom fallback behavior.
- [x] 1.3 Fail closed before Composer when Quality Patch IDs are configured without a locked applying package.

## 2. Lifecycle execution

- [x] 2.1 Delegate Cloud-enabled projects to `php vendor/bin/ece-patches apply --no-interaction` after Composer install.
- [x] 2.2 Invoke standalone `php vendor/bin/magento-patches apply <IDs...>` before deterministic local fallback patches.
- [x] 2.3 Add an argv-safe process-aware custom patch command that recognizes already-applied patches and never applies a local patch twice.
- [x] 2.4 Preserve path, symlink, host-tool, and non-zero failure handling for custom patches.

## 3. Verification and documentation

- [x] 3.1 Add focused unit tests for parser, package selection, command order, duplicate suppression, idempotency, and fail-closed errors.
- [x] 3.2 Update ECE parity, provenance, and implementation notes to describe upstream delegation and the clean-room boundary.
- [x] 3.3 Run focused PHP tests, static analysis, clean-room checks, and OpenSpec validation; record any remaining external certification boundary.
