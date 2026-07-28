---
gsd_state_version: 1.0
milestone: v1.0.0
milestone_name: milestone
current_phase: 2
current_phase_name: Tag-Ready Release Surface
status: planning
stopped_at: Completed 02-01-PLAN.md
last_updated: "2026-07-28T14:44:49.348Z"
last_activity: 2026-07-28
last_activity_desc: Phase 2 PLAN.md files created
progress:
  total_phases: 8
  completed_phases: 1
  total_plans: 14
  completed_plans: 11
  percent: 13
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-07-27)

**Core value:** A Magento team with no dedicated devops resource can deploy and operate production Magento on their own cloud account, at a cost they control, using a CLI and YAML that already feel familiar.

**Current focus:** Phase 2 — Tag-Ready Release Surface

## Current Position

Phase: 2 of 8 (Tag-Ready Release Surface)
Plan: 02-01 of 02-04 (wave 1 next)
Status: Phase 2 plans written (`02-01`…`02-04`); ready for `/gsd-execute-phase 2`
Last activity: 2026-07-28 — Phase 2 PLAN.md files created

Progress: [████████░░] 79% (Phase 1 offline complete; Phase 2 planning)

## Performance Metrics

**Velocity:**

- Total plans completed: 0
- Average duration: —
- Total execution time: —

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| - | - | - | - |

**Recent Trend:**

- Last 5 plans: —
- Trend: —

*Updated after each plan completion*
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 01 P02 | 20 | 3 tasks | 4 files |
| Phase 01 P03 | 6 | 2 tasks | 2 files |
| Phase 01 P04 | 6 | 3 tasks | 8 files |
| Phase 01-publishable-baseline-honest-fallbacks P05 | 4min | 2 tasks | 4 files |
| Phase 01 P05 | 5 | 2 tasks | 4 files |
| Phase 01-publishable-baseline-honest-fallbacks P06 | 5min | 3 tasks | 3 files |
| Phase 01 P07 | 25min | 2 tasks | 2 files |
| Phase 01-publishable-baseline-honest-fallbacks P08 | 8min | 3 tasks | 8 files |
| Phase 01-publishable-baseline-honest-fallbacks P09 | 5min | 2 tasks | 7 files |
| Phase 01-publishable-baseline-honest-fallbacks P09 | 3min | 2 tasks | 6 files |
| Phase 01-publishable-baseline-honest-fallbacks P10 | 10min | 2 tasks | 15 files |
| Phase 02 P01 | 2min | 3 tasks | 7 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Roadmap: acceptance efficiency is an early deliverable (Phase 3), before any GCP spend — self-funded credits are the binding constraint
- Roadmap: three paid passes planned total — AWS harness proof (Phase 3), batched GCP certification (Phase 7), small attach confirmation (Phase 8)
- Roadmap: `v1.0.0-rc.1` is taggable after Phase 3; Phases 4-8 are droppable in reverse order
- Roadmap: Phases 4-5 (brownfield, offline) and 6-7 (shared Kubernetes + GCP) are independent tracks after Phase 3
- [Phase ?]: Offline 01-02: lint coverage guard shipped; CI race + force-all verify deferred-ci until Actions minutes return
- [Phase ?]: Full local go test -race ./... deferred-local under Cursor (16GB Mac); narrow race sample only
- [Phase ?]: 01-03: preserve cost not-supported prefix for plan 01-08 tier append
- [Phase ?]: 01-03: platform go test -race deferred-local under Cursor; non-race verified
- [Phase ?]: 01-04: compacted CI Resource:* statements so inline cleared 90% of PutRolePolicy quota
- [Phase ?]: 01-04: OCU step rule recorded as empirical/unpublished; 1700 ceiling marker only
- [Phase ?]: AWS carve: 16 slots at bits+4, max 5 zones; capacity check before preset AZ policy
- [Phase ?]: GCP carve: /20 → 8 indices (0-7); caller zone cap before Pulumi registration
- [Phase ?]: 01-05: AWS carve capacity before preset zone-count so 6+ zones fail isolation
- [Phase ?]: 01-05: GCP keeps caller zone-capacity and helper index errors separate
- [Phase ?]: 01-06: Match DependsOn URNs by substring via RegisterRPC; never registration order
- [Phase ?]: 01-06: Scaleway QUALITY-05 closed as documented non-carving invariant
- [Phase ?]: 01-06: Phase 6 ceiling — lift RegisterRPC DependsOn helper for Kapsule/GKE
- [Phase ?]: 01-07: preview×amazon-mq rejected via Spec.Validate 3-AZ naming 2-AZ guard
- [Phase ?]: 01-07: U3/U4 assert sidecar presence; DependsOn gap is Finding F-01-07-1 not a baseline fix
- [Phase ?]: 01-07: go test -race deferred-local under Cursor; non-race package verifies passed
- [Phase ?]: 01-07: preview×amazon-mq rejected via queue AZ guard (3 unique zones), not Spec.Validate
- [Phase ?]: 01-08: append tier after yet — costNotSupportedPrefix remains byte prefix
- [Phase ?]: 01-08: stubExperimentalModule (ovh.mks) for CLI tier tests without cloud imports
- [Phase ?]: 01-08: AcquireLock named AST-guard exception for plan 01-10
- [Phase ?]: 01-09: Warning keyed on CertificationTier at planStack, never provider allowlist
- [Phase ?]: 01-09: Deploy refuses Magento-less path unless --infra-only; announces when flag set
- [Phase ?]: 01-09: warn at planStack keyed on CertificationTier (includes aws/eks-autopilot)
- [Phase ?]: 01-09: Magento-less deploy refuses without --infra-only; announces when flagged
- [Phase ?]: Phase 1 plans complete offline; CI criterion 1 still deferred (HUMAN_GATE / act)
- [Phase ?]: 01-10: warn+noop AcquireLock via platform.WarnNoDIYLock (not ErrNotSupported)
- [Phase ?]: 01-10: AST ErrNotSupported walk covers unsupported only; AcquireLock via warning tests
- [Phase ?]: 01-10: F-01-07-1 DependsOn gap deferred untouched during runtime split
- [Phase ?]: First public tag is v1.0.0-rc.1 on README/versioning/gate board (D-01)
- [Phase ?]: RC locks sdk/v1 + StackModule core; Phase 6 may change kube-shaped platform ports without v2

