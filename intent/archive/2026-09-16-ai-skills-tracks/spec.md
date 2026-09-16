---
status: done
slug: ai-skills-tracks
intent: intent.md
---

# Spec: two skill tracks with acceptance

## Requirements

What the system must do. Testable. Not file paths. One block per requirement,
each with at least one scenario.

### Requirement: Track separation

The release binary SHALL embed exactly the five user skills. Contributor
skills SHALL NOT be embedded in the binary or installable through
`magelift skills`.

#### Scenario: Embedded set is exactly the user track

- **WHEN** `magelift skills list` runs on a release build
- **THEN** it names exactly `magelift-configure`, `magelift-dependencies`, `magelift-local-runtime`, `magelift-migrate`, `magelift-operate` (5 entries)
- **AND** `grep -rn "contrib/skills" internal/cli/skills.go internal/skills/*.go` returns empty

#### Scenario: Manifest matches the user skill dirs

- **WHEN** `agents/manifest.json` regenerates via `make generate`
- **THEN** its entries equal the `agents/skills/*/` dirs by name, with matching digests
- **AND** `make generate-check` exits 0 afterward

### Requirement: Offline install and verify

Skill install and verify from the release binary SHALL work with no network
access, copying embedded files without executing them.

#### Scenario: Fixture install round-trips clean

- **WHEN** in a fixture shop dir with no network, `magelift skills install --agent generic --scope project` then `magelift skills verify --agent generic --scope project` run
- **THEN** both exit 0
- **AND** the report reads `"clean": true`
- **AND** every installed `SKILL.md` digest equals its manifest digest

### Requirement: Router completeness

The `AGENTS.md` skill table SHALL route every row to an existing skill, and
every skill SHALL appear in the table exactly once.

#### Scenario: Table and tree agree

- **WHEN** the router table is checked against the tree
- **THEN** every `agents/skills/<name>` and `contrib/skills/<name>` path resolves to a dir containing `SKILL.md`
- **AND** each of the 12 skill dirs (5 user + 7 contributor) appears in the table exactly once

### Requirement: Skill currency

Every `magelift <command>` named in a user skill SHALL exist in the generated
CLI reference, and every `magelift.yaml` key named SHALL exist in the JSON
schema. A change that alters CLI behavior SHALL update the matching skill in
the same diff.

#### Scenario: Cross-reference check passes

- **WHEN** the skill cross-reference check runs
- **THEN** it reports zero unknown commands and zero unknown keys

#### Scenario: Behavior change carries its skill update

- **WHEN** a change adds or alters a CLI command or `magelift.yaml` key
- **THEN** the same diff updates the user skill that teaches it

### Requirement: User-track acceptance run

A scripted agent run on a fixture shop, using only installed user skills,
SHALL complete one task per skill through documented Magelift commands only.

#### Scenario: Configure task passes offline

- **WHEN** the agent receives the configure prompt in the fixture shop
- **THEN** it validates the fixture YAML and shows effective values with provenance, exit 0

#### Scenario: Dependencies task passes

- **WHEN** the agent receives the dependencies prompt
- **THEN** `magelift doctor` runs and reports each required tool reachable or explicitly missing, exit 0

#### Scenario: Migrate task passes offline

- **WHEN** the agent receives the migrate prompt with the ACC/Upsun import fixtures
- **THEN** it produces mapped MageLift YAML with unmapped fields listed, and no secret values in YAML

#### Scenario: Local-runtime task passes

- **WHEN** the agent receives the local-runtime prompt
- **THEN** the local stack reaches healthy status and serves the Magento health endpoint

#### Scenario: Operate task passes

- **WHEN** the agent receives the operate prompt against the local stack
- **THEN** status, logs, and one safe Magento command (cache flush) all succeed, exit 0

### Requirement: Contributor-track check

Each contributor skill SHALL have a named check proving its procedure is
executable without running paid clouds.

#### Scenario: All seven contributor checks pass

- **WHEN** the contributor checklist runs
- **THEN** all 7 skills report pass: every command and script each skill names exists, and every docs link it cites resolves
- **AND** any procedure needing a live cloud or a release tag is marked dry-run-only with its last executed date recorded in the acceptance report

## Design

How it fits the existing codebase: surfaces, data, APIs, ownership.

The prompt pack and fixture live under `tests/fixtures/skills-acceptance/`
(prompts plus a minimal valid `magelift.yaml` plus small ACC/Upsun import
fixtures). They are test material, never shipped: nothing under
`tests/fixtures/` is embedded or installed.

The drift guard is a Go test in `internal/skills` (`package skills_test`, so
it can build the Cobra tree without an import cycle): it extracts
`magelift <command>` spans from the five user `SKILL.md` files and resolves
each against the real command tree, extracts `magelift.yaml` key spans and
resolves each against the generated JSON schema, and parses the `AGENTS.md`
skill table against the skill dirs. One suite covers the router, currency,
and separation scenarios that are statically checkable.

Ownership: the author of a v1 intent updates every skill its behavior
touches, in the same change. The rule is recorded in
`contrib/skills/magelift-contribute/SKILL.md`, and `sdlc-verify` checks the
diff for the skill update whenever the CLI surface moves.

The acceptance method follows the skill-creator loop (draft prompts, run,
evaluate against quoted pass outputs, iterate): the with-skills pass is the
gate; a without-skills baseline run is an optional diagnostic when a task
fails, to tell a bad prompt from a bad skill. The run needs Docker for the
local-runtime and operate tasks but no cloud account and no network for
install, configure, dependencies, or migrate.

## Gotchas / policy flags

Security, auth, PII, compatibility, contradictions the spec cannot satisfy.

- The acceptance run is human-judged: a fresh-context verifier confirms each
  quoted pass output against the transcript. It does not run in CI.
- The fixture and prompts SHALL contain no secret values; credentials stay in
  environment names per the local-runtime contract.
- The embed boundary is a leak boundary: contributor runbooks (certify,
  release, provider internals) must never reach shop repos through the
  binary. The separation scenario guards it.
- Installed `.cursor/` / `.claude/` skill copies stay local and uncommitted;
  the fixture run cleans them up afterward.
- `magelift local` tasks need Docker on the run host; the spec does not
  require them to pass on hosts without it, but the run report must say
  which tasks were skipped and why.

## Open questions carried forward

Unresolved items from intent.md, plus new ones. Each has an owner or a default.

- Acceptance shape (intent Q1): decided here as both — scripted run for the
  user track, checklist for the contributor track. Owner: this spec.
- Per-intent skill-update ownership (intent Q2): decided here as intent
  author, enforced at verify. Owner: this spec.
- Run cadence: each RC, or on every skill change? Default: run the full
  acceptance on each release candidate; run the automated checks (drift
  guard, router, offline install) in `make local-gates`. Owner: release
  captain.
- Fixture scope: extend `examples/sample-shop` or a dedicated minimal
  fixture? Default: dedicated minimal fixture under
  `tests/fixtures/skills-acceptance/`, so the acceptance run never depends
  on sample-shop edits. Owner: implementer.
