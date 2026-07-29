# Phase 5: Data Migration & Cutover - Research

**Researched:** 2026-07-29
**Domain:** Magento DB dump import, seedDump status journal, media object-storage sync, cutover runbook (offline-first)
**Confidence:** HIGH (codebase seams + CONTEXT locks); MEDIUM (external dump/timeout community patterns)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Hybrid: after the environment’s first successful infra deploy + Magento install/upgrade candidate, automatically run the dump importer **once** when `seedDump` is set and status is `recorded`. Provide explicit `magelift env import-dump` for manual runs and retries. Never import inside Pulumi create (ADR 0010). Never name the command `env seed` (collides with `magelift dev seed`). — **Reversibility:** one-way — `--dump` after-first-deploy contract becomes operator expectation
- **D-02:** Auto path fires only from `recorded`; later deploys must not re-import unless the operator runs `env import-dump` with confirmation when required. — **Reversibility:** costly — silent re-import on every deploy would be a data-loss footgun
- **D-03:** Keep `environments.<name>.seedDump` path in `magelift.yaml` (ADR 0010). Persist the status state machine (`recorded` → `importing` → `imported` | `failed` + reason) under `.magelift/` (release-journal pattern), not in committed YAML. `magelift env status` merges YAML path + journal. — **Reversibility:** costly — journal path becomes CI/automation contract
- **D-04:** Re-running import against a non-empty database exits non-zero unless persistent `--yes` is set (same MageLift destructive pattern as init/destroy — do not invent `--force` / `--confirm-overwrite`). With `--yes`, perform a full schema-replace import so interrupted re-runs converge. Define “non-empty” explicitly in implementation (user tables / Magento schema present). — **Reversibility:** one-way — exit codes + `--yes` become scripts’ contract
- **D-05:** Ship `magelift env media-sync --source <dir>` copying a local media tree into the environment’s media object-storage bucket; verify with listing diff empty (SC4). Prove on Floci. Do **not** add `seedMedia` auto-after-deploy this phase (follow-on). — **Reversibility:** reversible — `seedMedia` can layer later without breaking the command
- **D-06:** Write cutover runbook in `docs/migrating-from-paas.md` (DNS, maintenance mode, reindex, verification, rollback). Prove dump + media + maintenance/reindex/verify/rollback **locally** this phase with a recorded scratch run. Defer DNS + live non-prod cutover evidence to Phase 7 under an explicit HUMAN_GATE (no fourth paid pass). — **Reversibility:** costly if someone marks MIGRATE-04 Complete without the HUMAN_GATE split — STATE/REQUIREMENTS must record the split

### Claude's Discretion
- Exact `.magelift/` journal filename/layout for seed status
- Importer mechanism details (one-off migrate task vs `exec` helper vs local mysql client) as long as ADR 0010 + offline proof hold
- Media key prefix mapping (`pub/media` → object keys) and merge-vs-wipe default (document in help)
- Whether production-class envs need an extra protection gate beyond `--yes` (prefer no unless existing protection flag already applies)

### Deferred Ideas (OUT OF SCOPE)
- Live DNS + full non-prod cutover rehearsal — Phase 7 HUMAN_GATE
- Managed Aurora/Cloud SQL dump-import cell — rides Phase 7 GCP pass
- `seedMedia` config + auto post-deploy — follow-on
- Brownfield attach existing DB — Phase 8
- Deferred job/queue for huge dumps outside deploy lock — only if timeouts prove painful
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MIGRATE-01 | `--dump` + actual import after first successful deploy | Config `SeedDump` field (Wave 0), hybrid auto-hook after `deployflow.Run`, local MySQL importer; managed instance proof → Phase 7 |
| MIGRATE-02 | Accurate `seedDumpStatus` recorded→importing→imported\|failed+reason | `.magelift/` journal + `env status` merge; never persist status in YAML |
| MIGRATE-03 | Sync Magento media into target object storage | `env media-sync --source`; Floci S3 listing-diff proof |
| MIGRATE-04 | Cutover runbook (DNS, maintenance, reindex, verify, rollback) | `docs/migrating-from-paas.md` + local scratch proof; DNS/live → Phase 7 HUMAN_GATE |
| MIGRATE-05 | Safe retry; refuse overwrite without confirmation | Persistent `--yes` + schema-replace; define non-empty; no `--force` |
</phase_requirements>

