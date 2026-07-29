# Phase 4: Brownfield Onramp — PaaS Import & ece-tools Parity - Research

**Researched:** 2026-07-29
**Domain:** Go CLI PaaS→`magelift.yaml` importers + PHP build SCD/patches parity
**Confidence:** HIGH (codebase/current-state); MEDIUM (ACC/Upsun public config shapes & Magento SCD/patch public CLI)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Use synthetic config-only fixtures under `testdata/fixtures/{acc,upsun}/` as the CI proof path for IMPORT-01/02 (mapping + fail-loud unmapped keys). — **Reversibility:** reversible — fixtures can expand without API change
- **D-02:** Optional maintainer soak via `MAGELIFT_IMPORT_FIXTURE_ACC` / `MAGELIFT_IMPORT_FIXTURE_UPSUN` (config-root paths only); never vendor sibling/`ece-tools` trees into this repo; soak tests must skip cleanly when unset. — **Reversibility:** reversible
- **D-03:** Expose `magelift init --from-acc` and `magelift init --from-upsun` as mutually exclusive flags on existing `init` (literal IMPORT-01/02 wording). Do not add `magelift import` or unify under `--from <enum>` this phase. — **Reversibility:** one-way — published REQUIREMENTS/docs/CLI help become the contract operators learn
- **D-04:** Refuse if `magelift.yaml` (or `--config` path) already exists (exit 2), matching bare `init`. Overwrite only with `--yes` (MageLift destructive-confirmation pattern — do not invent `--force`). Optional `--output PATH` may write a side file for review without clobbering the default path. — **Reversibility:** costly — changing refuse/`--yes` semantics breaks scripts and operator muscle memory
- **D-05:** On any unmapped key: write mapped `magelift.yaml` + durable sidecar unmapped-keys report, exit non-zero (refuse success). Full map of a supported shape → write YAML only, exit 0. Never exit 0 with only stderr warnings; never default-lenient `--strict`. Soft path later, if needed, must be explicit opt-out (`--allow-unmapped`), not the default. — **Reversibility:** one-way — exit-code + sidecar path become CI/automation contract
- **D-06:** Ship `docs/ece-parity.md` with no blank rows (every hook/env closed or intentional-gap) AND close high-traffic half-done gaps already implied by shipping: clean-room patch apply, SCD strategy/threads wired through Go→PHP (not locale/theme only), plus common env→`magelift.yaml` mapping covered by D-07. Do **not** attempt full ece-tools behavioral clone this phase. — **Reversibility:** costly — parity matrix rows and php-test gates become honesty commitments
- **D-07:** Map structural PaaS YAML (app/services/routes/cron) plus a frozen documented v1 allowlist of store-breaker vars: crypt → encryption secret ref; routes/UPDATE_URLS intent → domain/base-URL; DB/Redis/OpenSearch/AMQP relationships → existing capability / `MAGENTO_DC_*` seams; SCD_* → existing `build.staticContent`. Everything else = intentional ECE-04 gap rows — not silent drops. — **Reversibility:** costly — allowlist is a published contract; shrinking it breaks “supported shapes without hand edits”

### Claude's Discretion
- Exact sidecar report filename/path convention (recommend adjacent `magelift.unmapped.md` or `.magelift/import-unmapped.md` — planner picks with tests)
- Fixture file layout details within `testdata/fixtures/{acc,upsun}/`
- Which long-tail stage vars get intentional-gap wording in `docs/ece-parity.md` beyond the locked allowlist

