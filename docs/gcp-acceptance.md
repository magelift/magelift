# Experimental GCP acceptance (local, destroy-on-exit)

Status: **experimental**. Not Magento-certified. Prefer
[aws-acceptance.md](aws-acceptance.md) for the certified path.

## Safety rules

- Set `MAGELIFT_GCP_ACCEPTANCE=1` or the script refuses to run.
- Default mode is `preview` (Pulumi plan only).
- `up` creates real GKE Autopilot, Cloud SQL, and Memorystore Valkey — costly and
  slow. Destroy always runs on EXIT unless `MAGELIFT_GCP_ACCEPTANCE_KEEP=true`.
- After destroy, the script asserts zero leftovers matching the acceptance name
  prefix (`mlacc` by default) for VPC, GKE, Cloud SQL, Memorystore, and secrets.
- Never interrupt mid-create/destroy.

## Invoke

```bash
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
| `MAGELIFT_GCP_ACCEPTANCE_NAME` | `mlacc` | Magento project name / resource prefix |
| `MAGELIFT_GCP_ACCEPTANCE_DIGEST` | placeholder digest | OCI image reference |
| `MAGELIFT_GCP_ACCEPTANCE_DIR` | `/tmp/magelift-gcp-acceptance` | Work dir + logs |
| `GOOGLE_OAUTH_ACCESS_TOKEN` | from `gcloud auth print-access-token` | Pulumi GCP creds |

Logs: `/tmp/magelift-gcp-acceptance/logs/*.log` — grep those files; do not rely on
shell scrollback.

## Force cleanup after a failed destroy

Cloud SQL soft-delete can block Service Networking peering removal for a few
minutes. The EXIT trap retries peering delete, then removes NAT/router/subnets,
the PSA address, and the VPC. If assert_clean still fails, re-run the trap helper
manually or wait and delete:

```bash
gcloud services vpc-peerings delete \
  --network=mlacc-preview-net \
  --service=servicenetworking.googleapis.com \
  --project=digital-lab-341608
gcloud compute networks delete mlacc-preview-net --project=digital-lab-341608
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
- First real `up` without that policy failed; code now creates the policy before
  the Valkey instance.
