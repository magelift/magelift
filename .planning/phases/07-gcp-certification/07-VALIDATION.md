---
phase: 07
slug: gcp-certification
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-30
---

# Phase 7 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Sourced from `07-RESEARCH.md` Validation Architecture + GSD VALIDATION template.
> Constraints: offline unit/harness dry-run first; single paid create-once in 07-06 only; serial `GOMAXPROCS=1 GOFLAGS=-p=1` (AGENTS.md).

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + bash acceptance harness |
| **Config file** | none — Makefile / scripts |
| **Quick run command** | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/gcp/... ./internal/dumpimport/... ./internal/cli/ -count=1 -short` |
| **Full suite command** | `MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash tests/acceptance/gcp_harness_shape_test.sh` && serial `go test ./internal/cloud/gcp/... ./internal/dumpimport/...` && (live, 07-06 only) `MAGELIFT_GCP_ACCEPTANCE=1 ./scripts/gcp-acceptance-local.sh up` |
| **Estimated runtime** | ~60–180s quick; ~3–8m full offline; ~25–40m live create+soak |

---

## Sampling Rate

- **After every task commit:** serial package tests for touched packages (`GOMAXPROCS=1`)
- **After every plan wave:** dry-run harness shape + serial gcp/dumpimport tests
- **Before `/gsd-verify-work`:** live matrix PASS rows + docs certified (after 07-07)
- **Max feedback latency:** 180s offline; abort under memory pressure (AGENTS.md)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 07-01-01 | 01 | 1 | GCP-01, GCP-02 | T-07-01, T-07-02 | WIF condition + no SA keys; SM Get loud | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/gcp/bootstrap/ ./internal/cloud/gcp/secrets/ ./internal/cli/ -count=1 -run 'WIF\|Identity\|Bootstrap\|Composer\|Secret\|GCP'` | ❌ W0 | ⬜ pending |
| 07-01-02 | 01 | 1 | GCP-01 | T-07-02 | Act WIF without credentials_json | docs/workflow | `rg` Act workflow + no private key | ❌ W0 | ⬜ pending |
| 07-02-01 | 02 | 2 | GCP-05 | T-07-05 | Account-free Notice; Estimated non-empty | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/gcp/cost/ ./internal/cloud/gcp/ops/ -count=1 -run 'Cost\|Estimate'` | ❌ W0 | ⬜ pending |
| 07-02-02 | 02 | 2 | GCP-05 | T-07-05 | unsupportedCost removed | unit | `go test ./internal/cloud/gcp/cost/ ./internal/cloud/gcp/ops/ -count=1` && `! rg unsupportedCost day2.go` | ❌ W0 | ⬜ pending |
| 07-04-01 | 04 | 1 | MIGRATE-04 | T-07-07 | Kube runner; no silent import success | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/dumpimport/ -count=1 -run 'Kube\|Runner\|Import\|Tiny'` | ❌ W0 | ⬜ pending |
| 07-04-02 | 04 | 1 | MIGRATE-04 | T-07-08 | Full dumpimport package | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/dumpimport/ -count=1` | ⚠️ partial | ⬜ pending |
| 07-05-01 | 05 | 1 | MIGRATE-04 | T-07-09 | DNS dry-run; Zone.DNS token docs | shell | `./scripts/cutover-dns-cloudflare.sh --dry-run` | ❌ W0 | ⬜ pending |
| 07-05-02 | 05 | 1 | MIGRATE-04 | T-07-10 | Mocked curl self-test | shell | `bash scripts/acceptance/cutover-dns-cloudflare_test.sh` | ❌ W0 | ⬜ pending |
| 07-03-01 | 03 | 3 | GCP-03..05 | T-07-11 | Dry-run cell loop; no accidental up | harness | `MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash tests/acceptance/gcp_harness_shape_test.sh` | ⚠️ partial | ⬜ pending |
| 07-03-02 | 03 | 3 | GCP-03..05 | T-07-12 | Checkpoint + append_row | harness | dry-run shape + `rg` checkpoint/evidence | ⚠️ partial | ⬜ pending |
| 07-06-01 | 06 | 4 | E1–E3 | T-07-15 | ADC + CF token + destroy-when-done | checkpoint | human-action prereqs | — | ⬜ pending |
| 07-06-02 | 06 | 4 | D-02 | T-07-13 | Authorize single paid create-once | checkpoint | decision option-a | — | ⬜ pending |
| 07-06-03 | 06 | 4 | GCP-01..05, MIGRATE-04 | T-07-13 | Live pass + force_clean | live harness | matrix PASS + scratch force_clean | ❌ live | ⬜ pending |
| 07-07-01 | 07 | 5 | GCP-06 | T-07-17 | Certify only with PASS evidence | docs gate | `rg PASS matrix` && certified docs | ❌ | ⬜ pending |
| 07-07-02 | 07 | 5 | GCP-06, MIGRATE-04 | T-07-17 | REQUIREMENTS/STATE close | docs | `rg` Complete + STATE | ❌ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| GCP-01 | WIF plan/Ensure + Act/STS | unit + workflow | `go test ./internal/cloud/gcp/bootstrap/ -run WIF` | ❌ Wave 0 |
| GCP-02 | Composer SM + loud secrets | unit | `go test ./internal/cli/ -run Composer` + secrets tests | ⚠️ partial |
| GCP-03 | Day-2 live cells | harness live | gcp acceptance day2 cells | ❌ live cells |
| GCP-04 | Deploy sequence on GKE | harness live | deploy:candidate cell | ❌ |
| GCP-05 | Cost per-cell estimate | unit + live | `go test ./internal/cloud/gcp/cost/` | ❌ Wave 0 |
| GCP-06 | Certified docs | docs gate | evidence + capability-matrix | ❌ |
| MIGRATE-04 DNS | Upsert/delete script | shell | cutover `--dry-run` + mock test | ❌ |
| MIGRATE-04 dump | Journal imported | unit + live | dumpimport kube + migrate:dump cell | ❌ managed path |
| force_clean | assert_clean after soak | harness EXIT | existing script + 07-06 scratch | ✅ shape offline |

---

## Wave 0 Requirements

- [ ] `internal/cloud/gcp/bootstrap` WIF unit tests (fake IAM/WIF client) — 07-01
- [ ] `internal/cloud/gcp/secrets` AccessSecretVersion + CLI Composer GCP tests — 07-01
- [ ] `internal/cloud/gcp/cost/` Estimator + tests — 07-02
- [ ] Expand `scripts/acceptance/cells-gcp-preview.txt` + live_cell_loop — 07-03
- [ ] dumpimport kube/managed runner tests — 07-04
- [ ] `scripts/cutover-dns-cloudflare.sh` + mocked shell test — 07-05
- [ ] Evidence gate before GCP-06 docs flip — 07-07

*Existing infrastructure:* Go test + `gcp-acceptance-local.sh` dry-run + force_clean helpers already present; Wave 0 is new/expanded tests for WIF/SM/cost/dump/DNS/cells, not framework install.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| gcloud ADC refresh | E1 / 07-06 | Interactive browser login | `gcloud auth application-default login` then print-access-token |
| Cloudflare Zone.DNS Edit token | E2 / 07-06 | Operator-held secret | Export CLOUDFLARE_API_TOKEN; optional script self-test |
| Destroy-when-done approval | E3 / D-02 | Spend consent | Confirm KEEP=false before up |
| Clean account after force_clean | D-02 | Visual/account check | Human-verify checkpoint in 07-06 |

Offline plan behaviors (07-01..05) have automated verification. Live certification requires the 07-06 checkpoints above.

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 / checkpoint dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency under memory-safe serial runs (AGENTS.md)
- [ ] Offline plans before paid 07-06
- [ ] `nyquist_compliant: true` set in frontmatter after validate-phase

**Approval:** pending
