# Phase 4: Brownfield Onramp — PaaS Import & ece-tools Parity - Context

**Gathered:** 2026-07-29
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver offline ACC/Upsun → reviewable `magelift.yaml` importers (`magelift init --from-acc` / `--from-upsun`) with fail-loud unmapped-key reporting, plus `docs/ece-parity.md` and closing high-traffic half-done build gaps (patches, SCD strategy/threads, common env allowlist). Never accept foreign PaaS schemas as deploy input. Phase 5 owns dump/media cutover; Phase 7 owns live GCP.

</domain>

<decisions>
## Implementation Decisions

### Fixture sources
- **D-01:** Use synthetic config-only fixtures under `testdata/fixtures/{acc,upsun}/` as the CI proof path for IMPORT-01/02 (mapping + fail-loud unmapped keys). — **Reversibility:** reversible — fixtures can expand without API change
- **D-02:** Optional maintainer soak via `MAGELIFT_IMPORT_FIXTURE_ACC` / `MAGELIFT_IMPORT_FIXTURE_UPSUN` (config-root paths only); never vendor sibling/`ece-tools` trees into this repo; soak tests must skip cleanly when unset. — **Reversibility:** reversible

### CLI surface + overwrite
- **D-03:** Expose `magelift init --from-acc` and `magelift init --from-upsun` as mutually exclusive flags on existing `init` (literal IMPORT-01/02 wording). Do not add `magelift import` or unify under `--from <enum>` this phase. — **Reversibility:** one-way — published REQUIREMENTS/docs/CLI help become the contract operators learn
- **D-04:** Refuse if `magelift.yaml` (or `--config` path) already exists (exit 2), matching bare `init`. Overwrite only with `--yes` (MageLift destructive-confirmation pattern — do not invent `--force`). Optional `--output PATH` may write a side file for review without clobbering the default path. — **Reversibility:** costly — changing refuse/`--yes` semantics breaks scripts and operator muscle memory

### Unmappable-key report UX
- **D-05:** On any unmapped key: write mapped `magelift.yaml` + durable sidecar unmapped-keys report, exit non-zero (refuse success). Full map of a supported shape → write YAML only, exit 0. Never exit 0 with only stderr warnings; never default-lenient `--strict`. Soft path later, if needed, must be explicit opt-out (`--allow-unmapped`), not the default. — **Reversibility:** one-way — exit-code + sidecar path become CI/automation contract

### ece-tools parity depth
- **D-06:** Ship `docs/ece-parity.md` with no blank rows (every hook/env closed or intentional-gap) AND close high-traffic half-done gaps already implied by shipping: clean-room patch apply, SCD strategy/threads wired through Go→PHP (not locale/theme only), plus common env→`magelift.yaml` mapping covered by D-07. Do **not** attempt full ece-tools behavioral clone this phase. — **Reversibility:** costly — parity matrix rows and php-test gates become honesty commitments

### Env-var mapping v1 surface
- **D-07:** Map structural PaaS YAML (app/services/routes/cron) plus a frozen documented v1 allowlist of store-breaker vars: crypt → encryption secret ref; routes/UPDATE_URLS intent → domain/base-URL; DB/Redis/OpenSearch/AMQP relationships → existing capability / `MAGENTO_DC_*` seams; SCD_* → existing `build.staticContent`. Everything else = intentional ECE-04 gap rows — not silent drops. — **Reversibility:** costly — allowlist is a published contract; shrinking it breaks “supported shapes without hand edits”

### the agent's Discretion
- Exact sidecar report filename/path convention (recommend adjacent `magelift.unmapped.md` or `.magelift/import-unmapped.md` — planner picks with tests)
- Fixture file layout details within `testdata/fixtures/{acc,upsun}/`
- Which long-tail stage vars get intentional-gap wording in `docs/ece-parity.md` beyond the locked allowlist

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase / requirements
- `.planning/ROADMAP.md` — Phase 4 goal + success criteria
- `.planning/REQUIREMENTS.md` — IMPORT-01..06, ECE-01..04
- `.planning/PROJECT.md` — clean-room + PaaS import checklist items

### Honesty / provenance
- `docs/provenance.md` — clean-room vs ece-tools / ACC references
- `docs/knowledge/lessons/MageLift reference and clean-room policy.md` — behavioral evidence only; no vendored source
- `docs/migrating-from-paas.md` — advertised `init --from-acc` / `--from-upsun` wording

### Build / runtime contracts already shipping
- `docs/configuration.md` — `magelift.yaml` / `build.staticContent` shapes
- `build/src/Magento/LifecyclePlan.php` — SCD command emission
- `internal/cli/root.go` — `init` refuse-if-exists (exit 2); `--yes` destructive pattern

### Deferred (do not implement in Phase 4)
- `.planning/REQUIREMENTS.md` MIGRATE-01..05 — Phase 5 dump/media cutover
- ADR / docs for brownfield attach — Phase 8

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/cli/root.go` `init` command: creates starter `magelift.yaml`, refuses if file exists (exit 2)
- `build/` PHP lifecycle: already emits locale×theme `setup:static-content:deploy`; strategy/threads not fully wired
- Schema `schema/magelift.schema.json` + `magelift config validate` for IMPORT-04 gate
- Acceptance fixture patterns under tests/ for hermetic offline proof

### Established Patterns
- Destructive confirmation uses `--yes`, not `--force`
- Honesty-first: refuse silent success; loud unmapped reporting (TRUST-02 lineage)
- Clean-room: reference sibling trees (`../ece-tools`, `../cli`) inform behavior but are never vendored

### Integration Points
- New importer flags hang off existing `init` cobra command
- Importer writes default config path (or `--output`) then operators run `config validate` / diff before deploy
- ECE parity doc + php-test gates for patches and SCD settings

</code_context>

<specifics>
## Specific Ideas

- [--auto] Persona synthesis (Staff Platform / DX CLI / Agency Lead / Release Manager / Magento Cloud Operator) locked the decisions above in a single pass
- Fixtures are config-only mini-repos (`.magento*` / `.platform*` / `.upsun`), not full Magento trees
- Sidecar unmapped report must be durable on disk (not stderr-only)

</specifics>

<deferred>
## Deferred Ideas

- Live GCP create / PSA soak — Phase 7
- Dump/media cutover and `seedDump` runner — Phase 5
- Brownfield attach of existing VPC/RDS — Phase 8
- Exhaustive ACC/Upsun env parity / full ece-tools behavioral clone — not in v1.0.0-rc scope
- Hosted Actions QUALITY-06 green — Phase 1 HUMAN_GATE

</deferred>

---

*Phase: 4-Brownfield Onramp — PaaS Import & ece-tools Parity*
*Context gathered: 2026-07-29*
