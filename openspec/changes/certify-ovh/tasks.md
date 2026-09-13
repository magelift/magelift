## 1. Tracker split

- [x] 1.1 Add `certification-ovh` to `openspec/README.md`. Point `audit-catalog-cert-campaign` task 6.4 OVH at this change. Verify `openspec validate --change certify-ovh --strict`.
- [x] 1.2 State in `docs/capability-matrix.md` that OVH MKS is experimental, certified subset empty, Bootstrap/Secrets `ErrNotSupported`. Verify no page calls OVH certified.

## 2. Implemented OVH catalog

- [x] 2.1 Enumerate MKS plan, MySQL plan/version/nodes/backup, Valkey plan/version, floating IPs, private-network routing, and Logs Data Platform `nativeReference` with Adobe / MageLift / experimental / unavailable columns. Verify every relevant `OVHTarget` field appears or is typed unavailable.
- [x] 2.2 Keep private-network cells split (gateway vs floating IP vs CNI). Verify existing OVH network tests still document that split.

## 3. Live bar

- [x] 3.1 After apply, run at most one MKS Magento preview when OVH credits allow; destroy always; no KEEP. Verify evidence uses `certification-ovh` cell identity or the cell stays `not-run` and visible. Unit/Floci MUST NOT certify Magento.

## 4. Docs lockstep

- [x] 4.1 Update `docs/ovh-experimental.md` and evidence README links to `certification-ovh`. Humanizer then remove-ai-marks. Verify `make docs` if that page is in the MkDocs nav.