### Deferred Ideas (OUT OF SCOPE)
- Live GCP create / PSA soak — Phase 7
- Dump/media cutover and `seedDump` runner — Phase 5
- Brownfield attach of existing VPC/RDS — Phase 8
- Exhaustive ACC/Upsun env parity / full ece-tools behavioral clone — not in v1.0.0-rc scope
- Hosted Actions QUALITY-06 green — Phase 1 HUMAN_GATE
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| IMPORT-01 | `magelift init --from-acc` → valid `magelift.yaml` | D-03 flags; ACC fixture shapes; extend `initCommand`; mapper package |
| IMPORT-02 | `magelift init --from-upsun` → valid `magelift.yaml` | Same CLI surface; Upsun/Platform.sh fixture dual; shared mapper core |
| IMPORT-03 | Translate app/services/routes/cron; report every unmapped key | Structural walk + D-05 sidecar; cron destination schema gap |
| IMPORT-04 | Generated YAML passes schema + `config validate` | Emit against `starterConfig`/`schema/magelift.schema.json`; gate in tests |
| IMPORT-05 | Reviewable file; never accept foreign schema as deploy input | Write MageLift YAML only; reject `.magento*` / `.platform*` as `--config` |
| IMPORT-06 | Clean-room policy | `docs/provenance.md` + knowledge lesson; no vendored sibling source; fixture-only CI |
| ECE-01 | `docs/ece-parity.md` matrix, no blank rows | New doc; hook/env rows closed or intentional-gap |
| ECE-02 | `magento-cloud-patches`-style patch apply | Clean-room `m2-hotfixes` apply in PHP build; php-test |
| ECE-03 | SCD locales/themes/strategy/threads | Extend Go protocol + PHP `LifecyclePlan` beyond locale×theme |
| ECE-04 | Env-var mapping documented | D-07 allowlist + intentional gaps in ece-parity / migrating docs |
</phase_requirements>

## Summary

Phase 4 is mostly **greenfield product surface on existing seams**: `init` today only writes a hardcoded `starterConfig` and refuses if the config path exists (exit 2). There is **no importer package**, **no `docs/ece-parity.md`**, and **no patch applicator** in `build/`. Static content already flows Go→PHP as a locale×theme matrix, but **strategy and threads are absent** from `internal/build/plan`, the runner protocol, and `LifecyclePlan.php`.

Importers must hang off existing `init` as `--from-acc` / `--from-upsun` (never `--from`, which collides with `promote --from`), refuse overwrite unless `--yes`, and on unmapped keys write YAML + a durable sidecar then exit non-zero. CI proof is synthetic fixtures under `testdata/fixtures/{acc,upsun}/`; sibling `../ece-tools` / `../magento-cloud-patches` may inform behavior only.

**Primary recommendation:** Six small plans — (1) fixtures + foreign-schema rejection, (2) init CLI flags/`--yes`, (3) shared mapper + ACC/Upsun + unmapped sidecar, (4) SCD strategy/threads end-to-end, (5) clean-room m2-hotfixes patch apply + php-test, (6) `docs/ece-parity.md` + D-07 allowlist docs + provenance/migration doc updates. Resolve the D-04 `--output` vs persistent format-flag collision before locking CLI help (see Open Questions).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| PaaS YAML → `magelift.yaml` mapping | API / Backend (CLI library) | — | Offline file transform; no cloud APIs |
| Init CLI flags / exit codes | API / Backend (CLI) | CDN / Static (generated `docs/cli-reference.md`) | Cobra command tree is source of truth |
| Unmapped-key sidecar report | API / Backend | Browser / Client (operator review) | Durable on disk for CI/automation |
| Foreign-schema rejection | API / Backend (`config.Load` / CLI) | — | Deploy path must only accept MageLift schema |
| SCD strategy/threads | API / Backend (Go plan) + PHP build runner | — | Prepare protocol already owns static content |
| Patch apply (`m2-hotfixes`) | PHP build package | — | Runs during prepare/build after composer install |
| ECE parity matrix | CDN / Static (`docs/`) | — | Honesty artifact; gates code claims |
| Runtime `MAGENTO_DC_*` injection | Cloud adapters | — | Already owned by `internal/platform` + AWS/GCP runtimes; importer maps *intent* to YAML seams, not raw env dumps |

## Project Constraints (from .cursor/rules/)

