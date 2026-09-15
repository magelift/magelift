# Scaleway Kapsule infrastructure live acceptance — 2026-09-15

Status: **PASS for the bounded infrastructure-only cell**. This record does
not certify Magento runtime health, multi-zone HA, backup/restore, regional
DR, observability collection, or edge behavior. It replaces the 2026-08-13
cell (kept in git history).

## Cell

- Provider project: redacted
- Region and zone: `nl-ams`, `nl-ams-1` (`fr-par` Kapsule was in
  provider-side `shortage` on 2026-09-15; admission correctly refused
  before mutation)
- Kapsule: `1.36.4`, one `DEV1-L` worker
- Managed database: MySQL `8`, `db-dev-s`, not HA
- Managed cache: Redis `8.6.6`, `RED1-micro`, cluster size 1
- Network: one disposable `/22` VPC/private network
- Artifact: public immutable NGINX digest, used only as a pullable
  infrastructure smoke image; it is not Magento-compatible
- Harness: `scripts/k8s-acceptance-local.sh scaleway`, `deploy --infra-only`
- Ownership prefix: `scw915`
- Catalog spec: `certification-scaleway`
- Cell: `nl-ams` / `nl-ams-1`, Kapsule `1.36.4`, RDB not HA, Redis cluster size 1, digest is NGINX infra-only (not Magento)
- TTL: `3600s`; Pulumi state and passphrase were local to the disposable
  acceptance work directory

## Result

PASS:

1. Dependency, authenticated-profile, project, region, one-cell-catalog, and
   immutable-image preflights passed before provider mutation.
2. The run created 23 resources: VPC/private network, RDB instance and
   database, Redis cluster, Kapsule cluster and pool, Kubernetes provider,
   generated secrets, Deployments, and a LoadBalancer Service.
3. Provisioning completed in `13m12s`. The RDB instance, Redis cluster,
   and Kapsule worker pool were the material asynchronous waits.
4. The infrastructure-only output stage passed and returned the owned cluster,
   network, private-subnet, database-secret, encryption-key-secret, and
   service identities. Runtime health was not claimed because the immutable
   NGINX image cannot run Magento.
5. Teardown deleted all 23 resources in `3m0s`.
6. The harness's independent cleanup assertion returned `scaleway
   assert_clean ok`.
7. Fresh direct owning-service inventories after the harness exited returned
   zero resources with the exact `scw915` prefix for Kapsule clusters, RDB
   instances, Redis clusters, VPCs, private networks, and load balancers.

## Corrections discovered by the live cell

- The generated managed-service passwords did not guarantee every character
  class (`Special:true` only widens the pool), and Scaleway now rejects
  passwords missing a digit, uppercase, lowercase, or special character.
  The cache and database generators now pin minima for all four classes;
  the mock-graph test pins the guarantee.
- Stale catalog defaults moved to the live values: Redis `8.6.6`,
  `RED1-micro`, RDB `db-dev-s`, Kapsule `1.36.4`.
- One attempt was interrupted mid-deploy by the operator; the orphaned
  cluster, RDB instance, Redis cluster, and VPC were removed with ordered
  provider deletes and re-verified empty before the passing run.

## Spend

About 16 minutes of minimum SKUs (one `DEV1-L` worker, `db-dev-s`,
`RED1-micro`, one Kapsule control plane): well under €1, inside the
shared $50 own-money cap with vendor margin intact.

## Explicit non-claims

- `--infra-only` skipped Magento migration, cutover, and runtime health.
- One zone and one worker do not prove Kapsule multi-zone HA or node-failure
  recovery.
- No backup, restore, known-content integrity, regional DR, fencing,
  failback, collector delivery, native Cockpit workload integration, or edge
  traffic test was run.
- The result does not promote Scaleway to a production-certified target; it
  closes only this deterministic fake-client plus disposable Kapsule
  infrastructure lifecycle evidence for the single-zone cell.
