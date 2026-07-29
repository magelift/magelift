# ece-tools parity matrix

Honesty surface for MageLift vs Adobe Commerce Cloud / ece-tools operator expectations.
Full matrix (no blank rows) is owned by plan 04-06. This stub records patch-related
ECE-02 claims as soon as the PHP build gate lands.

## Patches

| Capability | Status | Notes |
| --- | --- | --- |
| Custom `m2-hotfixes/*.patch` apply (alpha order, after `composer install`) | **Closed** | Clean-room `MageLift\Build\Magento\PatchApplier` via host `patch -p1`. See `build/src/Magento/PatchApplier.php`. |
| `QUALITY_PATCHES` / cloud-required Quality Patches Tool IDs | **Intentional gap** | No Adobe quality-patch database is vendored (IMPORT-06 / provenance). Selecting QPT IDs is not claimed as implemented. |

Remaining hook/env rows land in 04-06.