### Pending Todos

None yet.

### Blockers/Concerns

- GitHub Actions free minutes exhausted 2026-07-28 — Phase 1 criterion 1 CI proof deferred; continue offline (see `.planning/loop/HUMAN_GATE`)
- Cloud budget: acceptance is self-funded on AWS and GCP; never enter Phase 7 before Phases 3, 5, 6 are green offline
- Unverifiable at any price: Aurora `CreateDBCluster` (free-tier API block), `amazon-mq` × `preview` (2-AZ vs 3-AZ), live OpenSearch SigV4 data plane (post-tag)
- Local machine: full multi-platform `goreleaser release` cannot run locally (kernel panics); serial single-target only, outside Cursor
- CI: golangci-lint OOM/timeout is a workaround, not a fix — worsens with every provider package added in Phase 6, so QUALITY-06 must land in Phase 1
- `go.yaml.in/yaml/v4` pinned to a release candidate; re-verify strict-decoding/merge/provenance tests on every bump
- GitHub Actions billing/spending limit: jobs refuse to start (annotation on run 30354658603). Must increase spending limit or fix payment before QUALITY-06 CI proof.

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| Verification | OpenSearch live SigV4 data plane (SEARCH-01) | Deferred to post-tag paid acceptance | 2026-07-22 |
| Cost | ECS Managed Instances + RabbitMQ HA ladder (MI-01, MI-02) | Deferred, explicitly not excluded — revisit post-v1 | 2026-07-27 |
| Cost | CostEstimator adapters for OVH, Scaleway, EKS (COST-01, COST-02) | v2 | 2026-07-27 |
| Operations | Pulumi Cloud / ESC hosted state (STATE-01), GitHub org move (ORG-01), storefront recipes (STORE-01) | v2 | 2026-07-27 |

## Session Continuity

Last session: 2026-07-28T14:44:49.342Z
Stopped at: Completed 02-01-PLAN.md
Resume file: None
Note: `02-RESEARCH.md` written; hosted CI remains HUMAN_GATE; packaging smoke needs plain Terminal; PHP/Composer required for RELEASE-06 proof on this Mac