## Summary

Phase 5 turns ADR 0010’s inert seam into a real offline-first migration path. Today `magelift env create --dump` writes `seedDump` into the overlay and returns a placeholder `seedDumpStatus` string, but `config.Load` uses `KnownFields(true)` and **`Environment` has no `SeedDump` field** — so `--dump` currently fails validation before the file is written (`field seedDump not found in type config.Environment`). [VERIFIED: probe `go test` on KnownFields + `internal/cli/env.go` + `internal/config/model.go`] That Wave 0 config/schema fix unblocks everything else.

Locked design: hybrid auto-import once from journal status `recorded` after a successful deploy (not inside Pulumi), plus `magelift env import-dump` for retries; status lives under gitignored `.magelift/` mirroring release-journal atomic writes; overwrite safety uses persistent `--yes` only; media is an explicit `env media-sync` command proven on Floci; cutover docs are proven locally with DNS/live deferred to Phase 7.

**Primary recommendation:** Six small plans — (1) config/schema `SeedDump` + journal init on create, (2) seed-dump journal + `env status`, (3) local dump importer + `--yes`/non-empty tests, (4) hybrid auto-hook + `env import-dump`, (5) `env media-sync` + Floci listing diff, (6) cutover runbook + scratch proof + REQUIREMENTS/STATE Phase-7 HUMAN_GATE split.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| `seedDump` path in YAML | API / Backend (CLI + config) | — | Committed operator intent; ADR 0010 config field |
| `seedDumpStatus` state machine | API / Backend (`.magelift/` journal) | — | Runtime status must not pollute committed YAML (D-03) |
| Auto-import after first deploy | API / Backend (CLI lifecycle) | — | After successful `deployflow.Run`, outside Pulumi; once from `recorded` |
| Manual `env import-dump` | API / Backend (CLI) | — | Retries / recovery; `--yes` for non-empty |
| Dump I/O into MySQL | Database / Storage | API / Backend (importer runner) | Local: `magelift dev` MySQL; managed cell → Phase 7 |
| Media tree → object storage | CDN / Static (S3/GCS bucket) | API / Backend (`media-sync`) | Floci proves S3 contract offline |
| Cutover DNS / live verify | — (ops runbook) | Phase 7 HUMAN_GATE | Offline docs + local proof this phase only |
| Maintenance / reindex / rollback steps | API / Backend (existing day-2 CLI) | docs | Reuse `reindex` / `exec` / destroy-restore patterns |

## Project Constraints (from .cursor/rules/)

| Directive | Implication for Phase 5 |
|-----------|-------------------------|
| Serial builds only on this 16 GB Mac | All Go tests/builds: `GOMAXPROCS=1 GOFLAGS=-p=1`; never parallel agent builds; packaging only via `make release-smoke` / `./scripts/release-smoke-local.sh` |
| No full multi-platform goreleaser locally | Phase 5 verification stays unit/integration/Floci; no local release matrix |
| Abort under memory pressure | Do not retry heavy builds under Cursor; reap orphan `go tool compile` only when no intentional build |

Also: AGENTS.md clean-room + serial-build notes; `make check-clean-room` remains a phase-gate check for any new fixtures/docs (no vendored sibling PaaS dump tools).

## Standard Stack

### Core