| Directive | Implication for Phase 4 |
|-----------|-------------------------|
| Serial builds only (`GOMAXPROCS=1`, `GOFLAGS=-p=1`) | Never parallel `go test`/`go build` matrices locally; prefer focused package tests |
| No full goreleaser / multi-platform locally | Phase 4 needs no packaging; skip release-smoke unless planner adds unrelated work |
| Abort on memory pressure | Keep verification to `go test` on touched packages + `make php-test` |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go + `testing` | go 1.26.0 / toolchain 1.26.5 [VERIFIED: go.mod] | Importer + CLI tests | Existing module |
| `github.com/spf13/cobra` | v1.10.2 [VERIFIED: go.mod] | `init` flags | Existing CLI |
| `go.yaml.in/yaml/v4` | v4.0.0-rc.6 [VERIFIED: go.mod] | Parse PaaS YAML / emit MageLift YAML | Already used by `internal/config` |
| PHPUnit / PHPStan / Psalm | via `build/` Composer [VERIFIED: Makefile `php-test`] | Patch + SCD php-tests | Existing PHP gate |
| `schema/magelift.schema.json` | repo schema [VERIFIED: schema file] | IMPORT-04 validation target | Generated/kept in sync with config models |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| OS `patch` (or PHP `proc_open` to `patch -p1`) | host/tooling [ASSUMED] | Apply `m2-hotfixes/*.patch` | ECE-02 clean-room apply after `composer install` |
| Existing `internal/config.Load` / `config validate` | repo | Gate generated YAML | After every successful import write |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `init --from-acc/--from-upsun` | `magelift import` or `--from <enum>` | Forbidden by D-03; also `--from` collides with `promote --from` |
| `--force` overwrite | `--yes` | Forbidden by D-04; repo already uses `--yes` |
| Vendoring `ece-tools` / `magento-cloud-patches` | Clean-room reimplementation | Forbidden by D-02 / IMPORT-06 / provenance |
| Exhaustive env parity | D-07 allowlist + intentional gaps | D-06 / deferred ideas |

**Installation:** No new Go module dependencies required for importers. Prefer stdlib + existing yaml/cobra.

**Version verification:** `go list -m go.yaml.in/yaml/v4` → `v4.0.0-rc.6`; `go list -m github.com/spf13/cobra` → `v1.10.2` [VERIFIED: go.mod / go list].

## Package Legitimacy Audit

> No new external packages are recommended for this phase.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| — | — | — | — | — | N/A | No installs |

**Packages removed due to [SLOP] verdict:** none  
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```text
Operator cwd (ACC or Upsun fixture)
        │
        ▼
 magelift init --from-acc|--from-upsun [--yes] [--config-out PATH]
        │
        ├─ refuse if target exists && !--yes  → exit 2
        ├─ mutually exclusive --from-*        → exit 2 (invalid)
        │
        ▼
 internal/paasimport (new)
   read: .magento.app.yaml | .platform.app.yaml
         .magento/services.yaml | .platform/services.yaml
         .magento/routes.yaml   | .platform/routes.yaml
         .magento.env.yaml      | .platform.env.yaml / Upsun equiv
        │
        ├─ structural map (app/services/routes/cron)
        ├─ D-07 allowlist env → magelift.yaml fields
        └─ collect unmapped keys
        │
        ├─ write magelift.yaml (or --config-out side file)
        ├─ if unmapped: write sidecar report → exit non-zero
        └─ if fully mapped: exit 0
        │
        ▼
 Operator: magelift config validate / diff / edit
        │
        ▼
 Deploy path: config.Load rejects foreign schemas (IMPORT-05)
```

Build parity path (orthogonal to import):

```text
magelift.yaml build.staticContent {locales,themes,strategy,threads}
        → internal/build/plan.PrepareRequest
        → runner protocol StaticContent (+ strategy/threads fields)
        → MageLift\Build LifecyclePlan::staticContentCommands
        → bin/magento setup:static-content:deploy --language … --theme … [-s …] [-j …]

composer install → clean-room PatchApplier → m2-hotfixes/*.patch (alpha order)
```

### Recommended Project Structure

```text
internal/paasimport/          # new: pure library, no cobra
  doc.go
  source.go                   # detect ACC vs Upsun roots + file set
  map.go                      # structural + allowlist mapping
  unmapped.go                 # report model + render markdown
  acc.go / upsun.go           # thin source adapters
  map_test.go                 # table-driven fixture tests

internal/cli/root.go          # extend initCommand only
testdata/fixtures/acc/...     # D-01 CI fixtures (config-only)
testdata/fixtures/upsun/...
docs/ece-parity.md            # ECE-01
build/src/Magento/…           # SCD + PatchApplier
```

### Pattern 1: Init flags on existing command (not new verb)

**What:** Local bool flags `--from-acc` / `--from-upsun` on `init`; mutual exclusion in `RunE`; reuse persistent `--yes` and `--config`.  
**When to use:** Always for IMPORT-01/02.  
**Example:**

```go
// Source: internal/cli/root.go initCommand pattern (extend; do not invent --force)
var fromACC, fromUpsun bool
cmd := &cobra.Command{Use: "init", …}
cmd.Flags().BoolVar(&fromACC, "from-acc", false, "generate magelift.yaml from Adobe Commerce Cloud config")
cmd.Flags().BoolVar(&fromUpsun, "from-upsun", false, "generate magelift.yaml from Upsun/Platform.sh config")
```

