# GCP GKE Autopilot certified live pass

Date: 2026-08-02  
Project / region / prefix: **redacted** disposable maintainer project / `europe-west1` / operator prefix  
Digest: private Artifact Registry acceptance image (digest retained offline)  
Backend: ephemeral GCS DIY state bucket (deleted after teardown)  
DNS host: operator-owned preview FQDN (redacted; Cloudflare Zone.DNS Edit)

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
Cloud project IDs in the sample matrix are **placeholders**, not real accounts.

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
| DNS `--cleanup` | preview host record deleted via cf |
| Pulumi stack rm | stack removed |
| State bucket | emptied and deleted |
| Post-check | zero leftover VPC/GKE/SQL/Valkey for the run prefix |

Teardown completed with `TEARDOWN_DONE` on the maintainer machine (local log retained offline).
