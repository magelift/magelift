# GCP acceptance (local, destroy-on-exit)

Catalog ownership: `certification-gcp` cells in the [capability matrix](capability-matrix.md)
plus [evidence](evidence/README.md). Certified subset is GKE
Autopilot evidenced runtime cells. Standard and HA stay experimental.
`gcap29` KEEP is closed (destroy emptied the prefix). It is not a
certified-row replacement for `gcap28`. Magento 2.4.8-p5 stays `not-run`
until a signed digest exists. Cell identity (runtime, Magento release, Cloud SQL
availability, Memorystore, OpenSearch, queue, digest) belongs in the evidence
file. GCP KEEP does not wait on AWS.

Status: **current evidence exists for three 2.4.9 profiles, but the complete
release and service matrix remains open**. On 2026-08-07, the preview profile
passed 12 cells with Cloud SQL `MYSQL_8_4`, database-backed messaging, and a
one-replica OpenSearch workload. Standard and HA Standard each passed 13 cells
with Cloud SQL `MYSQL_8_4`, RabbitMQ 4.3, pinned OpenSearch 3, and the real B2B
dump. The 2.4.6-p15 standard and HA Standard runs also passed 13 cells with
the current images and full dump, but their MySQL engine is not an Adobe-listed
latest-patch database choice. IDs are redacted in public docs.
Current proof lives in the [evidence pack](evidence/README.md): Autopilot
Magento 2.4.9 preview is
[gcap28](evidence/gcp-gke-autopilot-magento-live-gcap28-20260820.md);
GKE Standard Valkey 9.0 is
[20260815](evidence/gcp-gke-standard-magento-valkey90-live-20260815.md);
HA known-content (pod/node/zone-loss simulation) is
[gcha36](evidence/gcp-gke-ha-standard-magento-live-gcha36-20260820.md).
Local re-runs write under `.magelift/gcp-matrix/`. Sibling path:
[aws-acceptance.md](aws-acceptance.md).

A 2026-08-12 fail-closed infrastructure-only probe confirmed Memorystore
`VALKEY_9_0` (`ACTIVE`, one shard, zero replicas, provider-reported
`MULTI_ZONE`). That is a cache version/topology boundary, not Magento
application certification.

## Offline harness shape

GCP shares the AWS acceptance control-flow shape (checkpoint/resume + evidence
`append_row` + destroy/`force_clean_orphans`/`assert_clean` on EXIT) without
spending credits in dry-run:

```sh
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
bash tests/acceptance/gcp_harness_shape_test.sh
# or: ./providers/gcp/scripts/gcp-acceptance-local.sh
```

The combined account-free stack is `make local-gates` (Pulumi mocks, harness,
Floci AWS, floci-gcp).

| Artifact | Path |
|----------|------|
| Cell catalog | `scripts/acceptance/cells-gcp-preview.txt`, `cells-gcp-standard.txt`, `cells-gcp-high-availability.txt`, or the automatic infra-only `scripts/acceptance/cells-gcp-infra-only.txt` |
| Checkpoint | `.magelift/gcp-matrix/acceptance-checkpoint.json` |
| Evidence | `.magelift/gcp-matrix/matrix-results.md` (gitignored local ledger); publish one current proof under [evidence](evidence/README.md) |

**Evidence contract:** certification requires harness `append_row`
PASS rows for SC1-SC5 cells. Hand-typed matrix rows are not certification
evidence. Resume kills mid-matrix at the first incomplete cell via
shared `lib-checkpoint.sh` (`cell_done` / `record_cell`).

Dry-run never sets `created=1` and never calls Pulumi/gcloud `up`. Live create,
PSA soak timing (`MAGELIFT_GCP_PSA_SOAK_SECS`), and leftover proof are the paid
pass documented in the [evidence pack](evidence/README.md).

### Ordered cells (create-once, then updates)

Catalog order (teardown is EXIT, not a cell):

1. `bootstrap:wif`: Ensure WIF **plus** Act or gcloud STS/WIF token-exchange
   proof (`MAGELIFT_GCP_WIF_ACT_LOG` / `MAGELIFT_GCP_WIF_ACT_PROOF=1` or
   impersonation exchange). Ensure-only is refused.
2. `composer:sm-write` / `composer:sm-read`: Secret Manager Composer creds
   (`gcp-secret-manager://`); no nil-success.