### Pattern 2: Fail-loud unmapped report (D-05)

**What:** Always write the best-effort mapped YAML; if `len(unmapped)>0`, also write sidecar and return `invalid(...)` / non-zero (exit 2 family).  
**When to use:** Any import with residual keys.  
**Recommendation (discretion):** Write sidecar next to the output YAML as `magelift.unmapped.md` (same basename stem when using `--config-out`). Prefer adjacent over `.magelift/` so operators see it in `ls` beside the candidate config.

### Pattern 3: SCD Go→PHP protocol extension

**What:** Keep locale×theme cartesian product; add optional top-level `strategy` (enum `quick|standard|compact`) and `threads` (int) on `build.staticContent`, thread through `PrepareRequest` / PHP `staticContentCommands` as `-s` / `-j`.  
**When to use:** ECE-03.  
**Source:** Adobe Magento SCD CLI docs map `SCD_STRATEGY`→`-s`, `SCD_THREADS`→`-j` [CITED: experienceleague.adobe.com commerce-operations static-view-file-deployment].

### Anti-Patterns to Avoid

- **`--from` enum on init:** Collides with `promote --from` (source environment) [VERIFIED: `internal/cli/releases.go`].
- **Inventing `--force`:** Violates D-04 and repo `--yes` pattern [VERIFIED: root PersistentFlags].
- **Exit 0 with stderr-only unmapped warnings:** Violates D-05 / TRUST-02 lineage.
- **Accepting `.magento.app.yaml` as `--config`:** Violates IMPORT-05; `config.Load` expects `schemaVersion: 1` MageLift shape [VERIFIED: `internal/config`].
- **Copying ece-tools / magento-cloud-patches source into the repo:** Violates provenance + D-02.
- **Dumping full `MAGENTO_DC_*` blocks into YAML:** Runtime already injects these from capabilities [VERIFIED: `internal/platform/env.go`]; importer maps *relationships/intent* to target/capability fields and secret refs.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| YAML parse/emit | Custom tokenizer | `go.yaml.in/yaml/v4` | Already in module; edge cases covered |
| CLI flag parsing | Custom argv | cobra local flags on `init` | Matches repo |
| Config validation | Ad-hoc checks only | `config.Load` + `magelift config validate` + schema | IMPORT-04 gate |
| SCD locale×theme matrix | Reinvent in PHP | Existing `internal/build/plan.staticContent` | Already ships |
| Full ece-tools clone | Port StageConfig tree | Document gaps + close D-06 half-dones | Explicit non-goal |
| Quality Patches Tool reimplementation | Bundle Adobe patch DB | Map `QUALITY_PATCHES` → intentional gap **or** thin documented list→apply later; **must** still apply `m2-hotfixes` | ECE-02 “-style”; D-06 not full clone |

**Key insight:** The expensive honesty work is **unmapped reporting + parity matrix**, not another orchestration framework.

## Common Pitfalls

### Pitfall 1: `--output PATH` vs persistent `--output` format
**What goes wrong:** D-04 names `--output PATH`, but root already defines `--output/-o` as `table|json|yaml` [VERIFIED: `internal/cli/root.go`]. Local `StringVar` named `output` conflicts.  
**Why it happens:** Context used “output” for side-file intent.  
**How to avoid:** Prefer init-local `--config-out` (or `--write`) for the side file; document deviation under Open Questions; keep global `-o` for format.  
**Warning signs:** `gendocs` / cobra panic on duplicate flags; `init -o json` suddenly means a path.

### Pitfall 2: `promote --from` collision
**What goes wrong:** Unifying import under `--from <enum>` breaks promote workflows and CI templates that pass `--from staging` [VERIFIED: `internal/cli/ci.go`].  
**How to avoid:** Keep D-03 literal `--from-acc` / `--from-upsun`.

### Pitfall 3: Silent drop of PaaS keys
**What goes wrong:** Partial mapper “succeeds” and operators deploy incomplete config.  
**How to avoid:** D-05: write YAML + sidecar + non-zero; tests assert every fixture unmapped key appears in the report.

