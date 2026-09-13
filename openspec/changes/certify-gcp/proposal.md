## Why

GCP Magento in the field is not one Autopilot preview: agencies pick Autopilot vs Standard, zonal vs regional Cloud SQL, database vs RabbitMQ, OpenSearch on or off, Memorystore shape, and HA. Those choices already exist as YAML on `target.gcp`, but certification tracking is buried in the packed campaign next to AWS. Operators need a GCP catalog they can compose, with only the main architectures certified.

## What Changes

- Add a first-party **`certification-gcp`** spec that enumerates every implemented GCP cell and names the **certified subset** (today: GKE Autopilot evidenced runtime cells).
- Treat `target.gcp` dimensions (runtime Autopilot/Standard, Cloud SQL availability, Memorystore, `openSearchMode`, `queueMode`, HA replicas, Armor) as the field catalog — not three opaque presets that hide combinations.
- GKE Standard, HA, Armor Magento exclusion, and Cloud SQL attach-existing stay classified honestly (experimental, withheld, or named gap). They MUST remain selectable or typed unavailable; they MUST NOT inherit Autopilot certified.
- Take packed-campaign task 6.2 out of `audit-catalog-cert-campaign` and track GCP KEEP here. Independent of AWS KEEP.
- Magento env contracts stay platform-level. Do not copy AWS OpenSearch/Aurora wiring into this spec.

## Capabilities

### New Capabilities

- `certification-gcp`: GCP implemented-cell catalog, certified Autopilot subset, Standard/HA experimental rules, KEEP ownership, and vendor-on-origin attach policy.

### Modified Capabilities

- `efficient-cloud-certification`: GCP KEEP is its own provider group; it MUST NOT block or be blocked by AWS cell status.
- `certification-evidence`: GCP evidence files map to `certification-gcp` cell IDs; KEEP rows are not certified until destroy + orphan assert.

## Impact

`docs/capability-matrix.md` GCP sections, `docs/gcp-acceptance.md`, `docs/evidence/`, GCP acceptance harness, OpenSpec README, and packed-campaign task 6.2. YAML keys on `target.gcp` already exist; expanding “preset-only” docs into an explicit catalog is the product change. Live Autopilot 2.4.8-p5 / 2.4.9 KEEP remains apply-time work.