3. `day2:secrets` / `day2:state` / `day2:logs` / `day2:exec` / `day2:health`
4. `migrate:dump`: after infrastructure create and before the candidate (D-03);
   kube dumpimport runner for private SQL
5. `deploy:candidate`: Magento CLI preflight, then migrate → cutover → health →
   record (`kube.Steps`)
6. `cost:estimate`
7. `cutover:dns`: after `applicationURL` exists (D-04);
   `scripts/cutover-dns-cloudflare.sh`

Create-once, before that catalog loop, runs `magelift sign` when a Cosign
identity-token source is configured, then `magelift promote` of
`MAGELIFT_GCP_ACCEPTANCE_DIGEST` with `MAGELIFT_CERTIFICATE_IDENTITY`
(or the `MAGELIFT_GCP_CERTIFICATE_IDENTITY` alias). Unsigned digests fail closed
at promote, not at Magento cutover. Rollback copies those signatures onto later
deploy rows.

Live EXIT contract: when `created=1` and KEEP is false, cleanup runs `destroy` →
`force_clean_orphans` (PSA soak + peering teardown) → `assert_clean`. Dry-run
short-circuits soaks and never enters that path.

The current preview profile passed all 12 cells in its profile-scoped catalog.
It intentionally omits `queue:health` because database-backed messaging has no
broker. It still creates and checks OpenSearch, so it is not a no-search run.
See [gcap28](evidence/gcp-gke-autopilot-magento-live-gcap28-20260820.md).

The current 2.4.6-p15 standard shape passed all 13 cells with GKE Autopilot,
one OpenSearch replica, and one RabbitMQ replica. Its HA shape passed all 13
cells with GKE Standard, three OpenSearch replicas, two RabbitMQ replicas, and
node-level sysctl support. These are MageLift topology/runtime results; Adobe's
current 2.4.6-p15 latest-patch row does not list MySQL.

The current 2.4.9 standard and HA shapes passed all 13 cells with Cloud SQL
`MYSQL_8_4`, OpenSearch 3, RabbitMQ 4.3, and the B2B candidate. Those historical
runs used the former Valkey 8 shape. The implementation now selects
Memorystore `VALKEY_9_0` (GA) for the 2.4.9 profile and permits
`VALKEY_9_1` only as an explicit Preview profile. The corrected 2026-08-12
infrastructure-only probe proves the live `VALKEY_9_0` API mapping, but did not
run the Magento image, dump, migration, or application cache checks; the full
application/cache intersection remains open. None of these profiles closes the
2.4.6-through-2.4.9 matrix.

## Safety rules

- Set `MAGELIFT_GCP_ACCEPTANCE=1` or the script refuses to run (unless dry-run).
- Default mode is `preview` (Pulumi plan only).
- `up` creates the selected GKE runtime, Cloud SQL, and Memorystore Valkey.
  Costly and slow. Destroy always runs on EXIT unless
  `MAGELIFT_GCP_ACCEPTANCE_KEEP=true`. High-availability defaults to GKE
  Standard because the three-node OpenSearch shape needs a node-level sysctl.
- A failed cell also cleans up by default. Set
  `MAGELIFT_GCP_ACCEPTANCE_KEEP_ON_FAILURE=true` only when you explicitly need
  to retain the paid stack for a resume.
- The live harness assigns a six-hour disposable TTL by default. Override it with
  `MAGELIFT_ACCEPTANCE_TTL_SECONDS` (maximum 24 hours); the watchdog forces the
  EXIT cleanup path at expiry and overrides `KEEP=true`.
- **`MAGELIFT_GCP_ACCEPTANCE_DIGEST` must be pullable** for Magento cells
  (day2 logs/exec/health, `deploy:candidate`). The default placeholder digest
  fails ImagePull. For a managed-service/topology probe when no Magento image
  or real schema dump is available, set `MAGELIFT_GCP_ACCEPTANCE_INFRA_ONLY=1`
  and accept the automatic `cells-gcp-infra-only.txt` catalog, or provide a
  custom catalog containing only `bootstrap:wif`, Composer Secret Manager,
  state/secret inventory, `search:health`, `queue:health`, and `cost:estimate`
  cells. The harness rejects application cells in this mode
  before creating a state bucket or provider resource. Composer cells are
  accepted only when `MAGELIFT_GCP_COMPOSER_SECRET_ID` is explicitly set to a
  run-owned `$MAGELIFT_GCP_ACCEPTANCE_NAME-...` secret; the preserved
  unmarked `magelift-composer-auth` secret is never a write target. Infra-only
  evidence is explicitly not Magento application certification.