| Library / Asset | Version | Purpose | Why Standard |
|-----------------|---------|---------|--------------|
| Go + cobra CLI | module Go 1.26.x (repo) | `env import-dump`, `env media-sync`, `env status` | Existing MageLift CLI surface [VERIFIED: `go version`, `internal/cli`] |
| `internal/config` + `go generate ./internal/config` | in-tree | `SeedDump` on `Environment`/`Config` + schema | Strict KnownFields; schema drift gated by `make generate-check` [VERIFIED: Makefile] |
| `internal/releasejournal` pattern | in-tree | Atomic `.magelift/` writes + lock | Proven durable runtime journal [VERIFIED: `internal/releasejournal/store.go`] |
| `magelift dev` MySQL 8.4 Compose | digest-pinned in `localdev` | Offline dump import target | ADR 0005 / compose template [VERIFIED: `internal/localdev/compose.go`] |
| `github.com/aws/aws-sdk-go-v2/service/s3` | v1.105.2 | Media PutObject / ListObjectsV2 | Already in `go.mod`; Floci media tests reuse same client shape [VERIFIED: go.mod + `tests/floci/storage_test.go`] |
| Persistent `--yes` (`-y`) | root PersistentFlags | Destructive overwrite confirmation | Repo-wide pattern; D-04 forbids `--force` [VERIFIED: `internal/cli/root.go`] |

### Supporting

| Library / Asset | Version | Purpose | When to Use |
|-----------------|---------|---------|-------------|
| Host `mysql` client **or** `docker compose exec … mysql` | N/A | Pipe `.sql` / `.sql.gz` into DB | Prefer host client if present; else exec into Compose `database` service (host `mysql` missing on research machine) [VERIFIED: `command -v mysql` → missing; Docker available] |
| `tests/floci` + `make floci-test` | in-tree | Offline S3 media proof | MIGRATE-03 / SC4 |
| Synthetic dump/media fixtures under `testdata/` | new | Unit/integration without PII | Clean-room; never commit real dumps [CITED: ADR 0010 Consequences] |
| Existing day-2: `reindex`, `cache-flush`, `exec` | in-tree | Cutover runbook executable steps | Local/dev proof of maintenance/reindex [VERIFIED: `internal/cli/exec.go`] |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `mysql` CLI / docker exec | `github.com/go-sql-driver/mysql` + SQL parse | Reject for Phase 5 — dumps are scripts (DEFINER, DELIMITER); CLI pipe is Magento-standard [CITED: Adobe KB / community import recipes] |
| Auto-import inside `RunMigrations` | After `deployflow.Run` in CLI lifecycle | Migrations are Magento setup:upgrade candidate; dump is separate ADR 0010 step; sharing migration `waitTimeout` risks false timeout failures |
| Status in YAML | `.magelift/` journal | Locked D-03 — YAML is committed intent only |
| `--force` overwrite | Persistent `--yes` | Locked D-04 |
| `env seed` command name | `env import-dump` | Locked D-01 — collision with `magelift dev seed` |
| New paid AWS pass for managed dump | Phase 7 GCP pass cell | Locked cloud-spend map |

**Installation:** none — no new registry packages for Phase 5 offline path.

**Version verification:** `aws-sdk-go-v2/service/s3 v1.105.2` present in `go.mod` [VERIFIED: go.mod]. Go MySQL driver not recommended → no legitimacy install.

## Package Legitimacy Audit

> No new external packages. Reuse in-tree modules + existing AWS SDK S3 dependency.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| *(none new)* | — | — | — | — | N/A | Approved reuse of `service/s3` already in module |

**Packages removed due to [SLOP] verdict:** none  
**Packages flagged as suspicious [SUS]:** none  

*Do not add `go-sql-driver/mysql` unless a later plan proves CLI pipe insufficient — if added, re-run legitimacy against the Go module proxy and gate behind human verify.*

## Architecture Patterns

### System Architecture Diagram

```text
Operator
  │
  ├─ env create --dump PATH ──► magelift.yaml.seedDump (intent)
  │                              └─ journal: status=recorded (.magelift/)
  │
  ├─ deploy --env X --yes ──► deployflow.Run (lock → … → Health → Record)
  │                              │ success
  │                              ▼
  │                         [auto] if seedDump set && status==recorded
  │                              │
  │                              ▼
  │                         importer: recorded→importing→imported|failed
  │                              │
  │                         (lock already released — dump I/O outside deploy lock)
  │
  ├─ env import-dump ──► same importer (retries; --yes if non-empty & not first recorded path)
  ├─ env media-sync --source DIR ──► S3/GCS media bucket (Floci offline)
  └─ env status ──► merge YAML seedDump path + journal status/reason
```

