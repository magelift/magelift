---
phase: 08
slug: brownfield-attach-tag-day
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-30
---

# Phase 8 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + race (`make test` → `go test -race ./...`) |
| **Config file** | none — standard Go |
| **Quick run command** | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/network ./internal/cloud/aws/database ./internal/cloud/aws/stack ./internal/cli -count=1 -run 'Existing\|Adopt\|Refuse\|Detach\|PlanFromConfig'` |
| **Full suite command** | `GOMAXPROCS=1 GOFLAGS=-p=1 make test` (abort if swap climbs; AGENTS.md) |
| **Estimated runtime** | ~60–180s quick; full suite longer |

---

## Sampling Rate

- **After every task commit:** Run package-scoped quick command for touched packages
- **After every plan wave:** Run quick command across network+database+stack+cli
- **Before `/gsd-verify-work`:** Full serial `make test` must be green (or documented skip under memory pressure)
- **Max feedback latency:** 180 seconds for quick map

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|----------------|-----------------|-----------|-------------------|-------------|--------|
| 08-01-01 | 01 | 1 | ATTACH-01, ATTACH-03 | T-08-01 | Refuse before mutate names VPC | unit | `go test ./internal/cloud/aws/stack/ -run 'Adopt\|Refuse' -count=1` | ❌ W0 | ⬜ pending |
| 08-01-02 | 01 | 1 | ATTACH-01 | T-08-02 | Zero VPC creates on Existing | unit | `go test ./internal/cloud/aws/network/ -run ExistingNetwork -count=1` | ✅ | ⬜ pending |
| 08-02-01 | 02 | 1 | ATTACH-02 | T-08-04 | Secret ARN only in YAML | unit+generate | `go generate ./internal/config && go test ./internal/config/ -count=1` | ✅ model | ⬜ pending |
| 08-02-02 | 02 | 1 | ATTACH-02 | T-08-05 | Incomplete DB ref rejected | unit | `go test ./internal/cloud/aws/stack/ -run ExistingDatabase -count=1` | ❌ W0 | ⬜ pending |
| 08-03-01 | 03 | 2 | ATTACH-02 | T-08-08 | Zero RDS creates on Existing | unit/mock | `go test ./internal/cloud/aws/database/ -run Existing -count=1` | ❌ W0 | ⬜ pending |
| 08-03-02 | 03 | 2 | ATTACH-02 | T-08-07 | Execution role single secret ARN | unit | `go test ./internal/cloud/aws/stack/ -run ExistingDatabase -count=1` | ❌ W0 | ⬜ pending |
| 08-04-01 | 04 | 3 | ATTACH-03 | T-08-11 | ADOPT network+DB in preview | unit | `go test ./internal/cloud/aws/stack/ ./internal/cli/ -run 'Adopt\|Preview' -count=1` | ❌ W0 | ⬜ pending |
| 08-04-02 | 04 | 3 | ATTACH-03 | T-08-10 | Refuse names DB+network | unit | `go test ./internal/cloud/aws/stack/ -run 'Adopted\|Refuse\|Detach' -count=1` | ❌ W0 | ⬜ pending |
| 08-05-01 | 05 | 4 | ATTACH-04 | T-08-13 | Detach docs present | docs gate | `test -f docs/brownfield-attach.md` | ❌ W0 | ⬜ pending |
| 08-05-02 | 05 | 4 | ATTACH-04 | T-08-13 | Offline proof cited | unit+docs | `go test ./internal/cloud/aws/stack/ -run 'Detach\|Refuse' -count=1` | ✅ adopt_test after 04 | ⬜ pending |
| 08-06-01 | 06 | 5 | D-04 | T-08-15 | Offline evidence recorded | unit+scratch | quick map + evidence file | ❌ W0 | ⬜ pending |
| 08-06-02 | 06 | 5 | D-04 | T-08-17 | Paid AWS confirm or ADC gate | human | `aws sts get-caller-identity` + confirm file | ❌ W0 | ⬜ pending |
| 08-06-03 | 06 | 5 | RELEASE-05 | T-08-16 | Board Closed/Deferred/Pending→07 | audit | release-readiness + REQUIREMENTS rg gates | ✅ docs | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/cloud/aws/stack/adopt_test.go` — AdoptReport + RefuseAdoptedMutation + detach (08-01/04)
- [ ] `internal/cloud/aws/stack/config_test.go` — ExistingDatabase PlanFromConfig cases (08-02)
- [ ] `internal/cloud/aws/database/database_test.go` — Existing branch zero-RDS mock (08-03)
- [ ] CLI lifecycle adopt preview tests (08-01/04)
- [ ] `docs/brownfield-attach.md` — created in 08-05
- [ ] Optional Floci smoke only after mocks green (not required to close ATTACH offline)

*Existing infrastructure covers network Existing zero-create (`network_test.go`) and greenfield database mocks; Wave 0 fills adopt/DB Existing gaps.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Free-tier AWS VPC+RDS adopt + describe-after-destroy | D-04 / ATTACH-04 live half | Requires AWS ADC + real account spend | 08-06 HUMAN_GATE: preview/apply/destroy then `aws ec2 describe-vpcs` / `aws rds describe-db-instances` |
| GCP certify board Closed | RELEASE-05 / Phase 7 | Blocked on 07-06/07 live evidence | Leave Pending→07; do not close in Phase 8 |
| Hosted GitHub Actions green | RELEASE-05 | Minutes exhausted; Act-only | Deferred Act-only on board |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies (paid AWS is checkpoint + evidence file)
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency under 180s for quick map
- [ ] `nyquist_compliant: true` set in frontmatter after validate-phase

**Approval:** pending
