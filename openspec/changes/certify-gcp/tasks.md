## 1. Tracker split

- [x] 1.1 Add `certification-gcp` to `openspec/README.md`. Point `audit-catalog-cert-campaign` task 6.2 at this change. Verify `openspec validate --change certify-gcp --strict`.
- [x] 1.2 State in `docs/capability-matrix.md` that GCP cells live here and Autopilot evidenced runtime is the certified subset. Verify Standard and HA remain experimental.

## 2. Implemented GCP catalog

- [x] 2.1 Replace “preset-derived / no AWS-style toggles” wording with an explicit `target.gcp` dimension table (runtime, Cloud SQL availability/backup, Memorystore, `openSearchMode`, `queueMode`, Standard nodes, Armor). Verify every relevant `GCPTarget` field appears or is typed unavailable/withheld.
- [x] 2.2 Keep presets as omitted-field defaults only. Verify `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/config/ -count=1` still resolves preview/standard/HA defaults.
- [x] 2.3 Name remaining gaps without inheriting certified: Cloud SQL attach, Pub/Sub Magento module, Armor Magento body exclusion, Autopilot HA OpenSearch `vm.max_map_count`. Verify FAQ/matrix match.

## 3. KEEP ownership

- [x] 3.1 Record `gcap29` as KEEP evidence, not a certified-row replacement, until destroy + orphan assert. Verify the evidence file states KEEP.
- [x] 3.2 After apply, finish Autopilot Magento 2.4.8-p5 / 2.4.9 KEEP under this tracker when a signed 2.4.8-p5 digest exists; destroy on close. Verify evidence tuple identity. Do not wait on AWS KEEP to record GCP status. 2.4.9 `gcap29` catalog 13/13 PASS then destroy emptied the prefix (2026-08-23). 2.4.8-p5 is `not-run`: no signed digest in Artifact Registry.

## 4. Docs lockstep

- [x] 4.1 Update `docs/gcp-acceptance.md` and evidence README links to `certification-gcp`. Humanizer then remove-ai-marks. Verify `make docs` if those pages are in the MkDocs nav.
