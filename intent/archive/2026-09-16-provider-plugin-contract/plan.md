---
status: done
slug: provider-plugin-contract
spec: spec.md
---

# Plan: provider plugin contract (the boundary v1 never had)

## Files that change

Exact paths. New vs edit. One line each on what changes.

- `docs/adr/0013-provider-plugin-contract.md` (new): accepted ADR with ownership split, typed operations, negotiation, fail-closed rules, module homes, tag scheme, and the 0008/0011 supersession list.
- `docs/adr/0008-provider-load-path.md` (edit): scope note naming which bullets stand and which 0013 supersedes.
- `docs/adr/0011-subprocess-dial-proof.md` (edit): scope note naming which bullets stand (provider-side Pulumi execution, go-plugin transport) and which 0013 supersedes (exact-version equality, silent fallback).
- `docs/adding-a-provider.md` (edit): boundary sections rewritten to the specified contract (ownership, typed-operations direction, `providers/<name>/` homes, negotiation, no-sandbox).
- `docs/architecture.md` (edit): boundary section updated to the specified contract (same five points, architecture-level wording).
- `contrib/skills/magelift-provider/SKILL.md` (edit only if its Boundary section contradicts the new text): align provider homes and contract direction.
- `docs/evidence/*`, `internal/*`, `sdk/*`, `cmd/*` (verify-only, zero edits): this intent ships no code and moves no directories.

## Order of work

Build and verify order, not a task dump. Group by area, number within the group.
Each box carries the check that closes it.

- [x] 1.1 Write ADR 0013 (Accepted, dated, with ownership table, operations list, negotiation rule, failure-mode table, module homes, tag scheme, compat matrix rule, proof gates, and the verbatim 0008/0011 supersession list) — verify: the file exists with `Status: Accepted`, and each of the eight spec requirements has a matching ADR section found by grep
- [x] 1.2 Add reciprocal scope notes to ADRs 0008 and 0011 — verify: each file names the standing bullets and the 0013-superseded bullets, and quotes no superseded bullet as current
- [x] 2.1 Rewrite the boundary sections of `docs/adding-a-provider.md` to the specified contract — verify: ownership, typed-operations direction, `providers/<name>/` homes, negotiation/fail-closed, and no-sandbox statements are all present and read identically to the spec
- [x] 2.2 Update the boundary section of `docs/architecture.md` to the specified contract — verify: the same five points are present in architecture-level wording with no contradictions to the spec or ADR
- [x] 2.3 Check `contrib/skills/magelift-provider/SKILL.md` Boundary against the new text; align only on contradiction — verify: the skill either needed no change (recorded why) or its Boundary now matches the spec with no other sections touched
- [x] 3.1 Run humanizer, then remove-ai-marks, on touched human pages — verify: both passes completed (or the marks check recorded as unperformed with reason) and boxes 1.1–2.3 verifies still pass
- [x] 3.2 Prove the no-code scope held — verify: `git status` shows only the five listed doc paths (plus this intent's spec/plan/report) changed, and `go build ./...` plus the affected doc test still pass

## Risks

What could break, and the check for each.

- ADR wording drifts from the spec (two sources of truth): box 1.1 grep maps each requirement to its ADR section; the spec stays normative, the ADR records the decision.
- Scope notes misquote 0008/0011 (stale bullets): box 1.2 verify quotes bullets verbatim from the current files before writing.
- Human docs contradict the spec on homes or negotiation: boxes 2.1–2.2 verify by reading the three documents side by side; contradictions fail the box.
- Skill drift (Boundary contradicts, checklist renumbered by accident): box 2.3 restricts edits to the Boundary section and diffs the rest as unchanged.
- Accidental code or directory changes: box 3.2 `git status` scoping fails the box on any `internal/*`, `sdk/*`, `cmd/*`, or moved directory.
- Marks service unreachable: box 3.1 records unperformed explicitly; never claim the pass.

## Proof

The end-to-end evidence that the whole spec is met, not the per-step verifies
above. Tests, commands, or screenshots.

- `docs/adr/0013-provider-plugin-contract.md` exists, Accepted and dated.
- `make docs` exits 0 (new ADR plus edited pages build).
- `git status` shows only the listed doc paths plus intent records.
- No cloud resources created, changed, or destroyed (no live commands run).
