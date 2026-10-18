# GCP GKE Standard Magento 2.4.9 and Valkey 9.0 repeat cell - 2026-08-15

Status: **PASS for a bounded repeatability cell**. This run repeats the
current GKE Standard Magento boundary already recorded by
`20260813an`; it is not a new architecture claim.

## Scope

| Field | Value |
| --- | --- |
| Provider | Google Cloud |
| Project and region | redacted disposable project / `europe-west1` |
| Run ID | `gcpstd20260815` |
| Profile and runtime | `standard` / GKE Standard |
| Magento | 2.4.9, PHP 8.5, Composer 2.10 |
| Artifact | immutable Artifact Registry linux/amd64 digest `sha256:1ee1e6d41a4e34d58c3e56eb240ec916bde98d2b479396c2b5a253394b8b469d` |
| Database | Cloud SQL MySQL `MYSQL_8_4` |
| Cache | Memorystore for Valkey `VALKEY_9_0` |
| Search | OpenSearch 3, one ready replica on GKE |
| Queue | RabbitMQ 4.3, one ready replica |
| Seed | definer-free installed-schema Magento 2.4.9 dump kept outside the repository |

The create-once graph contained 56 resources and completed in 17m58s. The
application runtime reached two ready web replicas. The harness used the
profile-scoped 13-cell Standard catalog.

## Cell results

All 13 cells passed:

| Cell | Result |
| --- | --- |
| `bootstrap:wif` | PASS |
| `composer:sm-write` | PASS |
| `composer:sm-read` | PASS |
| `day2:secrets` | PASS |
| `day2:state` | PASS |
| `day2:logs` | PASS |
| `day2:exec` | PASS |
| `day2:health` | PASS |
| `search:health` | PASS |
| `migrate:dump` | PASS |
| `deploy:candidate` | PASS |
| `queue:health` | PASS |
| `cost:estimate` | PASS |

The harness verified Cloud SQL `MYSQL_8_4` for Magento 2.4.9, imported the
installed schema, reported Magento CLI 2.4.9, completed the candidate
migration, observed two ready of two desired web replicas, and verified one
RabbitMQ replica with cluster membership.

## Cleanup

The first Pulumi destroy pass deleted 36 resources, then hit the known Google
Service Networking producer-release error. The bounded cleanup path confirmed
the producer resources were gone, soaked the PSA path for 180 seconds, and
removed the exact run-owned GKE node pool, GKE cluster, Valkey instance,
Cloud SQL instance, PSA resources, network, secrets, WIF identity, and state
bucket. Observed provider deletion durations included 634 seconds for Valkey,
274 seconds for the GKE cluster, 263 seconds for the node pool, and 82 seconds
for Cloud SQL.

The harness exited 0 with `assert_clean ok`. Independent post-run checks found
no run-matching GKE cluster, Cloud SQL instance, Memorystore instance, storage
bucket, Workload Identity pool, or Secret Manager secret. The exact ownership
inventory also found no active run-owned network or state resources.

## Boundary and correction

This run confirms Memorystore `VALKEY_9_0` on GKE Standard Magento with
cleanup convergence. The older 2026-08-07 HA record used Valkey 8.0; a current
2.4.9 HA application
cell with Valkey 9.0 remains open.

This evidence does not prove public Magento HTTP or TLS traffic, Cloud Armor
request behavior, HA failure injection, backup or restore, regional DR,
fencing or failback, collector delivery, New Relic composition, or any other
provider architecture.

## Implementation references

- Acceptance wrapper: `scripts/gcp-acceptance-local.sh`
- Profile catalog: `scripts/acceptance/cells-gcp-standard.txt`
- Local operator log: `/tmp/magelift-gcp-wt/logs/acceptance.log`