### Recommended Project Structure

```text
internal/
├── config/                 # SeedDump on Environment + Config; regenerate schema
├── seeddump/               # NEW: journal store + status machine + non-empty probe helpers
├── dumpimport/             # NEW: importer (gunzip/sql → mysql; schema-replace)
└── cli/
    ├── env.go              # create: init journal; add status / import-dump / media-sync
    └── lifecycle.go        # post-successful-deploy auto-import hook (D-01/D-02)

testdata/fixtures/migrate/  # tiny .sql / .sql.gz + media tree (no PII)
.magelift/seed-dumps/       # runtime only (gitignored) — recommended layout
docs/migrating-from-paas.md # cutover runbook section
.planning/.../scratch/      # recorded local cutover proof
```

### Pattern 1: Journal status (not YAML)

**What:** Mutable seed status file under `.magelift/seed-dumps/<env>.json` with atomic write + exclusive lock (copy `releasejournal` `acquireLock` / `writeAtomic` pattern). Fields: `status`, `reason`, `updatedAt`, optional `dumpPath` echo for diagnostics.  
**When to use:** Always for MIGRATE-02.  
**Example:** Mirror `internal/releasejournal/store.go` single-document JSON (not append-only jsonl) because status is a state machine, not a release history. [VERIFIED: releasejournal pattern; discretion picks filename]

### Pattern 2: Hybrid auto-import after deploy success

**What:** In `runDeploymentWithOptions`, after `deployflow.Run` returns nil, if resolved `SeedDump` nonempty and journal status is `recorded`, run importer once. Later deploys no-op on status≠`recorded`.  
**When to use:** Cloud and local deploy paths that complete Magento install/upgrade candidate.  
**Anti-coupling:** Do not call importer from Pulumi components or from `RunMigrations`. [VERIFIED: ADR 0010; D-01]

### Pattern 3: `--yes` schema-replace

**What:** Non-empty DB → exit non-zero without `o.yes`. With `--yes`: `DROP DATABASE`/`CREATE DATABASE` (or drop all tables) then import so interrupted re-runs converge.  
**Non-empty definition (recommended):** target schema has ≥1 base table via `information_schema.tables` (or `SHOW TABLES` nonempty). Magento markers (`store`, `core_config_data`) are sufficient but broader “any user table” is safer for partial imports. [ASSUMED: exact SQL — implementer picks one and tests]  
**First `recorded` auto-import:** Allowed without `--yes` even if Magento install left tables — operator intent is already recorded via `--dump` (auto path is not a “re-run”). Explicit `import-dump` when status is `imported`/`failed`/`importing` requires `--yes` if nonempty (D-04).

### Pattern 4: Media sync merge + listing diff

**What:** Walk `--source`; map keys as: if path under `pub/media/`, strip to relative under that root; else treat source root as media root. Default **merge** (upload/overwrite keys; do not wipe bucket). Document wipe as future flag if needed. Verify SC4: ListObjectsV2 key set equals expected relative file set (empty diff). Prove against Floci endpoint env (`MAGELIFT_FLOCI=1`). [VERIFIED: Floci storage test client pattern]

### Anti-Patterns to Avoid

- **Inventing `--force` / `--confirm-overwrite`:** Violates D-04 and repo muscle memory.
- **Writing status into `magelift.yaml`:** Violates D-03; breaks CI/git honesty.
- **Naming command `env seed`:** Collides with `dev seed` (D-01).
- **Import inside Pulumi create:** Rejected by ADR 0010.
- **Holding deploy lock during large dump I/O:** Timeout footgun; run after lock release (see Pitfalls).
- **Committing real dumps/PII fixtures:** ADR 0010 + clean-room.
- **Marking MIGRATE-04 Complete without Phase 7 DNS HUMAN_GATE split:** D-06.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Atomic `.magelift/` status writes | Ad-hoc open/write | `releasejournal`-style lock + rename | Partial writes / races under concurrent deploy+status |
| Magento SQL dump execution | Go SQL parser / statement splitter | `mysql` CLI pipe (+ gunzip, DEFINER strip) | Triggers/DEFINER/DELIMITER edge cases [CITED: Adobe Commerce KB restore dump] |
| S3 upload + list | Custom HTTP | Existing aws-sdk-go-v2 S3 client | Already certified via Floci tests |
| Destructive confirm UX | New flags | Persistent `--yes` | Established CLI contract |
| Media bucket wiring | New IaC this phase | Existing stack `mediaBucket` output / Floci bucket | Phase 5 syncs into provisioned or test bucket |

