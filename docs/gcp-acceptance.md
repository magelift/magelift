# Experimental GCP acceptance (local, destroy-on-exit)

Status: **experimental**. Not Magento-certified. Prefer
[aws-acceptance.md](aws-acceptance.md) for the certified path.

## Offline harness shape (Phase 7 catalog)

GCP shares the AWS acceptance control-flow shape (checkpoint/resume + evidence
`append_row` + destroy/`force_clean_orphans`/`assert_clean` on EXIT) without
spending credits in dry-run:

```sh
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
bash tests/acceptance/gcp_harness_shape_test.sh
# or: ./scripts/gcp-acceptance-local.sh
```

| Artifact | Path |
|----------|------|
| Cell catalog | `scripts/acceptance/cells-gcp-preview.txt` |
| Checkpoint | `.magelift/gcp-matrix/acceptance-checkpoint.json` |
| Evidence | `.magelift/gcp-matrix/matrix-results.md` (six columns; provider=`gcp`) |

**Evidence contract:** certification (plan 07-07) requires harness `append_row`
PASS rows for SC1–SC5 cells. Hand-typed matrix rows are not certification
evidence (T-07-12). Resume kills mid-matrix at the first incomplete cell via
shared `lib-checkpoint.sh` (`cell_done` / `record_cell`).

Dry-run never sets `created=1` and never calls Pulumi/gcloud `up`. Live create,
PSA soak timing (`MAGELIFT_GCP_PSA_SOAK_SECS`), and leftover proof are the paid
pass (07-06).

### Ordered cells (create-once, then updates)

Catalog order (teardown is EXIT, not a cell):

1. `bootstrap:wif` — Ensure WIF **plus** Act or gcloud STS/WIF token-exchange
   proof (`MAGELIFT_GCP_WIF_ACT_LOG` / `MAGELIFT_GCP_WIF_ACT_PROOF=1` or
   impersonation exchange). Ensure-only is refused.
2. `composer:sm-write` / `composer:sm-read` — Secret Manager Composer creds
   (`gcp-secret-manager://`); no nil-success.
3. `day2:secrets` / `day2:state` / `day2:logs` / `day2:exec` / `day2:health`
4. `deploy:candidate` — migrate → cutover → health → record (`kube.Steps`)
5. `migrate:dump` — after deploy (D-03); kube dumpimport runner for private SQL
6. `cost:estimate`
7. `cutover:dns` — after `applicationURL` exists (D-04);
   `scripts/cutover-dns-cloudflare.sh`

Live EXIT contract: when `created=1` and KEEP is false, cleanup runs `destroy` →
`force_clean_orphans` (PSA soak + peering teardown) → `assert_clean`. Dry-run
short-circuits soaks and never enters that path.

## Safety rules

- Set `MAGELIFT_GCP_ACCEPTANCE=1` or the script refuses to run (unless dry-run).
- Default mode is `preview` (Pulumi plan only).
- `up` creates real GKE Autopilot, Cloud SQL, and Memorystore Valkey — costly and
  slow. Destroy always runs on EXIT unless `MAGELIFT_GCP_ACCEPTANCE_KEEP=true`.
- **`MAGELIFT_GCP_ACCEPTANCE_DIGEST` must be pullable** for Magento cells
  (day2 logs/exec/health, `deploy:candidate`). The default placeholder digest
  fails ImagePull; create-once falls back to `--infra-only` only for that case.
- After destroy, the script asserts zero leftovers matching the acceptance name
  prefix (`mlgcpwt` by default) for VPC, GKE, Cloud SQL, Memorystore, and secrets.
- Never interrupt mid-create/destroy.
- Serial builds only (`GOMAXPROCS=1`) — do not spawn parallel `go build` from the
  harness (see `AGENTS.md`).
- When another agent works on AWS in the main checkout, run this from the GCP
  worktree (`magelift-gcp`) and keep the `mlgcpwt` prefix so resource names and
  `/tmp` workdirs do not collide.

## Invoke

```bash
# From the GCP worktree (recommended when AWS work is in progress elsewhere)
cd /home/alex/workspace/magelift-gcp

# Preview graph (default)
MAGELIFT_GCP_ACCEPTANCE=1 ./scripts/gcp-acceptance-local.sh preview

# Short-lived real stack + live_cell_loop (destroy on EXIT)
MAGELIFT_GCP_ACCEPTANCE=1 \
MAGELIFT_GCP_ACCEPTANCE_DIGEST='ghcr.io/acourtiol/magento@sha256:…' \
./scripts/gcp-acceptance-local.sh up
```

