---
status: done
slug: ai-skills-tracks
spec: spec.md
---

# Plan: two skill tracks with acceptance

## Files that change

Exact paths. New vs edit. One line each on what changes.

- `internal/skills/crossref_test.go` (new, `package skills_test`): CLI cross-reference, schema key check, router-table check, embed-boundary check.
- `tests/fixtures/skills-acceptance/prompts.md` (new): the five user-track task prompts with quoted pass outputs.
- `tests/fixtures/skills-acceptance/magelift.yaml` (new): minimal valid fixture shop config, local-only, no secrets.
- `tests/fixtures/skills-acceptance/imports/` (new): small ACC/Upsun import fixtures for the migrate task.
- `tests/fixtures/skills-acceptance/contributor-checklist.md` (new): per-skill check commands for the 7 contributor skills.
- `contrib/skills/magelift-contribute/SKILL.md` (edit): records the per-intent skill-update ownership rule.
- `agents/skills/*/SKILL.md` (edit, only if the drift guard finds drift): bring named commands/keys back in sync.
- `agents/manifest.json` (regenerate only via `make generate`, and only if a skill body changed).
- `intent/ai-skills-tracks/acceptance.md` (new, written during execution): run transcript summary with per-task evidence.

## Order of work

Build and verify order, not a task dump. Group by area, number within the group.
Each box carries the check that closes it.

- [x] 1.1 Add the cross-reference test: extract `magelift <command>` spans from the five user skills, resolve against the Cobra tree; extract `magelift.yaml` key spans, resolve against `schema/magelift.schema.json`; parse the `AGENTS.md` table against the 12 skill dirs; assert no `contrib/skills` reference in the skills CLI path — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/skills/ -count=1` exits 0
- [x] 1.2 Fix whatever drift 1.1 finds (skill bodies and/or test parsing), then regenerate if a skill body changed — verify: the 1.1 suite stays green AND `make generate-check` exits 0
- [x] 2.1 Write the fixture `magelift.yaml` plus the ACC/Upsun import fixtures; pin the exact `magelift` argv for each prompt against the CLI tree — verify: `magelift config effective --config tests/fixtures/skills-acceptance/magelift.yaml` (or the tree-verified equivalent recorded in the prompt) exits 0
- [x] 2.2 Write the five task prompts with quoted pass outputs — verify: each prompt names its skill, its exact commands, and its pass output, and a cold read finds no invented flags
- [x] 2.3 Offline install and verify round-trip in a copy of the fixture dir with no network — verify: `magelift skills install --agent generic --scope project` then `magelift skills verify --agent generic --scope project` both exit 0 with `"clean": true` and digests matching the manifest
- [x] 2.4 Execute the user-track run: one agent session, five prompts in order, Docker available for local/operate tasks — verify: 5/5 quoted pass outputs observed and recorded in `intent/ai-skills-tracks/acceptance.md` (skipped tasks name the missing host capability)
- [x] 3.1 Write the contributor checklist with per-skill check commands; run the automated subset (command existence, docs-link resolution) — verify: 7/7 contributor skills report pass
- [x] 3.2 Record the per-intent skill-update rule in `magelift-contribute` — verify: `grep -F "same change" contrib/skills/magelift-contribute/SKILL.md` exits 0
- [x] 3.3 Run the `humanizer` skill, then `remove-ai-marks`, on touched human docs and skill prose — verify: both passes completed and the 1.1 suite plus `make generate-check` still exit 0

## Risks

What could break, and the check for each.

- Prompt/CLI drift between writing and running: box 2.1 pins argv against the tree, and box 1.1 re-runs green before 2.4 starts.
- Agent-run flake or host limits (no Docker): box 2.4 records skips with reasons instead of faking passes; at minimum the three offline tasks (configure, dependencies, migrate) must pass.
- Fixture rot after this intent: the 1.1 suite covers commands/keys, but fixture YAML validity is only checked when the run executes; the spec carries cadence as an open default (each RC).
- Skill-body edits invalidate the manifest: box 1.2 regenerates via `make generate` only, never by hand.

## Proof

The end-to-end evidence that the whole spec is met, not the per-step verifies
above. Tests, commands, or screenshots.

- `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/skills/ -count=1` exits 0 (separation, router, currency scenarios).
- `make generate-check` exits 0 (manifest in sync).
- `intent/ai-skills-tracks/acceptance.md` records the 5/5 user-track pass outputs with transcript excerpts plus the 7/7 contributor checklist results.
- `magelift skills list` on the release build names exactly the five user skills.
