## Why

EU agencies often land Magento on OVH Public Cloud (MKS, managed MySQL, Valkey, private networking). MageLift already exposes a typed OVH target, but certification is a single “one MKS preview” line inside a packed AWS/GCP campaign. Operators need the full implemented OVH possibility space listed, with only a thin experimental preview as the live bar until matrix plus evidence say otherwise.

## What Changes

- Add a first-party **`certification-ovh`** spec that enumerates implemented OVH cells (MKS plan, MySQL plan/version/nodes, Valkey plan/version, floating IPs, private-network routing, Logs Data Platform) and names the **certified subset** (empty until evidence exists).
- Keep OVH **experimental**. One live MKS Magento preview when credits allow; **no KEEP**. Bootstrap/Secrets stay `ErrNotSupported` until implemented.
- Combinations the adapter cannot ship MUST fail closed. Combinations it can ship MUST appear in the catalog even if never live-certified.
- Take packed-campaign task 6.4 OVH slice out of `audit-catalog-cert-campaign` and track it here.

## Capabilities

### New Capabilities

- `certification-ovh`: OVH implemented-cell catalog, experimental status, one-preview live bar, private-network honesty, and campaign ownership.

### Modified Capabilities

- `efficient-cloud-certification`: OVH MUST stay one preview, serialized, no KEEP, independent of AWS/GCP KEEP status.
- `certification-evidence`: OVH evidence maps to `certification-ovh` cell IDs; topology-only or unit/Floci rows MUST NOT certify Magento.

## Impact

`docs/capability-matrix.md` OVH rows, `docs/ovh-experimental.md`, `docs/evidence/`, OVH adapter admission, OpenSpec README, and packed-campaign 6.4 OVH. No new cloud topology in this planning step. Live MKS preview remains apply-time and credit-gated (~$200 OVH credits when used).