**Key insight:** Phase 5 is CLI + journal + offline proof; managed dump cell and live DNS are Phase 7 spend, not new architecture.

## Runtime State Inventory

> Migration from placeholder status string → real importer + journal.

| Category | Items Found | Action Required |
|----------|-------------|-----------------|
| Stored data | No existing seed-dump journal files (feature not shipped). Release journals under `.magelift/releases/*.jsonl` unrelated. | Create new `.magelift/seed-dumps/` writers; no data migration |
| Live service config | None for seedDump (inert). Floci/LocalStack ephemeral. | None |
| OS-registered state | None | None — verified by absence of launchd/systemd MageLift seed units [ASSUMED: no OS registration found in repo docs] |
| Secrets/env vars | Dump paths may contain PII; `.magelift/local.env` holds local admin password only | Keep dumps outside git; do not log dump contents; journal stores status/reason only |
| Build artifacts | Placeholder string at `internal/cli/env.go:144`; schema without `seedDump` | Code edit + `make generate`; no published artifact rename |

**Nothing found in category (where empty):** Live service config — none for this feature; OS-registered state — none in-repo.

## Common Pitfalls

### Pitfall 1: Deploy timeout vs dump I/O
**What goes wrong:** Large import runs under deploy lock or shares migration `waitTimeout` (30m on ECS candidate wait) → deploy fails / lock stuck while DB half-imported. [VERIFIED: `waitTimeout: 30 * time.Minute` in `internal/cloud/aws/operations/deployment.go`; Fargate stopTimeout community notes MEDIUM]  
**Why it happens:** Treating dump as another migration step.  
**How to avoid:** Auto-import **after** successful `deployflow.Run` (lock released). Phase 5 fixtures stay small. Managed huge dumps → Phase 7 cell; async job only if timeouts prove painful (deferred).  
**Warning signs:** Deploy duration >> infra+migrate; status stuck `importing`.

### Pitfall 2: Inventing `--force`
**What goes wrong:** Parallel confirmation flags break scripts and Phase 4 muscle memory.  
**How to avoid:** Only persistent `--yes`; refuse otherwise with exit 2-style invalid.  
**Warning signs:** Help text mentioning `--force`.

### Pitfall 3: Journal vs YAML confusion
**What goes wrong:** Status committed to YAML → git noise, stale CI, dishonest “imported” in PRs.  
**How to avoid:** YAML = path only; journal = status; `env status` merges. Hosted CI may upload `.magelift/` as artifact — never commit. [VERIFIED: `.gitignore` has `.magelift/`]

### Pitfall 4: `seedDump` config field missing (shipped bug)
**What goes wrong:** `env create --dump` cannot Load prospective config today. [VERIFIED: KnownFields probe]  
**How to avoid:** Wave 0 add `SeedDump` to `Environment` + `Config`, regenerate schema, test create→Load→Resolve.

### Pitfall 5: Auto re-import on every deploy
**What goes wrong:** Data loss / wipe of production-like data.  
**How to avoid:** D-02 — only `recorded`; document in help + tests.

### Pitfall 6: Naming collision with `dev seed`
**What goes wrong:** Operators run wrong command; docs confuse Magento setup:install with dump import.  
**How to avoid:** Command name `import-dump` only (D-01).

### Pitfall 7: Claiming MIGRATE-04 / SC5 complete from local-only
**What goes wrong:** ROADMAP SC1/SC5 mention managed/live cutover language.  
**How to avoid:** D-06 split — local scratch proves executable steps; DNS/live HUMAN_GATE Phase 7; update REQUIREMENTS/STATE explicitly.