- Live defaults target Adobe Commerce 2.4.9, PHP 8.5, and Composer 2.10. The
  candidate cell runs `bin/magento list --no-ansi` in the ready web pod and
  requires `cache:clean`, `cache:flush`, and `cache:status` before it starts a
  migration candidate.
- The high-availability catalog includes `resilience:pod-loss`. That bounded
  cell deletes one ready web pod, waits for a different ready replacement and
  the desired replica count, then reruns runtime health. It is pod-rescheduling
  evidence only; it does not claim node, zone, database, queue, cache, DR,
  fencing, or failback behavior.
- Before live cells begin, the harness reads the Cloud SQL instance's actual
  `databaseVersion` and compares it with the Magento release mapping. A
  mismatch fails the run before Magento cells spend more credits.
- After destroy, the script asserts zero leftovers matching the acceptance name
  prefix (`mlacc` by default; override with `MAGELIFT_GCP_ACCEPTANCE_NAME`) for VPC, GKE, Cloud SQL, Memorystore, and secrets.
- The disposable Pulumi state bucket is created only after the cleanup trap and
  all cleanup helpers are installed, so a preflight or state-bucket failure is
  still followed by bucket cleanup.
- Before ADC, API enablement, state-bucket creation, or any provider resource,
  the harness checks local free space. The default `12 GiB` threshold protects
  the serial Go/Pulumi build from macOS temporary-volume exhaustion; override
  it only after measuring the local build footprint with
  `MAGELIFT_GCP_ACCEPTANCE_MIN_FREE_MB`.
- Never interrupt mid-create/destroy.
- The harness builds its CLI under `GOMEMLIMIT` at 75% of available RAM
  (`scripts/go-memlimit.sh`). Do not pin `GOMAXPROCS` or `-p`.
- When another agent works on AWS in the main checkout, run this from a separate
  checkout and keep a unique acceptance prefix so resource names and
  `/tmp` workdirs do not collide.

## Warm matrix sessions

The live catalog already creates one stack and updates it between cells. It does
not destroy GKE, Cloud SQL, or Memorystore between cells. The expensive part is
the first create and the provider teardown, so related application runs can use
one retained stack:

```bash
export MAGELIFT_GCP_ACCEPTANCE=1
export MAGELIFT_GCP_PROJECT=YOUR_DISPOSABLE_PROJECT
export MAGELIFT_GCP_REGION=europe-west1
export MAGELIFT_GCP_ACCEPTANCE_PROFILE=standard
export MAGELIFT_GCP_ACCEPTANCE_NAME=warm249
export MAGELIFT_GCP_ACCEPTANCE_DIR=/tmp/magelift-gcp-warm249
export MAGELIFT_GCP_ACCEPTANCE_KEEP=true
export MAGELIFT_CERTIFICATE_IDENTITY='…'   # Sigstore subject that signed DIGEST
# export MAGELIFT_CERTIFICATE_OIDC_ISSUER=https://accounts.google.com
# MAGELIFT_GCP_CERTIFICATE_IDENTITY remains an alias.
# Optional non-interactive sign (JSON argv; stdout is a JWT, never logged):
# export MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV='["gcloud","auth","print-identity-token","--audiences=sigstore","--include-email","--impersonate-service-account","sa@project.iam.gserviceaccount.com"]'

# Run one release and catalog. Change the release, digest, seed dump, or cell
# catalog for the next compatible run while keeping NAME and DIR unchanged.
./providers/gcp/scripts/gcp-acceptance-local.sh up

# After the last warm run, unset KEEP and resume the completed checkpoint. This
# performs the final destroy, orphan scan, and resource assertion.
export MAGELIFT_GCP_ACCEPTANCE_KEEP=false
export MAGELIFT_GCP_ACCEPTANCE_RESUME=1
./providers/gcp/scripts/gcp-acceptance-local.sh up
```

The checkpoint now includes a configuration fingerprint. A changed release,
image digest, runtime, or cell catalog starts a fresh cell set and retains the
old checkpoint as a local `.stale-*.json` file. Keep the same `DIR` so the
temporary Pulumi passphrase remains available for the retained state.

