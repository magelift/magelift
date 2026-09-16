---
status: done
slug: magento-deployment-safety
intent: intent.md
---

# Spec: Magento deployment safety (health that means Magento, one lifecycle)

## Requirements

What the system must do. Testable. Not file paths. One block per requirement,
each with at least one scenario.

### Requirement: Health establishes the intended rollout

The deployment Health step SHALL establish rollout identity beyond scheduler
settlement. Stabilize keeps the existing settle waits; Health additionally
asserts, on Kubernetes: observed generation equals the Deployment generation,
updated replicas equal desired replicas, ready replicas equal desired
replicas, the Available condition is true, and the pod-template image digest
equals the intended digest. On ECS: the deployment carrying the new task
definition revision reports COMPLETED, running count equals desired count on
the new revision, and the served image digest equals the intended digest.
Stale healthy replicas from a previous revision SHALL fail the gate.

#### Scenario: Stale replicas fail

- **WHEN** the previous revision reports ready/available (or running) while
  the intended revision has zero updated (or new-revision) replicas
- **THEN** Health fails naming the revision mismatch instead of reporting
  success

#### Scenario: Intended rollout passes

- **WHEN** the intended revision reaches the asserted counts, conditions,
  and digest on either platform
- **THEN** the rollout-identity portion of Health passes

### Requirement: Health runs a bounded Magento probe

Health SHALL run one bounded Magento probe proving application readiness
without heavyweight functional acceptance: a one-shot probe workload reusing
the existing candidate-runner seam, executing `bin/magento setup:db:status`
(boots the framework, requires the database, exits nonzero on pending or
failed migrations) plus, when search is enabled, search-engine wiring
coherence (`config:show catalog/search/engine` matches the planned mode)
and search-endpoint reachability. The probe SHALL carry an explicit timeout
and failure budget, and its failure SHALL fail the deployment with the probe
output attached. Pod/container liveness probes stay scheduler-level; the
spec documents that split instead of replacing cheap probes with Magento
boots on a hot path.

#### Scenario: Bootstrap failure fails the deploy

- **WHEN** PHP cannot boot Magento (broken build, missing env) in the probe
  workload
- **THEN** Health fails with the probe's failure output attached

#### Scenario: Migration residue fails the deploy

- **WHEN** the database carries pending or failed migrations after the
  migration step
- **THEN** the probe fails and Health fails naming the migration residue

#### Scenario: Search misconfiguration fails the deploy

- **WHEN** the stack provides a search endpoint but Magento has no wired
  engine (`config:show catalog/search/engine` empty or failing) or the
  endpoint is unreachable from the probe workload
- **THEN** Health fails naming the search incoherence. When the stack
  provides no search endpoint (search disabled), the search checks are
  skipped. Planned-mode string comparison is out of scope: the planned mode
  does not reach the Health seam without new cross-platform plumbing, and
  provisioning failures already surface at UpdateServices.

#### Scenario: Unreachable database fails the deploy

- **WHEN** the database endpoint is unreachable from the probe workload
- **THEN** Health fails. Cache/queue/session presence beyond Magento env
  wiring (covered offline) and full data-plane proof stay owned by
  `reference-store-acceptance`; the Health docs state that boundary.

### Requirement: One Magento lifecycle authority with build-once assets

The PHP lifecycle plan SHALL be the single static-content authority: static
content bakes into the immutable image at build time. The build SHALL require
`build.staticContent` locales plus themes and fail fast with a clear error
otherwise (no silent unbaked images). Go's migration shell SHALL drop its
deploy-time `setup:static-content:deploy` and match the PHP Deploy plus
PostDeploy sequence (`app:config:import`, `setup:upgrade --keep-generated`,
`cache:clean`, `cache:flush`). Serving containers SHALL be proven to receive
the baked assets by image-digest identity between the migration candidate
and the serving workloads. The shared-versus-ephemeral runtime state split
SHALL be documented (baked: code, DI output, static content; runtime:
Secrets/env-provided `env.php` values, disposable `var/`, bucket-backed
media, Valkey sessions).

#### Scenario: Unconfigured static content fails the build

- **WHEN** a build runs without `build.staticContent` locales plus themes
- **THEN** it fails naming the missing keys and stating that static content
  bakes at build, never at deploy

#### Scenario: No deploy-time static content

- **WHEN** the migration shell is inspected and the candidate job runs
- **THEN** neither invokes `setup:static-content:deploy`, and the candidate
  image digest equals the serving image digest for the release

### Requirement: Conservative incompatible-migration policy

The existing locks, production `--yes` approval, forward-only acknowledgement,
and promoted-digest signature gates SHALL stay. Production deploys SHALL
additionally require an explicit `--ack-maintenance-drain` attestation that
incompatible migrations, if any, are covered by a fresh backup, maintenance
mode, and drained web/consumer/cron writers. A runbook SHALL define the
sequence (backup, maintenance on, drain, deploy with the flag, verify,
maintenance off, scale back) and state the old-writer rule: old pods keep
serving during migration execution, so incompatible schema changes always
ride maintenance mode; digest rollback never reverses a migration.

#### Scenario: Production without the attestation refuses

