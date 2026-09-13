## Context

AWS already has the widest YAML catalog (`target.aws.catalog`). Certification tracking still lives in `audit-catalog-cert-campaign` task 6.3 beside GCP/OVH/Scaleway. `certification-matrix` stays the evidence *how*. Magento env for RDS/Aurora/OpenSearch is a MageLift adapter contract with tests. Private sibling shops may illustrate one field architecture; they do not certify MageLift cells.

## Goals / Non-Goals

**Goals:**
- One AWS-owned spec and matrix section for every implemented cell.
- Named certified subset (Fargate preview Magento + recorded services).
- Packed KEEP remains the live method; campaign 6.3 moves here.
- Fail-closed or typed unavailable for combinations MageLift cannot ship.

**Non-Goals:**
- Cartesian live shops.
- Promoting experimental cells from KEEP PASS before destroy + orphan assert.
- Duplicating portable Magento overlays or inventing SQS as a `queueMode`.
- Changing Homebrew/Scoop or releasing a public tag.
- Destroying the in-flight `awsba` KEEP from this planning change.

## Decisions

1. **Catalog = schema enums, not a new DSL.** `databaseEngine`, `searchMode`, `queueMode`, Fargate/EKS compute modes, NAT, and web runtime already exist. The spec requires the matrix to list them with three-contract status. New YAML keys only when a field architecture is silently impossible today and must become selectable or typed unavailable.

2. **Certified subset stays small.** Fargate + `nginx-fpm` + evidenced catalog. Everything else experimental until tuple evidence. This matches field demand (many YAML shapes) without lying about live proof.

3. **Magento wiring is product + tests, not a private shop.** Provisioned OpenSearch = in-VPC domain Magento can query, no SigV4 sidecar. AOSS = sidecar. Writer endpoint + secret JSON for RDS and Aurora. Details stay in adapter tests. A private Chantelle shop is one example among many agency architectures.

4. **Campaign split is tracker-only until apply.** `audit-catalog-cert-campaign` 6.3 gets a pointer to this change; live KEEP continues under existing harness files. Do not rewrite `cells-aws-campaign-fargate-keep.txt` mid-KEEP (fingerprint).

5. **Shared matrix index lives here.** Sister changes (`certify-gcp`, `certify-ovh`, `certify-scaleway`) add provider specs; only this change adds the `certification-matrix` requirement that every first-party provider owns a catalog spec.

**Alternatives considered:** One change with four specs (simpler merge, worse independent archive). Four copies of Magento env contracts (rejected). Exhaustive live cert per combo (rejected: money and fake rows).

## Risks / Trade-offs

- Four in-flight changes plus the packed campaign can drift. Mitigation: campaign tasks 6.2–6.4 point here; do not duplicate KEEP procedure.
- Listing unimplemented field shapes as “must fail closed” can look like a product gap dump. That is intended honesty.
- Some private shops run Aurora 8.4; MageLift still pins Adobe-gated Aurora 3.11/3.12. Do not silently switch to 8.4 in this change, and do not treat that shop's engine as the certified pin.