Warm reuse is appropriate for releases that keep the same database major and
topology, such as the 2.4.7 through 2.4.9 Cloud SQL MySQL 8.4 runs. Use a cold
stack for a database engine or major-version change, Autopilot versus Standard,
preview versus high-availability, a provider change, or a managed-service
replacement. Those tests verify provisioning and migration behavior that a
warm update cannot prove.

The harness preserves Pulumi's dependency order during the first destroy pass:
Cloud SQL users and databases are removed before the instance, and the GKE
cluster stays behind the Kubernetes `LoadBalancer` Service. If that pass
returns a provider error, `force_clean_orphans` then starts exact producer
deletes and waits for direct inventories before removing the network. This is
slower than racing deletes, but it avoids orphaning child resources or making
the provider reject a valid destroy.

## Invoke

```bash
# From this repository (use a separate checkout only if you need parallel AWS work)
cd /path/to/magelift

# Preview graph (default)
MAGELIFT_GCP_ACCEPTANCE=1 ./providers/gcp/scripts/gcp-acceptance-local.sh preview

# Short-lived real stack + live_cell_loop (destroy on EXIT)
MAGELIFT_GCP_ACCEPTANCE=1 \
MAGELIFT_GCP_ACCEPTANCE_DIGEST='ghcr.io/magelift/magento@sha256:…' \
./providers/gcp/scripts/gcp-acceptance-local.sh up
```

Resume after a mid-matrix kill (stack kept with KEEP):

```bash
MAGELIFT_GCP_ACCEPTANCE=1 \
MAGELIFT_GCP_ACCEPTANCE_KEEP=true \
MAGELIFT_GCP_ACCEPTANCE_RESUME=1 \
MAGELIFT_GCP_ACCEPTANCE_DIGEST='ghcr.io/…@sha256:…' \
./providers/gcp/scripts/gcp-acceptance-local.sh up
```

Optional env:

| Variable | Default | Purpose |
| --- | --- | --- |
| `MAGELIFT_GCP_PROJECT` | **required** (live); dry-run uses `example-gcp-project` | Your disposable GCP project; never commit real IDs |
| `MAGELIFT_GCP_REGION` | `europe-west1` | Region |
| `MAGELIFT_GCP_ACCEPTANCE_NAME` | `mlacc` | Magento project name / resource prefix |
| `MAGELIFT_GCP_ACCEPTANCE_RUNTIME` | profile default | `gke-autopilot` or `gke-standard`; HA defaults to Standard when unset, and fails closed if a leftover Autopilot value is still set |
| `MAGELIFT_GCP_ACCEPTANCE_DIGEST` | placeholder digest | **Must be pullable** for Magento cells |
| `MAGELIFT_CERTIFICATE_IDENTITY` | **required** for Magento `up` | Sigstore certificate identity; `MAGELIFT_GCP_CERTIFICATE_IDENTITY` is an alias |
| `MAGELIFT_CERTIFICATE_OIDC_ISSUER` | `https://token.actions.githubusercontent.com` | Sigstore OIDC issuer; use `https://accounts.google.com` for Google SA tokens |
| `MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV` | unset | JSON argv that prints a JWT for `magelift sign`; file/env alternatives exist |
| `MAGELIFT_GCP_ACCEPTANCE_INFRA_ONLY` | `0` | `1`/`true` permits only the fail-closed managed-service/topology catalog; it does not certify Magento |
| `MAGELIFT_GCP_ACCEPTANCE_VERSION` | `2.4.9` | Adobe Commerce version sent to the runtime contract |
| `MAGELIFT_GCP_ACCEPTANCE_PHP` | `8.5` | PHP version sent to the build contract |
| `MAGELIFT_GCP_ACCEPTANCE_COMPOSER_VERSION` | `2.10` | Composer version sent to the build contract |
| `MAGELIFT_GCP_ACCEPTANCE_ENABLE_CLOUD_ARMOR` | `false` | Create the GCP Cloud Armor policy; enable only for a quota-enabled WAF cell |
| `MAGELIFT_GCP_ACCEPTANCE_MEMORY_REQUEST` | `2Gi` | Memory request for PHP-FPM, cron, queue, and migration workloads; lower values are an explicit under-sizing choice |
| `MAGELIFT_GCP_ACCEPTANCE_MIN_FREE_MB` | `12288` | Minimum local free space checked before ADC/API setup and the provider-specific build |
| `MAGELIFT_GCP_ACCEPTANCE_DIR` | `/tmp/magelift-gcp-wt` | Work dir + logs |
| `MAGELIFT_GCP_ACCEPTANCE_RESUME` | unset | Skip create-once when stack already up |
| `MAGELIFT_GCP_EXEC_RETRY_ATTEMPTS` | `10` | Retries for the specific GKE `No agent available` exec error |
| `MAGELIFT_GCP_EXEC_RETRY_INTERVAL_SECS` | `30` | Delay between those exec retries |
| `MAGELIFT_GCP_WIF_ACT_LOG` | unset | Act smoke log proving token exchange |
| `MAGELIFT_GCP_WIF_ACT_PROOF` | unset | Operator-attested Act proof (`=1`) |
| `MAGELIFT_DUMPIMPORT_RUNNER` | unset | Set `kube` for private Cloud SQL dump cell |
| `GOOGLE_APPLICATION_CREDENTIALS` | auto from gcloud login | Preferred; refreshable ADC |
| `GOOGLE_OAUTH_ACCESS_TOKEN` | avoided when ADC exists | Static token; expires mid-Up |