### Pitfall 8: Host without `mysql` client
**What goes wrong:** Importer assumes `mysql` on PATH; research machine has Docker but no client. [VERIFIED: environment probe]  
**How to avoid:** Resolve runner: `mysql` if present, else `docker compose -f .magelift/compose.local.yml exec -T database mysql …` using localdev credentials.

## Code Examples

### env create must stop returning placeholder-only success

```go
// Source: internal/cli/env.go (current — replace status story)
result["seedDump"] = dumpPath
result["seedDumpStatus"] = "recorded" // from journal after init, not ADR prose placeholder
```

### Post-deploy auto-import seam (recommended)

```go
// After deployflow.Run succeeds in runDeploymentWithOptions — conceptual
if err := o.maybeAutoImportSeedDump(ctx, environment, planned); err != nil {
    return infrastructureResult{}, err // or return deploy OK + surface import failure? Prefer fail command if auto-import fails after marking failed in journal
}
```

Honesty recommendation: if auto-import fails, journal=`failed`+reason and **deploy command exits non-zero** so operators do not assume data landed (aligns with honesty-first). Infra/Magento may already be up — document recovery via `env import-dump --yes`.

### Non-empty guard

```go
// Pseudocode — implement in dumpimport
if nonempty && !yes && !firstRecordedAuto {
    return invalid(errors.New("importing into a non-empty database requires --yes"))
}
```

### Floci media listing diff (extend existing pattern)

```go
// Source: tests/floci/storage_test.go client setup
client := s3.NewFromConfig(configuration, func(options *s3.Options) {
    options.BaseEndpoint = awssdk.String(endpoint)
    options.UsePathStyle = true
})
// PutObject each file; ListObjectsV2; diff keys vs walk(source)
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Placeholder `seedDumpStatus` string in create output | Journal state machine + real importer | Phase 5 | MIGRATE-01/02 |
| Manual `mysql < dump.sql` only | CLI `import-dump` + one-shot auto | ADR 0010 → Phase 5 | Agency DX |
| Import in Pulumi | After first successful deploy | ADR 0010 | Avoids IaC timeout/state bloat |
| Paid AWS dump cell this milestone | Offline MySQL + Floci; managed on Phase 7 GCP pass | ROADMAP cloud spend map | No fourth paid pass |

**Deprecated/outdated:**
- Treating `env.go:144` placeholder as “done” for ADR 0010 — replace in Phase 5.
- Marking cutover Complete without HUMAN_GATE split (D-06).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Non-empty = any base table in target schema | Pattern 3 | Too strict blocks auto after Magento install — mitigate via recorded-auto exception |
| A2 | Journal path `.magelift/seed-dumps/<env>.json` (single JSON) | Pattern 1 | Rename is cheap if tests own the path |
| A3 | No OS-level MageLift seed registrations | Runtime State | Unlikely; verify on implementer host if needed |
| A4 | Auto-import failure should fail the deploy CLI exit code | Code Examples | Alternate: deploy success + failed status — worse honesty; stick with fail |

**If this table is empty:** N/A — four assumed items need implementer confirmation only where marked; CONTEXT locks do not need re-ask.

## Open Questions

1. **Importer transport for Phase 5 offline** — RESOLVED (CONTEXT discretion + research): use local `mysql` CLI or `docker compose exec` into `magelift dev` MySQL; managed ECS one-off task is Phase 7 cell, not this phase.
2. **`--force` vs `--yes`** — RESOLVED (D-04): `--yes` only.
3. **Status in YAML vs journal** — RESOLVED (D-03): journal under `.magelift/`; YAML path only.
4. **DNS / live cutover evidence** — RESOLVED (D-06): Phase 7 HUMAN_GATE; local scratch this phase.
5. **Managed-instance SC1 proof** — RESOLVED (ROADMAP cloud spend + D-06): rides Phase 7; do not buy AWS pass in Phase 5.
6. **`seedMedia` auto** — RESOLVED (D-05 deferred): out of scope.
7. **Production extra gate beyond `--yes`** — RESOLVED (discretion prefer no): do not add unless reusing existing `protection` flag for destroy-class ops; import overwrite stays `--yes` only.
8. **Exact journal filename** — RESOLVED for planning (discretion recommendation): `.magelift/seed-dumps/<env>.json`; planner may rename if tests assert the path.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All packages/tests | ✓ | go1.26.5 | — |
| Docker | Local MySQL + Floci | ✓ | 29.6.2 | — |
| Host `mysql` client | Dump import | ✗ | — | `docker compose exec -T database mysql` via Compose file |
| Floci / LocalStack endpoint | Media sync proof | probe at test time | `MAGELIFT_FLOCI_ENDPOINT` default `http://localhost:4566` | Skip Floci tests if down; unit-test sync with fake S3 API |
| Paid AWS account | Managed dump | N/A this phase | — | Phase 7 GCP pass |
| Serial build discipline | All `go test` | ✓ (policy) | GOMAXPROCS=1 | Abort if memory pressure |

