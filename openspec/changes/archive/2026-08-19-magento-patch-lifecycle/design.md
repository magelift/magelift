# Design: Magento patch lifecycle

## Context

The build plan is assembled before the isolated Composer install, while the target project's `vendor/` directory is deliberately excluded from the copied source. The plan can therefore use immutable source metadata (`composer.json`, `composer.lock`, `.magento.env.yaml`, and `m2-hotfixes`) to select the command that will run after Composer installs the locked dependencies.

The upstream Cloud Patches executable owns the complete Adobe order: required Cloud Patches, selected Quality Patches, then local `m2-hotfixes`. A project that has only the standalone Quality Patches package cannot apply required Cloud Patches, so MageLift invokes that package for configured IDs and retains the local fallback afterward.

## Decisions

### 1. Select from production Composer metadata before execution

Read package names from `composer.lock`'s production `packages` list, with `composer.json`'s `require` section as the declaration fallback when no lock is present. `require-dev` and `packages-dev` do not select patch tooling because the build runs `composer install --no-dev`.

Selection is:

1. Cloud lifecycle package present: run `php vendor/bin/ece-patches apply --no-interaction`.
2. Otherwise, Quality Patches package plus a non-empty `QUALITY_PATCHES` list: run `php vendor/bin/magento-patches apply <IDs...>`, then local fallback commands.
3. Otherwise, local `m2-hotfixes` only: run the clean-room idempotent fallback.
4. Quality Patch IDs without a package: fail before lifecycle execution.

The upstream Cloud command is the only command emitted for local patches in case 1. This prevents the common double-application error where ECE-Tools applies a hotfix and MageLift then invokes `patch(1)` against the already-modified tree.

### 2. Parse only the supported Quality Patches configuration surface

Use a small strict parser for the documented `stage.build.QUALITY_PATCHES` list, including block lists and simple inline lists. It is intentionally not a general YAML parser. Unsupported YAML constructs, duplicate IDs, empty IDs, tabs in indentation, and malformed list values fail with a message that points to `.magento.env.yaml`.

No environment variable is used to alter the selected patch list during preparation; the source configuration and lock are the build inputs. This keeps local, CI, and cloud command sequences reproducible.

### 3. Make the host fallback idempotent with a process-aware command

The lifecycle executor currently sends argv-only commands to the process runner. Add the smallest command execution hook needed by the local patch command:

- run `patch --dry-run` in forward mode;
- if that succeeds, run the real forward patch;
- if it fails, run a reverse dry-run;
- if the reverse dry-run succeeds, return an idempotent success without changing files;
- otherwise return a failure containing both diagnostics.

The command remains argv-based and uses `bypass_shell`; no shell interpolation or untrusted path concatenation is introduced. The existing discovery checks remain the trust boundary.

### 4. Preserve upstream failure semantics

Upstream commands are not wrapped in a warning-only path and are not retried by the build step. A non-zero result reaches the existing lifecycle failure reporter, which includes the command output. MageLift does not inspect or replicate the upstream patch database, version matrix, or required patch pool.

## Testing strategy

- Unit-test Quality Patches parsing and package selection with production lock and manifest fixtures.
- Assert exact command vectors and ordering for Cloud lifecycle, standalone Quality Patches, and custom-only projects.
- Test missing-package and malformed-config failures before the runner receives a Composer request.
- Run a real synthetic unified-diff patch twice to verify idempotent fallback behavior.
- Keep the existing path traversal, symlink, corrupt-patch, and lifecycle failure tests.
- Run the focused PHP suite, static analysis, clean-room check, and OpenSpec validation.

## Alternatives rejected

- Vendoring ECE-Tools, Cloud Patches, or a Quality Patches database: violates the repository clean-room boundary and would stale independently of Adobe's supported package versions.
- Reimplementing required patch pools or version compatibility: duplicates upstream behavior and risks applying the wrong patch to a Magento release.
- Shelling out through `sh -c`: makes safe path handling and error attribution harder when the existing process runner already supports argv-only execution.
- Always invoking both upstream and local commands: applies `m2-hotfixes` twice in the normal Cloud Patches case.
