# Proposal: Complete the Magento patch lifecycle

## Why

MageLift currently applies project `m2-hotfixes/*.patch` files with a clean-room host `patch` fallback, but its parity matrix still marks Adobe Commerce Quality Patches and required Cloud Patches as an intentional gap. That makes a build behave differently depending on whether it runs through MageLift, ECE-Tools, or an Adobe Commerce Cloud build, and it can cause configured security or compatibility patches to be silently absent.

The supported behavior is already defined by the project-installed Adobe tooling: required Cloud Patches, configured Quality Patches, and local `m2-hotfixes` in deterministic order. MageLift should invoke that tooling when the target project locks it, while retaining a narrowly scoped custom fallback for projects that intentionally have no Adobe patch package.

## What Changes

- Detect the patch packages and `QUALITY_PATCHES` configuration from the copied project inputs before constructing the build plan.
- Delegate the complete patch lifecycle to the project-installed `ece-patches` executable when Cloud Patches tooling is locked.
- Support standalone Quality Patches Tool IDs from `.magento.env.yaml` when only the Quality Patches package is locked, then apply custom hotfixes in deterministic order.
- Fail before build execution when Quality Patches are configured without the package that can apply them, and fail the build when the upstream tool rejects an unavailable or incompatible patch.
- Make the clean-room custom fallback recognize an already-applied patch without applying it twice, while retaining path, symlink, and failure validation.
- Record the completed parity and provenance contract without vendoring Adobe patch databases or upstream source.

## Capabilities

### New Capabilities

- `magento-patch-lifecycle`: deterministic, upstream-compatible patch selection and application for local, CI, and cloud builds.

### Modified Capabilities

- None.

## Impact

- Build planning and execution in `build/src/Magento/` and `build/src/Runner/`.
- PHP unit tests covering package detection, Quality Patches configuration, command order, idempotency, and fail-closed behavior.
- ECE parity and provenance documentation.
- No new runtime dependency is vendored into MageLift; the target project remains responsible for locking Adobe’s supported patch packages.