- **WHEN** a production-class deploy runs without `--ack-maintenance-drain`
- **THEN** it refuses before mutating, naming the flag and the runbook

#### Scenario: Runbook complete

- **WHEN** a reader opens the incompatible-migration runbook
- **THEN** they find the backup/maintenance/drain/deploy/verify/restore
  sequence, the old-writer rule, and the no-rollback-of-schema statement

### Requirement: Writable-path requirements resolved and deliberate

The definitive writable-path set SHALL be documented with code references,
and the writable root SHALL be confirmed as the deliberate alpha answer (not
a temporary workaround): Magento containers need `env.php` generation,
disposable `var/`, `/tmp`, and the nginx pid writable, and Fargate empty
volumes mount root-owned. The code comment calling the setting temporary
pending storage design SHALL be updated to the resolved rationale. No
read-only-root flip SHALL occur in this intent.

#### Scenario: Definitive set recorded

- **WHEN** a reader opens the runtime-storage section
- **THEN** each writable path names its Magento requirement and the code or
  test that pins it, and the section states writable root is deliberate for
  the alpha recipe

### Requirement: Five failure classes tested with the implementation

Each failure class SHALL have at least one named automated test shipped in
this change: stale healthy replicas, PHP/bootstrap failure, failed
migration, misconfigured search, unreachable application dependency. Tests
SHALL run offline with fake platform clients (no cloud, no Docker); live
proof of the full loop stays owned by `reference-store-acceptance`.

#### Scenario: Named tests exist and pass

- **WHEN** the affected package suites run
- **THEN** one test per class passes offline, each failing for its class
  reason (verified by running each against the pre-fix behavior or a
  fault-injecting fake and observing the fail first where the harness
  allows)

### Requirement: Docs and skills describe the built behavior

The operator runbook (`docs/operations.md`), the architecture lifecycle
wording, and the `magelift-operate` skill SHALL describe deploy, Health
semantics, the maintenance/drain policy, and recovery as built. No touched
page SHALL promise zero-downtime incompatible schema changes.

#### Scenario: No stale promises

- **WHEN** the touched pages are reviewed
- **THEN** deploy success, Health meaning, the ack flag, and the runbook
  match the implementation, and no zero-downtime schema promise appears

## Design

How it fits the existing codebase: surfaces, data, APIs, ownership.

Health splits from Stabilize in both Steps implementations
(`internal/cloud/kube/steps.go`, `internal/cloud/aws/deployment/steps.go`):
Stabilize keeps the settle waits; Health adds rollout-identity assertions
read from the existing runtime checkers (extended to return generation,
updated counts, and digests) plus the one-shot Magento probe through the
existing candidate-runner seam (register, run probe command with timeout,
cleanup). ECS and kube probe commands are identical Magento invocations;
only the workload runner differs per platform, as today. The migration
candidate already runs the new image; Health additionally asserts candidate
digest equals serving digest.

`internal/platform/workloads.go` drops `setup:static-content:deploy` from
`MagentoMigrationShell()`; all callers (AWS deployment command, kube
candidate, others enumerated in the plan) inherit the PHP-matching sequence.
`internal/build/plan/plan.go` requires `build.staticContent` locales plus
themes. The PHP package needs no changes (it is already the correct
authority); its suite runs unchanged as a regression gate.

The `--ack-maintenance-drain` flag threads from the deploy command
(`internal/cli/lifecycle.go` deploy options) through the orchestrator
request into a production-class gate beside the existing `--yes` and
forward-only gates. The runbook lives in `docs/operations.md`; the
storage rationale extends the architecture runtime section; the skill
update stays scoped to deploy/recovery passages.

## Gotchas / policy flags

Security, auth, PII, compatibility, contradictions the spec cannot satisfy.

- Keep the existing locks, production approval, forward-only acknowledgement,
  promoted-digest gates, and bounded cleanup. This intent adds gates; it
  removes none.
- Keep probes bounded: one Magento probe per deploy with explicit timeout;
  no steady-state exec-probe flip; readiness-vs-liveness split documented.
- No blind read-only-root flip; writable root is confirmed deliberate here.
- Zero-downtime incompatible schema changes are explicitly post-alpha; no
  page may promise them.
- Requiring `build.staticContent` is a breaking config change pre-stability:
  the error must name the missing keys and show the fix; Order 6 documents
  the keys in onboarding. The sample shop is updated in this change.
- Human docs and website copy go through humanizer, then remove-ai-marks.
- Tests ship with the implementation; live proof belongs to acceptance.

## Open questions carried forward

Unresolved items from intent.md, plus new ones. Each has an owner or a default.

- Readiness signal set adopted: rollout identity plus one-shot
  `setup:db:status` with search wiring plus reachability when enabled, all
  bounded. Owner: spec author (adopted).
- Lifecycle authority adopted: PHP owns, Go orchestrates; build-once baked
  assets. Owner: spec author (adopted).
- Asset shape adopted: immutable build-once assets (no shared
  static-content volume); delivery proved by digest identity. Owner: spec
  author (adopted).
- New: steady-state exec probes deferred with documented rationale (load and
  flakiness risk on the hot path); revisit with pilot data post-alpha.
  Owner: maintainer.
