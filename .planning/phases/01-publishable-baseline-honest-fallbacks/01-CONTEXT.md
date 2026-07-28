# Phase 1: Publishable Baseline & Honest Fallbacks - Context

**Gathered:** 2026-07-28
**Status:** Ready for execution
**Mode:** Auto-generated (autonomous milestone loop; discuss skipped — 9 plans + RESEARCH already present)

<domain>
## Phase Boundary

Clear the debt that worsens with every later phase, and make every target state its own tier. The repository survives a stranger's first read — CI actually runs and passes the verification suite for the first time, the highest-risk file is no longer a single construction path, every recently fixed bug has a regression guard, and no target can silently pretend to support something it does not.

Cloud spend: None — fully offline (except evidence that lands via GitHub Actions on push).
</domain>

<decisions>
## Implementation Decisions

### Agent Discretion (autonomous / yolo)
- Skip re-discuss and re-plan: `01-RESEARCH.md` and `01-01`…`01-09` PLAN.md files already exist and are the locked execution surface.
- Hardware: serial Go builds only (`GOMAXPROCS=1 GOFLAGS=-p=1`); never parallelize executors that compile.
- Human gates: pause for push-to-`main` CI proof (plan 01-01 Task 2), paid cloud phases (3/7/8), and any grey-area blocker the executor surfaces.

### Locked from research
- Lint partitioning into six matrix jobs (aws, gcp, ovh, scaleway, core, aggregate) plus cache-prime and go-verify.
- Criterion corrections in ROADMAP (go job never green, QUALITY-02 guard exists, etc.) stand.
</decisions>

<code_context>
## Existing Code Insights

See `01-RESEARCH.md`. Plans 01-01…01-09 encode the concrete file targets and verification commands.
</code_context>

<specifics>
## Specific Ideas

Execute plans in wave order. Do not rewrite plans unless an executor returns a planning defect (e.g. A1 issue-set mismatch).
</specifics>

<deferred>
## Deferred Ideas

Paid acceptance, OpenSearch SigV4 live plane, Aurora CreateDBCluster, amazon-mq × preview — remain deferred per STATE.md / ROADMAP.
</deferred>
