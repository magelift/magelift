# GCP preview-env loop (`mldp7` base + `pr-999`) — 2026-09-14/15

Live rehearsal of the preview-environment lifecycle on the certified GCP
origin (GKE Autopilot, `europe-west1`): long-running base up, PR preview
created from it, health-verified, guards exercised, both torn down.

## Scope

- Base env `mldp7-preview`: full acceptance matrix (13 cells).
- Preview env `mldp7-pr-999`: bootstrap, deploy, health, guard checks,
  destroy. Preview identity derives from repository plus PR number.
- Teardown completed 2026-09-15; all billable resources deleted.

## Loop rows

| Step | Outcome |
| --- | --- |
| Base `mldp7-preview` acceptance | 13/13 PASS (`bootstrap:wif`, `composer:sm-write`, `composer:sm-read`, `day2:secrets`, `day2:state`, `day2:logs`, `day2:exec`, `day2:health`, `search:health`, `migrate:dump`, `deploy:candidate`, `deploy:repeat`, `cost:estimate`) |
| Preview `pr-999` bootstrap + deploy | Deployed; Magento base URLs corrected post-deploy, runtime health confirmed |
| Production/protected destroy guard | Refusal verified: protected env destroy without `--yes` does not proceed |
| `env sweep --dry-run` | Reports the expired preview without touching the long-running base |
| Preview destroy | Compute, data, buckets, service accounts, subnets deleted; VPC peering delete refused by GCP (see below) |
| Base destroy | Cluster, Cloud SQL, Valkey, buckets, service accounts, routers, subnets deleted |

## Teardown notes

The Pulumi passphrase file for this rehearsal lived in `/tmp` and was
reaped before teardown, so `magelift destroy` could not unlock the stacks
and the teardown ran as ordered `gcloud` deletes (cluster, SQL, Valkey,
buckets, service accounts, NATs, routers, subnets). All billable resources
are gone; verification sweeps found no clusters, SQL instances, Valkey
instances, buckets, service accounts, load-balancer parts, DNS zones, or
firewall rules for either env.

Two Service Networking peerings (`mldp7-preview-net`,
`mldp7-pr-999-net`) plus their empty VPCs and reserved ranges remain:
GCP refuses the peering delete with "Producer services ... are still
using this connection" although no consumers exist (no SQL, no
Memorystore/Valkey, no Redis in any region). This is GCP-side stuck
state, not a MageLift orphan: peerings, empty VPCs, and unused ranges
cost nothing. Retried across 14+ hours, most recently at intent close;
still refused. Future sessions may re-attempt the delete.

## Spend

Both envs lived under one day inside the acceptance project; no separate
budget line was tracked for the rehearsal.

## Sources

- Base transcript: `gcp-loop-mldp7.log` (13 `cell-done ... PASS` rows).
- Preview deploy log: `mldp7-pr999-deploy.log`.
- Session transcript: preview CLI steps plus guard checks plus teardown.