### Pitfall 4: Cron has no MageLift schema home
**What goes wrong:** IMPORT-03/D-07 require cron translation, but schema/model have no cron schedule list—only cron *workload* sizing/ops [VERIFIED: schema/model grep].  
**How to avoid:** Add a minimal portable `application.cron` (or top-level `cron:`) list in schema **or** treat non-standard shell crons as unmapped and Magento-default cron as “covered by runtime cron service” with an ece-parity intentional-gap row—**planner must pick one and make IMPORT-03 tests pass**.

### Pitfall 5: SCD strategy/threads only in YAML
**What goes wrong:** Document `build.staticContent.strategy` but forget protocol/PHP → false ECE-03 claim.  
**How to avoid:** Single vertical slice: schema model → `plan.staticContent` → runner JSON → `LifecyclePlan` → phpunit asserts argv contains `-s`/`-j`.

### Pitfall 6: Clean-room bleed
**What goes wrong:** Copying identifiers/docs from sibling trees into MageLift.  
**How to avoid:** Fixtures are synthetic; observe public Adobe docs + behavioral evidence only; record provenance; CI check that `vendor/` / ece-tools paths are not vendored.

### Pitfall 7: `--yes` semantics for bare init
**What goes wrong:** Bare `init` historically has no overwrite path; adding `--yes` for import must also define bare-init overwrite consistently (or refuse `--yes` without `--from-*`).  
**How to avoid:** Spec: `--yes` only enables overwrite when writing the config path; without `--from-*`, `--yes` may rewrite starter config (document) **or** remain import-only—pick one in plan and test.

### Pitfall 8: `MAGENTO_DC_*` misunderstanding
**What goes wrong:** Importer emits plaintext DB hosts into YAML that deploy ignores or treats as secrets.  
**How to avoid:** Map relationships → catalog/capability intent + `encryptionKeySecretArn` (or provider equivalent) placeholders; leave runtime env emission to cloud adapters.

## Code Examples

### Current init refuse-if-exists [VERIFIED: codebase]

```go
// Source: internal/cli/root.go initCommand
if _, err := os.Stat(o.configPath); err == nil {
    return &exitError{code: 2, err: fmt.Errorf("%s already exists", o.configPath)}
}
return os.WriteFile(o.configPath, []byte(starterConfig), 0o644)
```

### Current SCD emission (locale/theme only) [VERIFIED: codebase]

```php
// Source: build/src/Magento/LifecyclePlan.php
$commands[] = new Command(Executable::Magento, [
    'setup:static-content:deploy',
    '--language', $content['locale'],
    '--theme', $content['theme'],
    '--no-interaction',
]);
```

### Target SCD emission (add strategy/threads) [CITED: Adobe SCD CLI]

```php
// Target pattern — Magento public CLI flags -s / -j
$args = ['setup:static-content:deploy', '--language', $locale, '--theme', $theme, '--no-interaction'];
if ($strategy !== '') { $args[] = '-s'; $args[] = $strategy; }
if ($threads > 0) { $args[] = '-j'; $args[] = (string) $threads; }
```

### Go staticContent today [VERIFIED: codebase]

```go
// Source: internal/build/plan/plan.go — locales × themes only; strategy/threads ignored
func staticContent(settings map[string]any) ([]buildrunner.StaticContent, error) {
    locales, err := stringList(settings, "locales")
    // …
    themes, err := stringList(settings, "themes")
    // cartesian product → []StaticContent{Locale, Theme}
}
```

### Schema opacity for staticContent [VERIFIED: schema]

```json
"staticContent": {
  "additionalProperties": true,
  "description": "Static content deployment settings",
  "type": "object"
}
```

Tighten in this phase (or document allowed keys): `locales`, `themes`, `strategy`, `threads`.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| ece-tools owns SCD + patches | MageLift PHP `build/` + Go prepare protocol | MageLift foundation | Must close half-done SCD + patches for ICP trust |
| Post-beta “importers later” (`docs/migrating-from-paas.md`, `docs/post-beta-roadmap.md`) | Phase 4 delivers importers | This milestone | Update those docs from “post-beta” to shipped |
| Locale/theme SCD only | Locale/theme + strategy/threads | Phase 4 ECE-03 | Matches store-breaker ACC vars |

