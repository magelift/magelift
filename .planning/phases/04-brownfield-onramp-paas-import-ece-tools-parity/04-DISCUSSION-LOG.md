# Phase 4: Brownfield Onramp — PaaS Import & ece-tools Parity - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-29
**Phase:** 4-Brownfield Onramp — PaaS Import & ece-tools Parity
**Areas discussed:** Fixture sources, CLI surface + overwrite, Unmappable-key report UX, ece-tools parity depth, Env-var mapping v1 surface
**Mode:** `--auto` with parallel personas; best recommendations locked

---

## Fixture sources

| Option | Description | Selected |
|--------|-------------|----------|
| Synthetic `testdata/fixtures/{acc,upsun}/` only | Hermetic CI | |
| Sibling/submodule real repos | Highest fidelity | |
| Hybrid synthetic CI + optional `MAGELIFT_IMPORT_FIXTURE_*` soak | CI hermetic + maintainer soak | ✓ |

**User's choice:** [--auto] Hybrid (Staff Platform Engineer persona)
**Notes:** Clean-room; no vendoring sibling ece-tools/cli trees

---

## CLI surface + overwrite

| Option | Description | Selected |
|--------|-------------|----------|
| `init --from-acc` / `--from-upsun` + refuse unless `--yes` | Matches IMPORT-01/02 + existing init honesty | ✓ |
| `init --from <enum>` | Scalable but rewrites REQUIREMENTS | |
| `magelift import …` | New verb; docs/MIGRATE clash | |

**User's choice:** [--auto] Separate init flags; refuse-if-exists unless `--yes`; optional `--output`
**Notes:** Prefer `--yes` over inventing `--force` (MageLift destructive pattern)

---

## Unmappable-key report UX

| Option | Description | Selected |
|--------|-------------|----------|
| Fail, write nothing | Atomic, no review artifact | |
| Write YAML + sidecar report, exit non-zero | Reviewable + refuses success | ✓ |
| Write YAML, warn stderr, exit 0 | False success | |
| Soft default; `--strict` to fail | Honesty opt-in | |

**User's choice:** [--auto] Write + sidecar + non-zero (Agency Lead persona)

---

## ece-tools parity depth

| Option | Description | Selected |
|--------|-------------|----------|
| `docs/ece-parity.md` only | Docs honesty | |
| Matrix + close high-traffic half-done gaps | Patches + SCD strategy/threads + common env map | ✓ |
| Full ece-tools behavioral parity | Clone | |

**User's choice:** [--auto] Matrix + high-traffic gaps (Release Manager persona)

---

## Env-var mapping v1 surface

| Option | Description | Selected |
|--------|-------------|----------|
| Structural YAML only | Thin IMPORT-04 | |
| Structural + v1 high-traffic allowlist | Store-breakers mapped; rest intentional gaps | ✓ |
| Exhaustive ACC/Upsun env parity | Over-claim | |

**User's choice:** [--auto] Structural + allowlist (Magento Cloud Operator persona)

---

## the agent's Discretion

- Sidecar report path/filename
- Exact fixture tree layout under `testdata/fixtures/{acc,upsun}/`
- Long-tail intentional-gap wording beyond allowlist

## Deferred Ideas

- Dump/media cutover — Phase 5
- Live GCP — Phase 7
- Brownfield attach — Phase 8
- Full ece-tools clone / exhaustive env parity — out of rc scope