**Missing dependencies with no fallback:** none for offline Phase 5 scope.

**Missing dependencies with fallback:** host `mysql` → docker exec.

## Validation Architecture

> `workflow.nyquist_validation` is enabled in `.planning/config.json`.

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` (race optional via Makefile `test`) |
| Config file | none — package tests; Floci via `//go:build floci` |
| Quick run command | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/seeddump/ ./internal/dumpimport/ ./internal/cli/ ./internal/config/ -count=1` |
| Full suite command | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/... -count=1` plus `make floci-test` when Docker/Floci up; `make check-clean-room` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MIGRATE-01 | create `--dump` persists SeedDump; import lands tables in local MySQL | unit + integration | `go test ./internal/cli/ ./internal/dumpimport/ -count=1 -run 'SeedDump\|ImportDump\|Create.*Dump'` | ❌ Wave 0 |
| MIGRATE-02 | status recorded→importing→imported; corrupt→failed+reason | unit | `go test ./internal/seeddump/ ./internal/cli/ -count=1 -run 'SeedDumpStatus\|EnvStatus'` | ❌ Wave 0 |
| MIGRATE-03 | media-sync listing diff empty | unit + floci | `go test ./internal/cli/ -count=1 -run MediaSync`; `make floci-test` (media sync case) | ❌ Wave 0 |
| MIGRATE-04 | runbook sections present; scratch proof recorded | docs + manual-recorded | grep runbook headings; scratch log path under phase dir | ❌ Wave 0 (docs partial) |
| MIGRATE-05 | nonempty refuse without `--yes`; `--yes` schema-replace converges | unit | `go test ./internal/dumpimport/ -count=1 -run 'NonEmpty\|Yes\|Converge'` | ❌ Wave 0 |
| D-02 | second deploy does not re-import | unit | `go test ./internal/cli/ -count=1 -run 'AutoImportOnce\|RecordedOnly'` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** focused package tests with `GOMAXPROCS=1 GOFLAGS=-p=1`
- **Per wave merge:** `./internal/config` + `seeddump` + `dumpimport` + `cli` + `make generate-check`
- **Phase gate:** above green + Floci media proof (or documented skip if Floci down) + scratch cutover log + `make check-clean-room` before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] Add `SeedDump` to `config.Environment` + `config.Config`; `make generate` / schema update; create `--dump` Load test
- [ ] `internal/seeddump/` journal package + tests
- [ ] `internal/dumpimport/` importer + nonempty/`--yes` tests + tiny `testdata/fixtures/migrate/*.sql.gz`
- [ ] CLI: `env status`, `env import-dump`, `env media-sync`; remove placeholder status prose
- [ ] Lifecycle auto-import once-from-recorded test (fake steps + fake importer)
- [ ] Floci or fake-S3 media listing-diff test
- [ ] Cutover section in `docs/migrating-from-paas.md` + scratch proof template
- [ ] REQUIREMENTS/STATE note: MIGRATE-04 DNS/live + managed dump → Phase 7 HUMAN_GATE

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | — |
| V3 Session Management | no | — |
| V4 Access Control | partial | Reuse env `protection` only for destroy-class ops; import overwrite = `--yes` (discretion: no extra prod gate) |
| V5 Input Validation | yes | Validate dump path readability; confine media `--source` (no `..` escape); KnownFields for YAML |
| V6 Cryptography | no new | Do not log dump contents; `.magelift/` mode 0600 files like release journal |