**Deprecated/outdated:**
- Docs claiming importers are post-beta only — supersede in Phase 4 docs wave.
- Treating `build.staticContent` as fully opaque forever — still `additionalProperties: true`, but plan should pin known keys.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Host `patch` binary (or equivalent) is acceptable for applying `.patch` files in PHP prepare | ECE-02 / Standard Stack | Need pure-PHP patch library or composer plugin instead |
| A2 | Upsun/Platform.sh file layout mirrors ACC with `.platform*` names sufficiently for shared mapper | IMPORT-02 | Need more Upsun-specific adapters than planned |
| A3 | Minimal new `application.cron` schema is preferred over marking all crons intentional-gap | Pitfall 4 | Schema churn vs incomplete IMPORT-03 |
| A4 | D-04 side-file flag may be named `--config-out` despite CONTEXT saying `--output` | Open Questions | Operator docs diverge from CONTEXT wording until confirmed |

## Open Questions (RESOLVED)

1. **D-04 `--output PATH` vs persistent `--output` format** — **RESOLVED**
   - Decision: Use init-local `--config-out PATH` for the side-file write (04-02 checkpoint). Root persistent `--output/-o` stays output format. D-04 intent (optional path for generated YAML) preserved without colliding with format `-o`.

2. **Cron destination schema** — **RESOLVED**
   - Decision: Add minimal portable `application.cron` list in schema/model for Magento cron entries (`bin/magento cron:run` and mapped schedules). Free-form shell crons that are not Magento cron → unmapped sidecar (04-03).

3. **QUALITY_PATCHES depth** — **RESOLVED**
   - Decision: ECE-02 closes clean-room `m2-hotfixes` apply + php-test (04-05). QUALITY_PATCHES / cloud-required package patch DBs remain intentional-gap in `docs/ece-parity.md` (04-06) — do not vendor Adobe patch databases.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Importer + CLI tests | ✓ | 1.26.5 | — |
| PHP + Composer | `make php-test` / patch+SCD | ✓ | PHP 8.5.8 | — |
| Sibling `../ece-tools` | Behavioral reference only | ✓ (workspace) | — | Public Adobe docs only; never vendor |
| Sibling `../magento-cloud-patches` | Behavioral reference only | ✓ (workspace) | — | Public Adobe docs; never vendor |
| `patch` CLI | ECE-02 apply | ? [ASSUMED] | — | PHP-native apply or skip with documented tool requirement |
| Docker / cloud creds | Phase 4 | N/A | — | Not required (offline) |

**Missing dependencies with no fallback:** none for offline CI path if patch apply is tested with a stub/`patch` present in CI images.

**Missing dependencies with fallback:** soak env vars unset → skip (D-02).

**Step 2.6 note:** External tools beyond Go/PHP are optional soak paths only.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` + race (`make test`); PHPUnit via `make php-test` |
| Config file | `build/phpunit.xml`; Go packages self-contained |
| Quick run command | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ ./internal/cli/ -count=1` |
| Full suite command | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ ./internal/cli/ ./internal/build/... -count=1` then `make php-test` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| IMPORT-01 | ACC fixture → valid YAML | integration | `go test ./internal/paasimport/ ./internal/cli/ -run Acc -count=1` | ❌ Wave 0 |
| IMPORT-02 | Upsun fixture → valid YAML | integration | `go test … -run Upsun` | ❌ Wave 0 |
| IMPORT-03 | Unmapped keys reported key-by-key | unit | `go test ./internal/paasimport/ -run Unmapped` | ❌ Wave 0 |
| IMPORT-04 | `config.Load` + validate | integration | assert Load + validate in import test | ❌ Wave 0 |
| IMPORT-05 | Reject foreign schema as `--config` | unit | `go test ./internal/cli/ -run ForeignSchema` | ❌ Wave 0 |
| IMPORT-06 | No vendored reference source | smoke/grep | CI/script assert no ece-tools tree under repo | ❌ Wave 0 |
| ECE-01 | ece-parity.md no blank rows | docs check | grep/assert table completeness | ❌ Wave 0 |
| ECE-02 | m2-hotfixes apply | phpunit | `make php-test` (focused case) | ❌ Wave 0 |
| ECE-03 | strategy/threads in SCD argv | phpunit + go | plan + LifecyclePlan tests | ❌ Wave 0 (locale/theme ✅ exists) |
| ECE-04 | allowlist documented | docs + unit | mapper allowlist table test | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** focused `go test` on touched packages (`GOMAXPROCS=1 GOFLAGS=-p=1`)
- **Per wave merge:** importer+cli+build Go tests + `make php-test` when PHP touched
- **Phase gate:** above green + docs present before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `testdata/fixtures/acc/` and `testdata/fixtures/upsun/` — supported + unmapped variants
- [ ] `internal/paasimport/` package + table tests
- [ ] CLI tests for `--from-acc`/`--from-upsun`, refuse/`--yes`, unmapped exit
- [ ] Foreign-schema rejection test
- [ ] PHPUnit cases for patch apply + SCD `-s`/`-j`
- [ ] Clean-room provenance check (no vendored sibling paths)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | — |
| V3 Session Management | no | — |
| V4 Access Control | no | — |
| V5 Input Validation | yes | Bound YAML parse; refuse unknown MageLift fields via schema; unmapped PaaS keys reported not silently coerced |
| V6 Cryptography | yes (mapping only) | Crypt key → secret **reference** fields only; never emit plaintext crypt keys into `magelift.yaml` |

### Known Threat Patterns for this phase

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Path traversal via fixture/soak env paths | Tampering | Clean + confine to config-root; reject `..` escapes |
| Secret plaintext in generated YAML | Information Disclosure | Map crypt → `aws-secrets-manager://…` / provider secret refs placeholders |
| Command injection via mapped hooks | Tampering | MageLift hooks are typed command vectors only; free-form PaaS hook shells → unmapped / intentional gap |
| Overwrite without confirmation | Tampering | Refuse exists unless `--yes` |

