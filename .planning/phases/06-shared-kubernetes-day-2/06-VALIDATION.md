---
phase: 06
slug: shared-kubernetes-day-2
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-30
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Sourced from `06-RESEARCH.md` Validation Architecture + GSD VALIDATION template.
> Constraints: offline fake clientset + Floci only; no live GCP; serial `GOMAXPROCS=1 GOFLAGS=-p=1`.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + race (`make test`); Floci build tag `floci` |
| **Config file** | none (Makefile `test` target); Floci via `docker-compose.floci.yml` / `make floci-test` |
| **Quick run command** | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/... ./internal/cloud/aws/state/... -count=1` |
| **Full suite command** | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/... ./internal/cloud/gcp/ops/... ./internal/cloud/aws/eksops/... ./internal/cloud/ovh/... ./internal/cloud/scaleway/... -count=1` then `make floci-test` for KUBE-05 |
| **Estimated runtime** | ~60–180s quick; ~3–8m full + Floci |

---

## Sampling Rate

- **After every task commit:** Run quick command on touched packages (serial, `GOMAXPROCS=1`)
- **After every plan wave:** Run full suite command above
- **Before `/gsd-verify-work`:** Full suite green + `make floci-test` for KUBE-05
- **Max feedback latency:** 180 seconds for quick; abort under memory pressure (AGENTS.md)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 06-01-01 | 01 | 1 | KUBE-05 | T-06-01 | endpoint.Parse loopback; KMS vs AES256 policy | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/state/ -count=1 -run 'Encryption\|AES256\|Manager\|Lock'` | ❌ W0 | ⬜ pending |
| 06-01-02 | 01 | 1 | KUBE-05 | T-06-01 | OVH/SCW State via shared manager + Floci | unit + Floci | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ -count=1 -run 'State\|AES256'` + `make floci-test` when available | ❌ W0 | ⬜ pending |
| 06-02-01 | 02 | 1 | KUBE-01..04 | T-06-04 | kubeconfig secret; ClientFromKubeconfig | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/ -count=1 -run 'ClientFrom\|Kubeconfig'` | ❌ W0 | ⬜ pending |
| 06-02-02 | 02 | 1 | KUBE-01..04 | T-06-04 | four stacks export secret kubeconfig | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/gcp/stack/ ./internal/cloud/aws/eksops/ ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ -count=1 -run 'Output\|Kubeconfig'` | ⚠️ partial | ⬜ pending |
| 06-03-01 | 03 | 2 | KUBE-01 | T-06-07 | TailLogs via fake clientset | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/ -count=1 -run 'Observe\|TailLogs'` | ❌ W0 | ⬜ pending |
| 06-03-02 | 03 | 2 | KUBE-02, KUBE-03 | T-06-06 | CheckRuntime + PrepareExec kubectl + type-identity | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/ ./internal/cloud/gcp/ops/ ./internal/cloud/aws/eksops/ ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ -count=1 -run 'CheckRuntime\|PrepareExec\|TypeIdentity'` | ❌ W0 | ⬜ pending |
| 06-04-01 | 04 | 3 | KUBE-04 | T-06-09 | Steps full sequence on fake JobAPI | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/ -count=1 -run 'StepsSequence'` | ❌ W0 | ⬜ pending |
| 06-04-02 | 04 | 3 | KUBE-04 | T-06-09 | four NewDeploySteps → *kube.Steps | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/ ./internal/cloud/gcp/ops/ ./internal/cloud/aws/eksops/ ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ -count=1 -run 'NewDeploySteps\|TypeIdentity\|Steps'` | ❌ W0 | ⬜ pending |
| 06-05-01 | 05 | 4 | KUBE-06, KUBE-07 | T-06-13 | unsupported allowlist + AcquireLock flip | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ -count=1 -run 'Unsupported\|Allowlist\|AcquireLock'` | ⚠️ partial | ⬜ pending |
| 06-05-02 | 05 | 4 | KUBE-07 | T-06-12 | matrix/docs honesty | docs + unit | `rg` docs + allowlist tests | ❌ | ⬜ pending |
| 06-06-01 | 06 | 5 | KUBE-01..04 | T-06-16 | cross-module identity + Steps | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube/... ./internal/cloud/gcp/ops/ ./internal/cloud/aws/eksops/ ./internal/cloud/ovh/... ./internal/cloud/scaleway/... -count=1 -run 'TypeIdentity\|StepsSequence'` | ❌ W0 | ⬜ pending |
| 06-06-02 | 06 | 5 | KUBE-05 + handoff | T-06-15 | Floci cite + Phase 7 DNS handoff | file + unit | handoff `rg` + AES256 tests | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| KUBE-01 | TailLogs via shared Observe | unit (fake pods) | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube -run TestObserveTailLogs -count=1` | ❌ Wave 0 |
| KUBE-02 | CheckRuntime via shared Observe | unit (fake Deployment) | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube -run TestObserveCheckRuntime -count=1` | ❌ Wave 0 |
| KUBE-03 | PrepareExec portable kubectl target | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube -run TestObservePrepareExec -count=1` | ❌ Wave 0 |
| KUBE-04 | Steps full sequence on fake | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube -run TestStepsSequence -count=1` | ❌ Wave 0 |
| KUBE-01..04 | Four modules return `*kube.Observe` / `*kube.Steps` | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/kube -run TestTypeIdentity -count=1` | ❌ Wave 0 |
| KUBE-05 | Lock/backup/restore AES256 + endpoint | unit + Floci | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/state -run TestEncryptionAES256 -count=1`; `make floci-test` | ⚠️ Floci KMS path exists; AES256 path ❌ |
| KUBE-06 | Bootstrap/Secrets never nil-success | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/ovh/stack ./internal/cloud/scaleway/stack -run TestUnsupported -count=1` | ✅ partial (extend) |
| KUBE-07 | Grep/allowlist remaining ErrNotSupported | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/ovh/stack -run TestRemainingUnsupportedAllowlist -count=1` | ❌ Wave 0 |
| SC5 | Matrix docs mention day-2 surface | docs gate | `rg` in plan 06-05 verify | ❌ |

---

## Wave 0 Requirements

- [ ] `internal/cloud/kube/observe_test.go` — KUBE-01..03 (created in 06-03)
- [ ] `internal/cloud/kube/steps_test.go` — KUBE-04 sequence (created in 06-04)
- [ ] `internal/cloud/kube/identity_test.go` — type-identity (created in 06-06; stubs may land earlier)
- [ ] `internal/cloud/aws/state` AES256 / ObjectEncryption tests — KUBE-05 (created in 06-01)
- [ ] OVH/SCW remaining-unsupported allowlist tests — KUBE-07 (created in 06-05)
- [ ] `tests/floci/state_aes256_test.go` — AES256 lock path (created in 06-01)
- [ ] Prefer `fake.NewClientset` only — do not add envtest / setup-envtest

*Existing infrastructure:* Go test + Floci compose already present; Wave 0 is new test files for shared kube + AES256, not framework install.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live GKE day-2 | Phase 7 | D-06 forbids Phase 6 cloud spend | Deferred — do not run in Phase 6 |
| Cloudflare DNS cutover | MIGRATE-04 / Phase 7 | Needs Zone.DNS Edit token; not Phase 6 | See `06-PHASE7-HANDOFF.md` after 06-06 |

All Phase 6 in-scope behaviors have automated offline verification (unit and/or Floci).

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency under memory-safe serial runs (AGENTS.md)
- [ ] `nyquist_compliant: true` set in frontmatter after validate-phase

**Approval:** pending
