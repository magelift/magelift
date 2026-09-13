# Scaleway Kapsule infrastructure live acceptance — 2026-08-13

Status: **PASS for the bounded infrastructure-only cell**. This record does
not certify Magento runtime health, multi-zone HA, backup/restore, regional
DR, observability collection, or edge behavior.

## Cell

- Provider project: redacted
- Region and zone: `fr-par`, `fr-par-1`
- Kapsule: `1.36.1`, one `DEV1-L` worker
- Managed database: MySQL `8`, `DB-DEV-S`
- Managed cache: Redis `8.6.3`, `RED1-MICRO`
- Network: one disposable `/22` VPC/private network
- Artifact: public immutable NGINX digest, used only as a pullable
  infrastructure smoke image; it is not Magento-compatible
- Harness: `scripts/k8s-acceptance-local.sh scaleway`, `deploy --infra-only`
- Ownership prefix: `scwrc1`
- Catalog spec: `certification-scaleway`
- Cell: `fr-par` / `fr-par-1`, Kapsule `1.36.1`, RDB not HA, Redis cluster size 1, digest is NGINX infra-only (not Magento)
- TTL: `3600s`; Pulumi state and passphrase were local to the disposable
  acceptance work directory

## Result

PASS:

1. Dependency, authenticated-profile, project, region, one-cell-catalog, and
   immutable-image preflights passed before provider mutation.
2. The run created 24 resources: VPC/private network, RDB instance and
   database, Redis cluster, Kapsule cluster and pool, Kubernetes provider,
   generated secrets, Deployments, and a LoadBalancer Service.
3. Provisioning completed in `8m23s` (`502s`). The RDB instance, Redis cluster,
   and Kapsule worker pool were the material asynchronous waits.
4. The infrastructure-only output stage passed and returned the owned cluster,
   network, private-subnet, database-secret, encryption-key-secret, and
   service identities. Runtime health was not claimed because the immutable
   NGINX image cannot run Magento.
5. Teardown deleted all 24 resources in `2m26s` (`146s`). The Kubernetes
   Service emitted a transient warning while its external load balancer was
   not ready; the Service and owning infrastructure still converged cleanly.
6. The harness's independent cleanup assertion returned `scaleway
   assert_clean ok`.
7. Fresh direct owning-service inventories after the harness exited returned
   zero resources with the exact `scwrc1` prefix for Kapsule clusters, RDB
   instances, Redis clusters, VPC private networks, and load balancers.

## Configuration correction discovered before the successful cell

The minimal example target was not sufficient for the Kapsule runtime graph:
the provider-neutral Kubernetes workload contract requires
`target.scaleway.databaseName`, `target.scaleway.masterUsername`, and
`target.scaleway.encryptionKeySecret`. The first attempt stopped at the
provider plan before provisioning, and the corrected disposable config passed
validation and created the intended graph. A direct preview without the
harness also inherited a missing user S3 Pulumi backend; the live harness
correctly isolates state in a local disposable backend.

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