### Known Threat Patterns for dump/media migration

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Silent overwrite of live DB | Tampering / Elevation | Refuse nonempty without `--yes`; auto only from `recorded` |
| PII dump committed to git | Information Disclosure | Fixtures synthetic; ADR warning; gitignore dumps |
| Path traversal via `--source` / dump path | Tampering | Clean + absolute-within-root checks |
| Status spoofing in YAML | Spoofing | Status only in journal; merge on read |
| Half-applied dump left as “success” | Tampering | `importing`→`failed`+reason; schema-replace on `--yes` retry |

## Recommended Plan Wave Breakdown

| Plan | Focus | Primary reqs | Notes |
|------|-------|--------------|-------|
| **05-01** | Config `SeedDump` + schema generate + create init journal `recorded` + create tests | MIGRATE-01/02 (seam) | Unblocks KnownFields; remove inert placeholder as create output source of truth |
| **05-02** | `internal/seeddump` journal + `magelift env status` merge YAML+journal | MIGRATE-02 | Discretion path `.magelift/seed-dumps/<env>.json` |
| **05-03** | `internal/dumpimport` local MySQL importer + nonempty/`--yes`/converge tests + fixtures | MIGRATE-01/05 | docker-exec fallback; DEFINER strip optional |
| **05-04** | `env import-dump` + post-deploy auto-import once-from-recorded | MIGRATE-01/02/05 | Hook after successful `deployflow.Run`; never Pulumi |
| **05-05** | `env media-sync --source` + Floci/unit listing-diff | MIGRATE-03 | Merge default; document key mapping in help |
| **05-06** | Cutover runbook in `docs/migrating-from-paas.md` + local scratch proof + REQUIREMENTS/STATE Phase-7 HUMAN_GATE split + capability-matrix honesty if needed | MIGRATE-04 | No paid pass; clean-room check |

**Recommended plan count: 6.**

## Sources

### Primary (HIGH confidence)
- `.planning/phases/05-data-migration-cutover/05-CONTEXT.md` — D-01..D-06
- `docs/adr/0010-database-dump-seed.md` — after-first-deploy contract
- `internal/cli/env.go` — create `--dump` + placeholder status
- `internal/config/config.go` / `model.go` — KnownFields; missing SeedDump (probe verified)
- `internal/releasejournal/store.go` — journal pattern
- `internal/deploy/orchestrator.go` + `internal/cli/lifecycle.go` — deploy success seam
- `internal/localdev/compose.go` — MySQL 8.4 local target
- `tests/floci/storage_test.go` — media S3 Floci
- `internal/cli/root.go` — persistent `--yes`; env command group
- `.gitignore` — `.magelift/`
- `.cursor/rules/serial-builds-only.mdc` / AGENTS.md — serial builds

### Secondary (MEDIUM confidence)
- Adobe Experience League DB snapshot restore (DEFINER strip + mysql pipe) — [CITED: experienceleague.adobe.com]
- Community Magento dump import recipes (drop/recreate + gunzip\|mysql)
- ECS Fargate stopTimeout / long-running task notes — informs timeout pitfall, not Phase 5 local path

### Tertiary (LOW confidence)
- Exact multi-hour dump durations for “typical” Magento catalogs — irrelevant to fixture-sized Phase 5 proofs

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — reuse verified in-repo; no new packages
- Architecture: HIGH — CONTEXT locks + deploy/config seams verified
- Pitfalls: HIGH for config/`--yes`/journal; MEDIUM for managed-timeout lore

**Research date:** 2026-07-29  
**Valid until:** 2026-08-28 (stable CLI/config domain; revisit if deploy orchestration changes)
