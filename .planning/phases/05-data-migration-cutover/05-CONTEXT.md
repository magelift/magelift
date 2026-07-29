# Phase 5: Data Migration & Cutover - Context

**Gathered:** 2026-07-29
**Status:** Ready for planning

<domain>
## Phase Boundary

Make `seedDump` real: import Magento dumps into the target DB after first successful deploy, expose an honest `seedDumpStatus` machine, sync media into the env’s object storage, and ship a cutover runbook whose executable steps are proven offline. Managed-instance dump cell and live DNS cutover ride Phase 7’s GCP pass (no new paid AWS pass). Phase 6 Kube day-2 and Phase 8 attach stay out of scope.

</domain>

<decisions>
## Implementation Decisions

### Dump import runner placement
- **D-01:** Hybrid: after the environment’s first successful infra deploy + Magento install/upgrade candidate, automatically run the dump importer **once** when `seedDump` is set and status is `recorded`. Provide explicit `magelift env import-dump` for manual runs and retries. Never import inside Pulumi create (ADR 0010). Never name the command `env seed` (collides with `magelift dev seed`). — **Reversibility:** one-way — `--dump` after-first-deploy contract becomes operator expectation
- **D-02:** Auto path fires only from `recorded`; later deploys must not re-import unless the operator runs `env import-dump` with confirmation when required. — **Reversibility:** costly — silent re-import on every deploy would be a data-loss footgun

### Status persistence
- **D-03:** Keep `environments.<name>.seedDump` path in `magelift.yaml` (ADR 0010). Persist the status state machine (`recorded` → `importing` → `imported` | `failed` + reason) under `.magelift/` (release-journal pattern), not in committed YAML. `magelift env status` merges YAML path + journal. — **Reversibility:** costly — journal path becomes CI/automation contract

### Overwrite / retry safety
- **D-04:** Re-running import against a non-empty database exits non-zero unless persistent `--yes` is set (same MageLift destructive pattern as init/destroy — do not invent `--force` / `--confirm-overwrite`). With `--yes`, perform a full schema-replace import so interrupted re-runs converge. Define “non-empty” explicitly in implementation (user tables / Magento schema present). — **Reversibility:** one-way — exit codes + `--yes` become scripts’ contract

### Media sync
- **D-05:** Ship `magelift env media-sync --source <dir>` copying a local media tree into the environment’s media object-storage bucket; verify with listing diff empty (SC4). Prove on Floci. Do **not** add `seedMedia` auto-after-deploy this phase (follow-on). — **Reversibility:** reversible — `seedMedia` can layer later without breaking the command

### Cutover runbook proof
- **D-06:** Write cutover runbook in `docs/migrating-from-paas.md` (DNS, maintenance mode, reindex, verification, rollback). Prove dump + media + maintenance/reindex/verify/rollback **locally** this phase with a recorded scratch run. Defer DNS + live non-prod cutover evidence to Phase 7 under an explicit HUMAN_GATE (no fourth paid pass). — **Reversibility:** costly if someone marks MIGRATE-04 Complete without the HUMAN_GATE split — STATE/REQUIREMENTS must record the split

### the agent's Discretion
- Exact `.magelift/` journal filename/layout for seed status
- Importer mechanism details (one-off migrate task vs `exec` helper vs local mysql client) as long as ADR 0010 + offline proof hold
- Media key prefix mapping (`pub/media` → object keys) and merge-vs-wipe default (document in help)
- Whether production-class envs need an extra protection gate beyond `--yes` (prefer no unless existing protection flag already applies)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase / requirements
- `.planning/ROADMAP.md` — Phase 5 goal + success criteria + cloud spend map
- `.planning/REQUIREMENTS.md` — MIGRATE-01..05
- `docs/adr/0010-database-dump-seed.md` — seedDump path + after-first-deploy import

### CLI / status patterns
- `internal/cli/env.go` — current inert `seedDump` / placeholder status string
- `internal/releasejournal/` — `.magelift/` journal pattern to mirror for status
- Destructive `--yes` pattern (init overwrite, destroy, secrets)

### Media / offline proof
- `tests/floci/storage_test.go` — versioned media / S3 Floci coverage
- `docs/migrating-from-paas.md` — cutover runbook home
- `docs/capability-matrix.md` — media Floci row honesty

### Deferred
- Phase 7 — managed dump-import cell + live DNS/cutover HUMAN_GATE
- Phase 8 — brownfield attach existing DB/VPC
- `seedMedia` auto-seed after deploy — post-Phase-5 follow-on

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `env create --dump` already validates path readability and writes `seedDump` into YAML
- Floci S3 media restore tests for offline object-storage proof
- `magelift dev` MySQL for local dump import verification
- Release journal under `.magelift/` for durable runtime status

### Established Patterns
- Destructive confirmation = `--yes`, not `--force`
- Honesty-first: no silent success; status must move or fail loudly
- Offline-first Phase 5; paid managed proof rides Phase 7

### Integration Points
- Deploy pipeline success gate triggers once-only auto-import when status=`recorded`
- `env status` / `env import-dump` / `env media-sync` hang off existing `env` cobra group
- Runbook updates `docs/migrating-from-paas.md` (importer docs already landed in Phase 4)

</code_context>

<specifics>
## Specific Ideas

- [--auto] Persona synthesis (Staff Platform / DX CLI / Agency Lead / Release Manager / Magento Cloud Operator) locked D-01..D-06 in a single pass
- Prefer command name `import-dump` over `seed` to avoid `dev seed` collision
- Hosted CI may upload `.magelift/` seed journal as artifact; do not commit transient status into git

</specifics>

<deferred>
## Deferred Ideas

- Live DNS + full non-prod cutover rehearsal — Phase 7 HUMAN_GATE
- Managed Aurora/Cloud SQL dump-import cell — rides Phase 7 GCP pass
- `seedMedia` config + auto post-deploy — follow-on
- Brownfield attach existing DB — Phase 8
- Deferred job/queue for huge dumps outside deploy lock — only if timeouts prove painful

</deferred>
