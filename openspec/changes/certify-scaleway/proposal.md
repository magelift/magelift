## Why

Some EU agencies run Magento on Scaleway Kapsule with managed RDB and Redis. MageLift already types those knobs, but certification is a one-liner under a packed campaign with a shared **$50** own-money cap. Operators need the implemented Scaleway possibility space listed, with only a thin experimental preview as the live bar.

## What Changes

- Add a first-party **`certification-scaleway`** spec that enumerates implemented Scaleway cells (Kapsule version/node, RDB HA/backup/encryption, Redis cluster size, Cockpit) and names the **certified subset** (empty until evidence exists).
- Keep Scaleway **experimental**. One live Kapsule Magento preview from the **$50** own-money cap shared with Cloudflare/SendGrid/Fastly/New Relic; **no KEEP**.
- Combinations the adapter cannot ship MUST fail closed. Combinations it can ship MUST appear in the catalog even if never live-certified.
- Take packed-campaign task 6.4 Scaleway slice out of `audit-catalog-cert-campaign` and track it here. Vendors attach to a GCP origin only, not a Scaleway origin, until this spec says otherwise.

## Capabilities

### New Capabilities

- `certification-scaleway`: Scaleway implemented-cell catalog, experimental status, $50-cap one-preview live bar, Cockpit honesty, and campaign ownership.

### Modified Capabilities

- `efficient-cloud-certification`: Scaleway MUST stay one preview, no KEEP, inside the $50 vendor/own-money cap, independent of AWS/GCP KEEP status.
- `certification-evidence`: Scaleway evidence maps to `certification-scaleway` cell IDs; account-free cost classification MUST NOT certify Magento.

## Impact

`docs/capability-matrix.md` Scaleway rows, `docs/scaleway-experimental.md`, `docs/evidence/`, Scaleway adapter admission, OpenSpec README, and packed-campaign 6.4 Scaleway. No new cloud topology in this planning step. Live Kapsule preview remains apply-time and money-capped.
