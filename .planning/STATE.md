---
gsd_state_version: 1.0
milestone: v1.0.0
milestone_name: milestone
current_phase: 7
current_phase_name: GCP Certification
status: executing
stopped_at: Completed 07-04-PLAN.md
last_updated: "2026-07-30T11:44:35.805Z"
last_activity: 2026-07-30
last_activity_desc: completed 07-05 Cloudflare DNS cutover script (offline)
progress:
  total_phases: 8
  completed_phases: 3
  total_plans: 49
  completed_plans: 41
  percent: 38
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-07-27)

**Core value:** A Magento team with no dedicated devops resource can deploy and operate production Magento on their own cloud account, at a cost they control, using a CLI and YAML that already feel familiar.

**Current focus:** Phase 7 GCP Certification — 07-01, 07-04, 07-05 complete; next incomplete 07-02

## Current Position

Phase: 7 of 8 (GCP Certification)
Plan: 4 of 7 (next incomplete: 07-02; 07-04 + 07-05 done out of wave order)
Status: Ready to execute
Last activity: 2026-07-30 — completed 07-04 kube-adjacent dumpimport runner (offline)

Progress: [████████░░] 84%

## Session Continuity

**Last session:** 2026-07-30T11:44:35.797Z
**Stopped at:** Completed 07-04-PLAN.md
**Resume file:** None

- Phase 4: verified 5/5 — ACC/Upsun import + ece-parity
- Phase 5: verified 5/5 — seedDump real, media-sync, cutover runbook (DNS/live → Phase 7)
- Phase 6: verified 5/5 — shared `*kube.Observe`/`*kube.Steps`, OVH/SCW AES256 State, honesty matrix; Floci AES256 optional; DNS/live → Phase 7 HUMAN_GATE
- Phase 7: 07-01 (WIF + Composer SM) + 07-04 (kube dump runner offline) + 07-05 (DNS cutover script offline) complete; MIGRATE-04 live rehearsal still HUMAN_GATE

## Performance Metrics

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 06 P01 | 6min | 2 tasks | 26 files |
| Phase 06 P02 | 5min | 2 tasks | 12 files |
| Phase 06 P03 | 7min | 2 tasks | 13 files |
| Phase 06 P04 | 6min | 2 tasks | 13 files |
| Phase 06 P05 | 4min | 2 tasks | 11 files |
| Phase 06 P06 | 5min | 2 tasks | 4 files |
| Phase 07-gcp-certification P01 | 5min | 2 tasks | 13 files |
| Phase 07 P05 | 3min | 2 tasks | 4 files |
| Phase 07 P04 | 4min | 2 tasks | 6 files |

## Decisions

- [Phase 6]: ObjectEncryption modes: KMS for AWS, AES256 for OVH/SCW/Floci, None only as Floci SSE-reject fallback
- [Phase 6]: OVH/SCW State requires stateBucket; endpoint via stateEndpoint or MAGELIFT_AWS_ENDPOINT_URL with Parse loopback gate
- [Phase 6]: OutputKubeconfig = kubeconfig; optional, not in RequiredOutputKeys (ECS free)
- [Phase 6]: Plumb Runtime.Kubeconfig as Pulumi ToSecret then export via stack Outputs
- [Phase 6]: EKS exports existing exec-plugin kubeconfig as-is (D-02 escape hatch deferred)
- [Phase ?]: Shared *kube.Observe for all four K8s modules (D-01); kubectl PrepareExec; CLI launcher=binary
- [Phase ?]: Shared *kube.Steps for all four K8s modules (D-03); portable DeploySpec
- [Phase ?]: Default Steps Job/Runtime via ClientFromOutputs, not GKE ADC
- [Phase ?]: Remove State methods from unsupported{}; type system proves State is not the shell
- [Phase ?]: Delete diyLockWarnOut on OVH/SCW/EKS; AcquireLock uses State.Lock
- [Phase ?]: Skip release-readiness.md — no Phase 6 gate row (D-06)
- [Phase ?]: identity_test.go uses package kube_test to import four providers without cycles
- [Phase ?]: Floci AES256 skipped this session (docker ps failed); unit AES256 is offline floor
- [Phase ?]: Phase 7 DNS: magelift-preview.alexandrecourtiol.com needs Zone.DNS Edit; Wrangler OAuth insufficient
- [Phase ?]: WIF attribute condition locked to assertion.repository == 'acourtiol/magelift'
- [Phase ?]: Bootstrap Details.wif is structured map never deferred; CI proof Act-only until minutes return
- [Phase ?]: DNS cutover: CLOUDFLARE_API_TOKEN/CF_API_TOKEN Bearer; dry-run skips token; CURL_BIN mock offline
- [Phase ?]: Cutover record type: hostname→CNAME, IPv4→A; TTL 120; proxied false unless --proxied
- [Phase ?]: Prefer kubectl exec piping SQL over Cloud SQL Auth Proxy+IAP for managed dump connectivity
- [Phase ?]: Password via MYSQL_PWD env arg to in-pod mysql — never -pPASSWORD (T-07-08)
- [Phase ?]: Do not mark MIGRATE-04 Complete — live evidence is 07-07
