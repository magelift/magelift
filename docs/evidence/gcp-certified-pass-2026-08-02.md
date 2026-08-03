# GCP GKE Autopilot certified live pass

Date: 2026-08-02  
Project: `digital-lab-341608` / `europe-west1` / prefix `mlgcpwt`  
Digest: `europe-west1-docker.pkg.dev/digital-lab-341608/magelift/acceptance@sha256:df04554d1bde2f7b754360fa2f70bb3e7cf8a13ffbf429e5891937e6e3b7e5e4`  
Backend: `gs://magelift-digital-lab-341608-europe-west1-mlgcpwt-preview-state` (deleted after teardown)  
DNS host: `magelift-preview.alexandrecourtiol.com` (cf CLI `dns_records:edit`)

## Cell results (checkpoint final)

| cell | result |
|------|--------|
| bootstrap:wif | PASS (Ensure + SA impersonation TokenCreator) |
| composer:sm-write | PASS |
| composer:sm-read | PASS |
| day2:secrets | PASS |
| day2:state | PASS |
| day2:logs | PASS (BindOutputs + serviceName label) |
| day2:exec | PASS (kubectl + --kubeconfig temp, no -t) |
| day2:health | PASS |
| deploy:candidate | PASS |
| migrate:dump | PASS (journal init + mysql:8 client pod + RunnerKube) |
| cost:estimate | PASS (account-free estimator) |
| cutover:dns | PASS (LB IP via kubectl fallback + cf CLI) |

Evidence: committed sample [gcp-matrix-results-2026-08-02.md](gcp-matrix-results-2026-08-02.md)
(harness `append_row`; FAIL rows are mid-pass resumes, final PASS present for each
required cell). Local re-runs also write under gitignored `.magelift/gcp-matrix/`.

## Fixes landed during live pass

1. Inverted GCP Composer secretref validation (`internal/config/config.go`)
2. GCP harness checkpoint path no longer inherits AWS `.magelift/acceptance-checkpoint.json`
3. Default `PULUMI_BACKEND_URL` to GCS DIY bucket (avoid broken S3 login)
4. `preview_reports_stale_state` treats preview failure as clean, not stale
5. Create-once is `--infra-only`; Magento via `deploy:candidate`
6. Secret stack outputs returned decrypted for day-2 (`automation.Outputs`); CLI redacts
7. `TailLogs` BindOutputs + serviceName; KEEP on cell FAIL; skip assert_clean when KEEP
8. CLI honors `MAGELIFT_DUMPIMPORT_*` for RunnerKube
9. cutover:dns falls back to Service LB IP when `applicationURL` empty

## Teardown (complete)

| Step | Result |
|------|--------|
| EXIT destroy + PSA lag | destroy stalled on producer services; force_clean continued |
| `force_clean_orphans` + PSA soak | producers gone; network deleted |
| `assert_clean` | **ok** (exit 0) |
| DNS `--cleanup` | `magelift-preview.alexandrecourtiol.com` deleted via cf |
| Pulumi stack rm | `mlgcpwt-preview-gcp-gke-autopilot` removed |
| State bucket | `gs://…-mlgcpwt-preview-state` emptied and deleted |
| Post-check | zero `mlgcpwt` VPC/GKE/SQL/Valkey leftovers |

Teardown completed with `TEARDOWN_DONE` on the maintainer machine (local log retained offline).
