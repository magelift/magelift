# Experimental GCP acceptance (local, destroy-on-exit)

Status: **experimental**. Not Magento-certified. Prefer
[aws-acceptance.md](aws-acceptance.md) for the certified path.

## Safety rules

- Set `MAGELIFT_GCP_ACCEPTANCE=1` or the script refuses to run.
- Default mode is `preview` (Pulumi plan only).
- `up` creates real GKE Autopilot, Cloud SQL, and Memorystore Valkey — costly and
  slow. Destroy always runs on EXIT unless `MAGELIFT_GCP_ACCEPTANCE_KEEP=true`.
- After destroy, the script asserts zero leftovers matching the acceptance name
  prefix (`mlgcpwt` by default) for VPC, GKE, Cloud SQL, Memorystore, and secrets.
- Never interrupt mid-create/destroy.
- When another agent works on AWS in the main checkout, run this from the GCP
  worktree (`magelift-gcp`) and keep the `mlgcpwt` prefix so resource names and
  `/tmp` workdirs do not collide.

## Invoke

```bash
# From the GCP worktree (recommended when AWS work is in progress elsewhere)
cd /home/alex/workspace/magelift-gcp

# Preview graph (default)
MAGELIFT_GCP_ACCEPTANCE=1 ./scripts/gcp-acceptance-local.sh preview

# Short-lived real stack (destroy on EXIT)
MAGELIFT_GCP_ACCEPTANCE=1 ./scripts/gcp-acceptance-local.sh up
```

Optional env:

| Variable | Default | Purpose |
| --- | --- | --- |
| `MAGELIFT_GCP_PROJECT` | `digital-lab-341608` | GCP project |
| `MAGELIFT_GCP_REGION` | `europe-west1` | Region |
| `MAGELIFT_GCP_ACCEPTANCE_NAME` | `mlgcpwt` | Magento project name / resource prefix |
| `MAGELIFT_GCP_ACCEPTANCE_DIGEST` | placeholder digest | OCI image reference |
| `MAGELIFT_GCP_ACCEPTANCE_DIR` | `/tmp/magelift-gcp-wt` | Work dir + logs |
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

## Known gaps (experimental)

- Memorystore Valkey needs a regional Service Connection Policy
  (`serviceClass=gcp-memorystore`) — created by the GCP cache component.
- Magento Ops and day-2 ports are implemented; full Magento acceptance with a
  real digest is still required before `TierCertified`.
- GitHub WIF identity on bootstrap is deferred (GCS DIY state bucket only).
