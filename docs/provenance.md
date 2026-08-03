# Clean-room provenance ledger

This ledger records material design inputs. It is not a dependency license inventory;
generated distributions must maintain that separately.

| Area | Origin | Public source or method | Allowed use | Status |
| --- | --- | --- | --- | --- |
| Configuration and deploy lifecycle | New MageLift design informed by public product behavior | Public ece-tools documentation and source under its own license | Concepts and independently implemented interfaces; no copied identifiers or text | Recorded |
| ACC / Upsun → `magelift.yaml` importers | Clean-room Phase 4 (`internal/paasimport`) | Public Adobe / Upsun config file docs + fixture shapes; no vendored PaaS trees | Generate reviewable YAML via `magelift init --from-acc` / `--from-upsun`; foreign schemas rejected by `schemaVersion` probe | Recorded |
| m2-hotfixes patch apply | Clean-room PHP build (`PatchApplier`) | Public Adobe hotfix / `patch -p1` behavior; no `magento-cloud-patches` / QPT database | Apply project `m2-hotfixes/*.patch` after composer install; `QUALITY_PATCHES` remains intentional gap | Recorded |
| Environment-centric CLI experience | Clean-room behavioral observation | Public Upsun CLI documentation and normal public CLI use | User-experience lessons only | Recorded |
| Infrastructure orchestration | New MageLift design | Pulumi Automation API and state backend documentation | Public APIs and documented behavior | Recorded |
| AWS topology | New MageLift design | AWS service documentation and Adobe system requirements/remote-storage guidance | Documented capabilities and compatibility constraints | Recorded |
| FrankenPHP classic adapter | New MageLift integration | FrankenPHP public Docker and configuration documentation | Runtime interoperability and documented image behavior; no source copied | Recorded |
| Local Compose developer workflow | New MageLift design | Docker Compose behavior and MageLift capability contracts | Generated local execution context; no external source copied | Recorded |
| Account-free AWS checks | New MageLift test integration | Floci public emulator documentation and independently observed SDK behavior | Contract tests against a pinned emulator image; no Floci source copied | Recorded |
| Product/CLI quality bar | General product inspiration | Public SST documentation and releases | Product principles only; no source or branding copied | Recorded |
| Turnkey/cost/Go CLI lessons | General public inspiration | Public MagenX and Kuberaptor materials | Independently implemented concepts only | Recorded |
| Existing company systems | Clean-room behavioral reference only | Authorized observation by contributors | Lessons and requirements; never source, documentation, identifiers, assets, secrets, or customer data | Restricted |

## Contribution procedure

Before adding code or documentation derived from an external input:

1. Confirm the contributor has authority to use the input.
2. Record the source, license, and whether use is API interoperability, public factual
   documentation, clean-room observation, or original work.
3. Prefer independently named and implemented contracts.
4. Preserve required licenses and notices for intentionally bundled material.
5. Stop and request maintainer/legal review if provenance or trademark use is unclear.

Name, GitHub organization, Packagist, GHCR, and documentation identifiers require
formal clearance before the repository is made public or the first tag is created.
