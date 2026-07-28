---
phase: 03-credit-efficient-acceptance-harness-evidence-tiering
generated: auto
source: yolo auto-from-roadmap after Phase 2 pass
---

# Phase 3 Context

## Locked Decisions
1. One long-lived AWS free-tier preview stack for harness proof (PAID pass 1 of 3) — do not start paid create until HUMAN_GATE confirms spend OK.
2. Offline-first: resume, evidence append, assert_clean, matrix tiering, Floci/mocks, GCP dry-run path — build and test before any paid pass.
3. Capability matrix must not over-claim; Aurora CreateDBCluster, amazon-mq×preview, OpenSearch SigV4 data-plane get explicit unverifiable reasons.
4. Phase 1 hosted CI still deferred; Phase 2 is complete.

## Agent Discretion
- Plan split offline harness vs paid proof wave
- Stop at HUMAN_GATE before any AWS create that spends money

## Scope Fence
IN: ACCEPT-01..06, TRUST-03, TRUST-04 harness + matrix evidence tiering
OUT: live GCP spend (Phase 7), import/migrate, kube day-2 implementation beyond what harness needs