Resume after a mid-matrix kill (stack kept with KEEP):

```bash
MAGELIFT_GCP_ACCEPTANCE=1 \
MAGELIFT_GCP_ACCEPTANCE_KEEP=true \
MAGELIFT_GCP_ACCEPTANCE_RESUME=1 \
MAGELIFT_GCP_ACCEPTANCE_DIGEST='ghcr.io/…@sha256:…' \
./scripts/gcp-acceptance-local.sh up
```

Optional env:

| Variable | Default | Purpose |
| --- | --- | --- |
| `MAGELIFT_GCP_PROJECT` | `digital-lab-341608` | GCP project |
| `MAGELIFT_GCP_REGION` | `europe-west1` | Region |
| `MAGELIFT_GCP_ACCEPTANCE_NAME` | `mlgcpwt` | Magento project name / resource prefix |
| `MAGELIFT_GCP_ACCEPTANCE_DIGEST` | placeholder digest | **Must be pullable** for Magento cells |
| `MAGELIFT_GCP_ACCEPTANCE_DIR` | `/tmp/magelift-gcp-wt` | Work dir + logs |
| `MAGELIFT_GCP_ACCEPTANCE_RESUME` | unset | Skip create-once when stack already up |
| `MAGELIFT_GCP_WIF_ACT_LOG` | unset | Act smoke log proving token exchange |
| `MAGELIFT_GCP_WIF_ACT_PROOF` | unset | Operator-attested Act proof (`=1`) |
| `MAGELIFT_DUMPIMPORT_RUNNER` | unset | Set `kube` for private Cloud SQL dump cell |
| `GOOGLE_APPLICATION_CREDENTIALS` | auto from gcloud login | Preferred; refreshable ADC |
| `GOOGLE_OAUTH_ACCESS_TOKEN` | avoided when ADC exists | Static token; expires mid-Up |

Logs: `/tmp/magelift-gcp-wt/logs/*.log` — grep those files; do not rely on
shell scrollback.

Auth note: long Ups must use Application Default Credentials (refreshable).
A static `GOOGLE_OAUTH_ACCESS_TOKEN` expires (~40–60m) and fails GKE/Memorystore
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
TOKEN=$(gcloud auth print-access-token)
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "https://compute.googleapis.com/compute/v1/projects/digital-lab-341608/global/networks/mlgcpwt-preview-net/removePeering" \
  -d '{"name":"servicenetworking-googleapis-com"}'
gcloud compute addresses delete mlgcpwt-preview-sql-psa --global --project=digital-lab-341608
gcloud compute networks delete mlgcpwt-preview-net --project=digital-lab-341608
```

## Credentials

- `gcloud` must be authenticated to the target project.
- Pulumi needs a backend (`pulumi login` or `PULUMI_BACKEND_URL`). Ephemeral agent
  accounts from Automation API must be claimed by a human if used.
- Prefer Application Default Credentials; the script falls back to
  `GOOGLE_OAUTH_ACCESS_TOKEN` from `gcloud auth print-access-token`.
- Cloudflare DNS cell needs `CLOUDFLARE_API_TOKEN` / `CF_API_TOKEN` with
  Zone.DNS Edit (Wrangler OAuth is insufficient).

## Known gaps (experimental)

- Memorystore Valkey needs a regional Service Connection Policy
  (`serviceClass=gcp-memorystore`) — created by the GCP cache component.
- Magento Ops cells need a pullable digest before `TierCertified`.
- GitHub WIF bootstrap is implemented offline (pool/provider/CI SA). CI proof is
  **Act-only** until hosted Actions minutes return — see
  [gcp-experimental.md](gcp-experimental.md#github-wif-act-only-until-minutes-return)
  and `.github/workflows/gcp-wif-act-smoke.yml`. Do not commit SA keys.
- `env import-dump` still defaults to host/compose unless
  `MAGELIFT_DUMPIMPORT_RUNNER=kube` is honored by the CLI for private Cloud SQL
  (07-04 runner exists; live wiring exercised in 07-06).
