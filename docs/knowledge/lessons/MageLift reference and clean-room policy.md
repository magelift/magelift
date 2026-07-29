---
type: lesson
title: MageLift reference and clean-room policy
description: Combine ece-tools phase separation/validation, Upsun CLI environment UX, the company stack
  operational lessons, and SST product polish/opinionated component patterns. Phase 4 importers and
  m2-hotfixes apply are clean-room only.
tags:
- magelift
- clean-room
- references
- ip
- paas-import
status: stable
generated:
  at: '2026-07-29'
---

Combine ece-tools phase separation/validation, Upsun CLI environment UX, the company stack operational lessons, and SST product polish/opinionated component patterns. Treat company repositories as behavioral evidence only: clean-room reimplementation, no copied source/docs/company identifiers/secrets unless future written authorization is documented per file. MagenX validates demand and supplies competitor lessons; Kuberaptor is a WIP future Hetzner/K3s reference, not a dependency. Avoid giant provider-leaky YAML, direct imperative cloud APIs without desired-state recovery, and premature lowest-common-denominator multi-cloud abstractions.

Phase 4 ACC/Upsun importers (`internal/paasimport`, `magelift init --from-acc` / `--from-upsun`) and the PHP `m2-hotfixes` `PatchApplier` are clean-room: public Adobe/Upsun documentation plus independently observed product behavior. Do not vendor `ece-tools`, `magento-cloud-patches`, Quality Patches Tool databases, or Adobe Commerce Cloud CLI source dumps into this repository. `QUALITY_PATCHES` stays an intentional gap in `docs/ece-parity.md`. CI/local gate: `make check-clean-room` → `scripts/check-clean-room.sh` (IMPORT-06 / `docs/provenance.md`).
