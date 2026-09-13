---
status: done
slug: onboarding-migration-presets
intent: intent.md
---

# Spec: install, import or preset, deploy

Auto-approved per the standing `/goal` instruction. Defaults taken:
starters per certified provider with three env blocks; sidecar gains
family guidance with a generic hook before/after (no user values
echoed, so no secret leak); `doctor` names the fix command on failure.

## Requirements

### Requirement: provider starters validate cleanly

`magelift init --provider aws|gcp` SHALL write a starter with
preview, staging, and production env blocks that passes `config
validate` with no other edits, and `doctor` SHALL name bootstrap as
the next command for it. Default stays `aws`.

#### Scenario: AWS starter validates

- **WHEN** a new team runs `magelift init --provider aws` in an empty
  directory
- **THEN** `magelift config validate` exits 0
- **AND** `magelift doctor` prints `Next: magelift bootstrap --env
  preview`

#### Scenario: GCP starter validates

- **WHEN** a new team runs `magelift init --provider gcp` in an empty
  directory
- **THEN** `magelift config validate` exits 0
- **AND** `magelift doctor` prints `Next: magelift bootstrap --env
  preview`

#### Scenario: unknown provider refused with authority

- **WHEN** a user passes `--provider` outside the certified set
- **THEN** init fails before writing anything and names MageLift as
  the authority for the supported set

### Requirement: import sidecar reads as a checklist

The `<stem>.unmapped.md` sidecar SHALL group residual keys by family
with one action each, SHALL show a before/after for common ACC shell
hooks, and SHALL NOT echo user values.

#### Scenario: hooks get a before/after

- **WHEN** an ACC import leaves `hooks.deploy` unmapped
- **THEN** the sidecar shows the ACC hook shape beside the
  `magelift.yaml` build-hooks shape with a link to the migration doc

#### Scenario: no values leak

- **WHEN** any import leaves residuals
- **THEN** the sidecar contains paths and guidance only; no PaaS value
  bytes appear in the report

### Requirement: doctor names the fix on failure

When readiness checks fail, `doctor` SHALL set `Next` to the one
command most likely to fix the first failure instead of leaving it
empty.

#### Scenario: build failure points at validate

- **WHEN** `doctor` fails on the `build` check
- **THEN** `Next` is `magelift config validate`

#### Scenario: missing dependency points at install

- **WHEN** `doctor` fails only on a dependency check
- **THEN** `Next` is `magelift doctor --install-dependencies`

### Requirement: starters promise three documented shapes

Getting-started SHALL name the two certified starters (provider,
envs, queue and search defaults) so trial teams know what they get
before they run init.

#### Scenario: shapes documented

- **WHEN** a reader opens getting-started
- **THEN** the AWS and GCP starter shapes (envs plus queue plus
  search defaults) are stated with the `init --provider` command

## Design

Surfaces: `init --provider` flag plus two starter constants;
`RenderUnmappedReport` gains a family-guidance section driven by a
small pattern table; `inspectProject` plus `appendDependencyChecks`
set `Next` from the first failed check via a check-ID map.
`config effective` already shows resolved queue/search values, so no
config changes. User skills updated only if they contradict the new
flags.

Data: starters keep the current starter's explicit style (edition,
version, mode, webRuntime, php, region, preset). GCP starter uses the
fixture's `target.gcp` shape (project plus region placeholders).

Ownership: CLI owns flags and starters; `paasimport` owns sidecar
guidance; docs own the shapes promise.

## Gotchas / policy flags

- Init refuses to overwrite without `--yes` (unchanged); the provider
  check runs before the overwrite check so a bad flag never looks
  like a file problem.
- Foreign YAML stays invalid as `--config` input (probe unchanged).
- No plaintext secrets in starters; composer credentials stay secret
  references where needed.
- Existing validation errors keep their text; only new errors add
  authority naming (full audit is out of scope).

## Open questions carried forward

- Starter shapes promised: AWS ecs-fargate plus GCP gke-autopilot,
  each with preview/staging/production envs, queue `db` default on
  preview, search `disabled` (defaults visible via `config
  effective`). OVH/Scaleway starters wait for experimental Magento
  runs.
- Top doctor failures from real trials: unknown until trials run;
  the Next map covers build, environment, and dependency families
  first, and trial feedback extends it.