### Native observability routing budget

The disposable native Google Cloud Logging/Monitoring cell is run with
`providers/gcp/scripts/gcp-observability-acceptance-local.sh`. Its default
`MAGELIFT_GCP_OBSERVABILITY_VERIFY_BUDGET=4m` is intentionally a cheap
configuration-and-delivery probe. A new sink can take substantially longer to
show matching entries in its destination, so an operator who needs the custom
bucket routing claim may set an explicit duration such as `1h`; the command
accepts values in milliseconds, seconds, minutes, or hours and caps the budget
at two hours. During that wait it republishes the same marker-scoped log and
metric probes through the provider adapter; this creates no additional
dashboard, alert, bucket, sink, or metric-descriptor resources. The longer
budget also extends the lifecycle context so cleanup still has a five-minute
grace period. A four-minute timeout must be recorded as
long-latency/inconclusive, never as a passing retention-delivery result.

### GKE collector delivery

The GKE Contrib collector cell runs through
`providers/gcp/scripts/gcp-collector-acceptance-local.sh` and verifies logs, metrics, and
traces through a separate New Relic query credential. The default mode uses an
existing cluster and never creates one. The live command requires
`MAGELIFT_GCP_COLLECTOR_ACCEPTANCE=1`.

For a disposable one-node GKE Standard cell, set the create gate and runtime
explicitly. The harness derives a cluster name when one is not supplied,
applies exact ownership labels, and deletes only that cluster after the
collector lifecycle finishes:

```bash
MAGELIFT_GCP_COLLECTOR_ACCEPTANCE=1 \
MAGELIFT_GCP_COLLECTOR_CREATE_CLUSTER=1 \
MAGELIFT_GCP_COLLECTOR_RUNTIME=gke-standard \
MAGELIFT_GCP_COLLECTOR_CLUSTER_LOCATION=europe-west1-b \
MAGELIFT_GCP_COLLECTOR_IMAGE_DIGEST=docker.io/otel/opentelemetry-collector-contrib@sha256:YOUR_VERIFIED_DIGEST \
MAGELIFT_NEWRELIC_ACCOUNT_ID=YOUR_ACCOUNT_ID \
MAGELIFT_NEWRELIC_OTLP_ENDPOINT=https://otlp.eu01.nr-data.net \
MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT=https://api.eu.newrelic.com/graphql \
make gcp-collector-acceptance-local
```

Use `MAGELIFT_ACCEPTANCE_DRY_RUN=1` to inspect the resolved name and location
without provider mutation. GKE Standard creation uses one `e2-medium` node in
`europe-west1-b` by default, the `regular` release channel, the default VPC and
subnetwork, and a bounded exact-name delete poll. Autopilot and existing-cluster
cells use the same command with `MAGELIFT_GCP_COLLECTOR_CREATE_CLUSTER=0`.

Logs: `/tmp/magelift-gcp-wt/logs/*.log`. Grep those files; do not rely on
shell scrollback.

The live script builds `cmd/magelift` (the shipped CLI). Set
`MAGELIFT_GCP_ACCEPTANCE_BIN` to an already built binary to avoid rebuilding
during a resume.

