## Why

Agencies ship as many AWS Magento shapes as there are SMEs: Fargate vs Managed Instances vs EKS, RDS vs Aurora, database vs RabbitMQ vs Amazon MQ, search off vs AOSS vs provisioned OpenSearch. The packed campaign lumps those cells with GCP/OVH/Scaleway, so AWS stall hides other providers and the AWS catalog is hard to audit. MageLift must expose the field-wide YAML space on AWS while certifying only the main architectures.

## What Changes

- Add a first-party **`certification-aws`** spec that enumerates every implemented AWS cell (Adobe vs MageLift vs certified vs experimental vs unavailable) and names the **certified subset**.
- Keep **`certification-matrix`** as the shared *how* a cell becomes certified (evidence tuple, no inferred smoke). Point it at per-provider catalogs instead of packing AWS/GCP/OVH/Scaleway into one campaign tracker.
- YAML MUST keep composing field architectures (compute, database, search, queue, NAT, HA, web runtime, edge, observability). A combination MageLift cannot ship MUST fail closed or be typed unavailable — not omitted from the catalog.
- Live KEEP certifies only the **main** AWS architectures. Warm/cold packing stays; Cartesian live shops stay forbidden.
- Take packed-campaign task 6.3 out of `audit-catalog-cert-campaign` and track it here. Magento env/search/DB wiring is a MageLift product contract (writer endpoint, secret JSON, dual search modes). Private sibling shops are architecture examples only and MUST NOT certify any cell. This spec does not duplicate `env.php` contracts.

## Capabilities

### New Capabilities

- `certification-aws`: AWS implemented-cell catalog, certified subset, KEEP/cold boundaries, Magento-on-AWS wiring honesty, and campaign ownership for Fargate / Managed Instances / EKS.

### Modified Capabilities

- `certification-matrix`: First-party providers MUST own a `certification-<provider>` catalog; shared evidence rules stay here.
- `efficient-cloud-certification`: AWS coverage is boundary-driven on one KEEP digest; exhaustive means the implemented catalog, not N live shops.
- `certification-evidence`: AWS evidence files map to `certification-aws` cell IDs; KEEP rows are not certified until destroy + orphan assert.

## Impact

`docs/capability-matrix.md` AWS sections, `docs/evidence/`, acceptance catalogs under `scripts/acceptance/`, `docs/aws-acceptance.md`, OpenSpec README, and the packed campaign tracker (6.3). Schema catalog enums already exist; this change does not invent new YAML keys unless a field architecture is typed unavailable today and must become selectable. Live Magento KEEP remains a later apply of this change, not this planning step.
