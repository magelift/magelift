---
status: done
slug: magento-deployment-safety
---
# Intent: Magento deployment safety (health that means Magento, one lifecycle)

## Problem

Deployments can report success before Magento works, and two lifecycle
authorities can diverge. Scheduler-level health (replica availability, static
HTTP responses, container checks without Magento) cannot prove a working
application, and a separate health command does not repair a deploy that
already claimed success. Meanwhile the PHP lifecycle plan and the Go
orchestration carry overlapping migration sequences, with static assets
generated in a disposable job at risk of never reaching serving containers and
no explicit policy for old writers during schema changes.

## Evidence

`intent/audit.md` F05: AWS stabilization and health in
`internal/cloud/aws/deployment/steps.go` share the same service-health wait;
Kubernetes stabilization and health in `internal/kube/steps.go` rely on
replica availability without establishing observed generation, updated
replicas, and image digest. Some resource creation uses `skipAwait`. The nginx
health endpoint is a static response; the AWS container definition explicitly
describes a health check without Magento.

F06: `build/src/Magento/LifecyclePlan.php` holds the PHP lifecycle plan while
`internal/platform/workloads.go` holds a separate migration shell sequence
including static-content work. The Kubernetes candidate migration job in
`internal/kube/candidate.go` does not mount a shared static-content volume in
the traced path — assets generated inside the disposable job may not reach
serving containers (source-level concern, not a demonstrated live failure).
The sequence needs an explicit policy for old web, consumer, and cron writers
while schema changes run; a digest rollback cannot undo an incompatible
migration. Existing locks, production approval, forward-only acknowledgement,
and bounded cleanup are good and stay.

F12 runtime half: writable-path requirements must be resolved before
read-only-root claims can stand; blindly enabling read-only root would likely
break Magento.

## Proposed outcome

Success means the intended rollout and application readiness: deployments
establish observed generation, updated replicas, and image digest plus bounded
Magento readiness (not heavyweight functional acceptance), distinguish
readiness from liveness, and fail loudly on stale healthy replicas,
PHP/bootstrap failure, failed migration, misconfigured search, and unreachable
dependencies. One Magento lifecycle authority exists — preferably the existing
PHP package with Go orchestrating it — with immutable application assets built
once, explicit shared-vs-ephemeral runtime state, and proof that serving
containers receive the intended assets. Incompatible migrations start under a
conservative maintenance/drain policy. No zero-downtime promise for alpha.

## Affected users and systems

Every deploy on every provider. `internal/kube/steps.go` and
`internal/cloud/aws/deployment/steps.go` (stabilize/health),
`internal/platform/workloads.go`, `build/src/Magento/LifecyclePlan.php`,
candidate/migration job wiring, container health checks and probes, Phoenix
image entrypoints as touched, operator docs and skills for deploy/recovery.

## Constraints

- Keep the existing locks, production approval, forward-only acknowledgement,
  and bounded cleanup.
- Keep probes bounded; readiness is not full functional acceptance (that
  belongs to `reference-store-acceptance` on the shipped path).
- Resolve actual writable-path requirements, then align configuration and
  claims — no blind read-only-root flip.
- Conservative incompatible-migration policy first; zero-downtime schema
  changes are explicitly post-alpha.
- Tests prove stale-replica, bootstrap-failure, migration-failure,
  search-misconfig, and dependency-unreachable behavior; implementation ships
  with its tests, not deferred to acceptance.
- Human docs touched here go through humanizer, then remove-ai-marks.

## Out of scope

- Provider plugin mechanics (owned by the contract plus `gcp-autonomous-provider`).
- The alpha recipe definition and onboarding prose (owned by
  `full-deployment-coverage`).
- Live store proof on the shipped path (owned by
  `reference-store-acceptance`); this intent makes deploys honest so that
  proof is meaningful.
- Cross-cloud disaster recovery.

## Open questions

- Exact readiness signal set per runtime (which Magento endpoints or commands,
  with what timeouts and failure budgets)? Default: minimal signals that catch
  the five failure classes above; the spec names them. Owner: spec author.
- PHP package as the single authority vs Go-owned sequence: confirmed, or is
  there a reason the PHP plan cannot own static-content and migration order?
  Default: PHP owns, Go orchestrates. Owner: spec author.
- Shared static-content volume vs baked-image assets: which shape does the
  alpha recipe use? Default: immutable build-once assets; the spec proves
  delivery to serving containers. Owner: spec author.
