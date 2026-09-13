## 1. Tracker split

- [x] 1.1 Add `certification-scaleway` to `openspec/README.md`. Point `audit-catalog-cert-campaign` task 6.4 Scaleway at this change. Verify `openspec validate --change certify-scaleway --strict`.
- [x] 1.2 State in `docs/capability-matrix.md` that Kapsule is experimental, certified subset empty, cache family Redis, Bootstrap `ErrNotSupported`. Verify no page calls Scaleway certified.

## 2. Implemented Scaleway catalog

- [x] 2.1 Enumerate Kapsule version/node, RDB HA/backup/encryption, Redis version/cluster size, and Cockpit with Adobe / MageLift / experimental / unavailable columns. Verify every relevant `ScalewayTarget` field appears or is typed unavailable.
- [x] 2.2 Keep Redis vs Adobe Valkey mismatch visible. Verify docs do not relabel Scaleway Redis as Valkey.

## 3. Live bar

- [x] 3.1 After apply, run at most one Kapsule Magento preview from the $50 own-money cap shared with Cloudflare/SendGrid/Fastly/New Relic; destroy always; no KEEP; vendors stay on GCP origin. Verify evidence uses `certification-scaleway` cell identity or the cell stays `not-run` if the cap is spent. Cost classification MUST NOT certify Magento.

## 4. Docs lockstep

- [x] 4.1 Update `docs/scaleway-experimental.md` and evidence README links to `certification-scaleway`. Humanizer then remove-ai-marks. Verify `make docs` if that page is in the MkDocs nav.