## Recommended Plan Wave Breakdown

| Plan | Focus | Primary reqs | Notes |
|------|-------|--------------|-------|
| **04-01** | Fixtures + foreign-schema reject + package skeleton | IMPORT-05, IMPORT-06 (partial) | Wave 0 tests red→green skeleton |
| **04-02** | `init` CLI: `--from-acc/--from-upsun`, mutual exclusion, refuse/`--yes`, `--config-out` | IMPORT-01/02 surface | Wire to stub mapper |
| **04-03** | Shared mapper + ACC + Upsun + unmapped sidecar + validate gate | IMPORT-01..04, ECE-04 allowlist code | Largest plan; keep fixtures config-only |
| **04-04** | SCD `strategy`/`threads` Go→PHP + schema keys | ECE-03 | Vertical slice with phpunit |
| **04-05** | Clean-room `m2-hotfixes` patch apply + php-test | ECE-02 | After composer install in build phase |
| **04-06** | `docs/ece-parity.md` + D-07 allowlist docs + update migrating/post-beta/provenance | ECE-01, ECE-04, IMPORT-06 | Honesty close-out |

**Recommended plan count: 6.**

## Sources

### Primary (HIGH confidence)
- `internal/cli/root.go` — init refuse-if-exists, persistent `--yes`/`--output`
- `internal/cli/releases.go` — `promote --from`
- `internal/build/plan/plan.go`, `internal/build/runner/protocol.go` — SCD locale/theme only
- `build/src/Magento/LifecyclePlan.php` — SCD command emission
- `schema/magelift.schema.json` — opaque `staticContent`
- `internal/platform/env.go` — `MAGENTO_DC_*` seams
- `docs/provenance.md`, clean-room lesson — IMPORT-06 policy
- `.planning/phases/04-…/04-CONTEXT.md` — D-01..D-07

### Secondary (MEDIUM confidence)
- Adobe Experience League: ACC config files overview, services/routes, deploy variables (`SCD_STRATEGY`/`SCD_THREADS`) [CITED: experienceleague.adobe.com]
- Adobe Experience League: static view deploy CLI (`-s`, `-j`) [CITED: experienceleague.adobe.com]
- Adobe Experience League: apply patches / m2-hotfixes order [CITED: experienceleague.adobe.com]
- Sibling `ece-tools` StageConfigInterface public constants — behavioral observation only (not copied)

### Tertiary (LOW confidence)
- Exact Upsun file-name variants beyond `.platform*` [ASSUMED] — confirm against Upsun public docs during 04-03

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — existing Go/PHP toolchain, no new deps
- Architecture: HIGH — clear seams; CLI landmines verified in-tree
- Pitfalls: HIGH for flag collisions; MEDIUM for cron schema / QUALITY_PATCHES depth

**Research date:** 2026-07-29  
**Valid until:** 2026-08-28 (30 days; ACC public docs stable)
