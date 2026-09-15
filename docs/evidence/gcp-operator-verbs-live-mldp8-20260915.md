# GCP operator verbs live proof - 2026-09-15 (`mldp8`)

Five-verb packed session for order 12 (`operator-finops-catalog`): one
GKE Autopilot preview, 13/13 acceptance cells PASS, then `health`,
`logs`, `exec` (plus the `--service deploy` rejection), `cost`, and
`cleanup reconcile` exercised live against the running stack. The
session also live-proved the destroy-after-expiry fix: an expired YAML
refused deploy and completed destroy. AWS `cost --live` (Fargate vCPU
$17.74, memory $3.87, Valkey $10.51/month) and `cost --budget`
(preview not-configured, staging lists five account budgets) were
proven creds-only the same day; those reads fixed five Price List
filter defects and a Budgets page-size defect. Do not reuse `mldp8`
(WIF pool name tombstoned 30d).

## Scope

| Field | Value |
| --- | --- |
| Project | GCP acceptance project (accountRef redacted in sealed JSONL) |
| Region | `europe-west1` |
| Name | `mldp8` |
| Profile | preview |
| Runtime | `gke-autopilot` |
| Catalog | `scripts/acceptance/cells-gcp-preview.txt` (13 cells) |
| Artifact A | `…/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13f…fdb2be4` |
| Seed | `magento-249-sanitized-definer-free.sql.gz` (102 KiB) |
| KEEP | true during verbs, manual destroy after |

## False start

The first `up` attempt inherited a stale `MAGELIFT_ACCEPTANCE_CELL_CATALOG`
pointing at an OVH cell file from order 11 and failed closed on
`unsupported gcp acceptance cell` with zero cloud changes. The operator
purged the leaked variables, removed the bogus checkpoint entry, and
resumed; all 13 cells below come from the clean resume. The sealed
bundle keeps final records only.

## Verbs

All five against the live `mldp8-preview` stack:

- `health --mode runtime`: healthy, deployment 1/1 ready, Magento
  HTTP 200.
- `logs --service web`: real nginx lines, early rollout 500s
  converging, matching the `mldp3` pattern.
- `exec --service web -- php -r ...`: PHP 8.5.9. `exec --service
  deploy` refused with the guided pointer to deploy logs.
- `cost`: account-free inputs (Autopilot web, Cloud SQL, Memorystore,
  edge, db queue) plus an explicit unpriced list; no live GCP prices
  claimed.
- `cleanup claim|record|plan|reconcile --yes` on a probe ledger:
  live Cloud SQL inventory ran over ADC, the gone identity resolved,
  report `complete`, ledger persisted. (Wiring the GCP cleanup
  provider into the CLI was part of this order; the hook existed but
  no binary populated it.)

## Expiry

`deploy` with an expired YAML refused (`preview environment has
expired`); `destroy --yes --destroy-backups` with the same YAML
deleted 22 resources. Pre-fix the destroy died in the Pulumi program;
the allow-expired flag now rides the spec in all five providers.

## Teardown

Pulumi destroy left the service-networking peering plus the network
it blocked (same GCP-side refusal as `mldp7`). Manual sweep deleted
NAT, router, service account, three secrets, media bucket, four
subnets, PSA range, and the VPC; deleting the VPC removed the stuck
peering with it. Pulumi stack removed, state bucket removed, WIF pool
deleted. Final sweep: zero `mldp8` resources in every listing.

## Spend

Paid resources (Autopilot cluster, Cloud SQL, Memorystore, LB) lived
for the run window (~75 min create-to-destroy); no leftovers.

## Non-claims

- GCP live unit prices (`cost` stays account-free there)
- RDS / OpenSearch / Amazon MQ live-price shapes (fixed, await a
  session that runs them)
- Cloud SQL `Delete` over a live instance (unit-covered; the probe
  resolved a gone identity)
