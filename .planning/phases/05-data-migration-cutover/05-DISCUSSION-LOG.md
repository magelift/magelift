# Phase 5: Data Migration & Cutover - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-29
**Phase:** 5-Data Migration & Cutover
**Areas discussed:** Dump import runner, Status persistence, Overwrite safety, Media sync, Cutover runbook proof
**Mode:** `--auto` with parallel personas; best recommendations locked

---

## Dump import runner placement

| Option | Description | Selected |
|--------|-------------|----------|
| A Auto post-deploy only | Zero-touch; weak MIGRATE-05 | |
| B Explicit `env import-dump` only | Honest; breaks `--dump` MIGRATE-01 | |
| C Hybrid auto-once + `env import-dump` | Agency Lead + honesty | ✓ |
| D Deferred job queue | Overbuilt for Phase 5 | |

**User's choice:** [--auto] Hybrid C (Agency Lead; Release Manager / Magento Cloud Operator dissent mitigated via status gate + no `env seed` name)
**Notes:** Auto only from `recorded`; retries via `env import-dump`; overwrite confirm via `--yes` (D-04)

---

## Status persistence

| Option | Description | Selected |
|--------|-------------|----------|
| A Status in magelift.yaml | Dirty desired config | |
| B Status only in `.magelift/` | Path underspecified | |
| C Path in YAML + status in `.magelift/` | ADR + journal pattern | ✓ |
| D Cloud tag/secret only | Not offline | |

**User's choice:** [--auto] C (Staff Platform / DX CLI / Release Manager)

---

## Overwrite / retry safety

| Option | Description | Selected |
|--------|-------------|----------|
| A `--yes` for non-empty overwrite | Matches MageLift D-04 pattern | ✓ |
| B Dedicated `--confirm-overwrite` | Flag sprawl | |
| C Never overwrite — destroy env | Breaks interrupted re-run | |
| D Soft truncate + `--yes` | False converge risk | |

**User's choice:** [--auto] A (DX CLI / Staff Platform)

---

## Media sync

| Option | Description | Selected |
|--------|-------------|----------|
| A `env media-sync --source` Floci-proven | SC4 offline | ✓ |
| B `seedMedia` + auto after deploy | Follow-on | |
| C Top-level `media sync` | Sprawl | |
| D Docs-only aws s3 sync | Fails MIGRATE-03 | |

**User's choice:** [--auto] A (Agency Lead / Magento Cloud Operator / Staff Platform)

---

## Cutover runbook proof

| Option | Description | Selected |
|--------|-------------|----------|
| A Local-only claim SC5 complete | Overclaim DNS | |
| B Paid AWS cutover this phase | 4th paid pass | |
| C Doc now; all evidence Phase 7 | Late failure on paid path | |
| D Hybrid local proof + Phase 7 DNS/live HUMAN_GATE | Honesty without extra spend | ✓ |

**User's choice:** [--auto] D (Release Manager / Staff Platform)

---

## Auto-selection log

```
[--auto] Selected all gray areas: Dump import runner, Status persistence, Overwrite safety, Media sync, Cutover runbook proof
[auto] Dump import — → Selected: Hybrid C (recommended)
[auto] Status — → Selected: Path YAML + status .magelift/ (recommended)
[auto] Overwrite — → Selected: --yes (recommended)
[auto] Media — → Selected: env media-sync --source (recommended)
[auto] Cutover proof — → Selected: Hybrid D (recommended)
```