Auth note: long Ups must use Application Default Credentials (refreshable).
A static `GOOGLE_OAUTH_ACCESS_TOKEN` expires (~40-60m) and fails GKE/Memorystore
operation polls with `ACCESS_TOKEN_TYPE_UNSUPPORTED`. The script materializes ADC
from `~/.config/gcloud/credentials.db` when ADC is missing.

## Force cleanup after a failed destroy

Cloud SQL can keep a Service Networking allocation for several minutes after the
instance is gone (`FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION`). The EXIT
trap waits for GKE/SQL/Memorystore/SCP producers to clear, soaks
(`MAGELIFT_GCP_PSA_SOAK_SECS`, default 180), then removes the peering via Compute
`networks.removePeering` (and async `vpc-peerings delete`), the PSA address, and
the VPC. If assert_clean still fails, wait and retry:

```bash
# Prefer Compute removePeering when services vpc-peerings delete races soft-delete
# Substitute YOUR_PROJECT and PREFIX from the failed run.
TOKEN=$(gcloud auth print-access-token)
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "https://compute.googleapis.com/compute/v1/projects/YOUR_PROJECT/global/networks/PREFIX-preview-net/removePeering" \
  -d '{"name":"servicenetworking-googleapis-com"}'
gcloud compute addresses delete PREFIX-preview-sql-psa --global --project=YOUR_PROJECT
gcloud compute networks delete PREFIX-preview-net --project=YOUR_PROJECT
```

## Credentials

The harness checks `jq` (and Mike Farah `yq` v4 where YAML is patched) before
it provisions GKE, Cloud SQL, or Memorystore. See
[acceptance command dependencies](acceptance-dependencies.md) for install
guidance and the offline preflight test.

- `gcloud` must be authenticated to the target project.
- Pulumi needs a backend. The harness ignores an ambient `PULUMI_BACKEND_URL` so
  a stale login cannot select a deleted bucket; set
  `MAGELIFT_GCP_ACCEPTANCE_BACKEND_URL` only when intentionally using an
  externally managed backend. Otherwise it creates and cleans its disposable
  GCS bucket. Ephemeral agent
  accounts from Automation API must be claimed by a human if used.
- Prefer Application Default Credentials; the script falls back to
  `GOOGLE_OAUTH_ACCESS_TOKEN` from `gcloud auth print-access-token`.
- Cloudflare DNS cell needs `CLOUDFLARE_API_TOKEN` / `CF_API_TOKEN` with
  Zone.DNS Edit (Wrangler OAuth is insufficient).

## Known gaps (post-certify)

- Memorystore Valkey needs a regional Service Connection Policy
  (`serviceClass=gcp-memorystore`). Created by the GCP cache component.
- Non-preview presets remain spend-gated (not free-tier certified).
- GitHub WIF CI proof is **Act-only** until hosted Actions minutes return. See
  [gcp-experimental.md](gcp-experimental.md#github-wif-act-only-until-minutes-return)
  and `.github/workflows/gcp-wif-act-smoke.yml`. Do not commit SA keys.
- Live dump cell requires `MAGELIFT_DUMPIMPORT_RUNNER=kube` (+ mysql client pod
  when the Magento image lacks `mysql`); evidenced on the Autopilot certified
  path.
- A 2026-08-04 disposable run passed WIF, Composer Secret Manager, secrets,
  state, and logs, then failed at GKE `No agent available` during `day2:exec`.
- The 2026-08-05 catalog run completed CLI preflight, seed import, candidate
  migration, health, and cleanup. The WIF row used the operator-attested Act
  proof flag, not a hosted Actions run.
- The 2026-08-07 2.4.9 MySQL 8.4 standard retry reached the real B2B dump but
  failed at `migrate:dump` with MySQL error 1419. The adapter now enables
  `log_bin_trust_function_creators`.

## Current proof

| Item | Value |
|------|--------|
| Pack | [evidence/README.md](evidence/README.md) |
| Certified Autopilot Magento | [gcap28](evidence/gcp-gke-autopilot-magento-live-gcap28-20260820.md) |
| Experimental Standard | [20260815](evidence/gcp-gke-standard-magento-valkey90-live-20260815.md) |
| Experimental HA Standard | [gcha36](evidence/gcp-gke-ha-standard-magento-live-gcha36-20260820.md) |
