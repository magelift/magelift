# Phase 1: Publishable Baseline & Honest Fallbacks - Research

**Researched:** 2026-07-27
**Domain:** Go CI/lint scaling on multi-provider Pulumi graphs; behaviour-preserving Go refactoring; Pulumi mock-based regression testing; Cobra CLI tier gating
**Confidence:** HIGH on codebase facts (read and cross-checked against live CI logs); HIGH on AWS IAM limits; MEDIUM on golangci-lint partitioning mechanics; **LOW on the AOSS collection-group OCU rule (AWS does not document it)**

---

## Summary

Three findings change this phase's shape before any planning starts.

**1. The `go` CI job has never been green.** All 38 CI runs of `.github/workflows/ci.yml` exist; in every one the `go` job either failed or was skipped by the path filter. There is no green `go` job in the repository's history. The most recent failure (run `30221546264`, 2026-07-26) shows `golangci-lint run ./...` starting at `21:42:12` and the log ending at `22:12:42` with `##[error]The runner has received a shutdown signal` — zero linter output, 30 minutes, runner killed. `actions/setup-go` logged `Cache is not found` in the same run, so both the Go build cache and module cache were cold. Because the runner is *killed*, `setup-go`'s cache-save post step never runs, so the next run is cold again: **the failure is self-reinforcing.** Phase 1 criterion 1 is therefore not "remove a workaround from a working system" — it is "make Go CI green for the first time." Plan accordingly: the workaround removal and the partitioning are one task, not two, and the phase needs an explicit "go job green on a PR" gate.

**2. Two of the eight QUALITY items are largely already done, and two require code changes the requirement does not mention.** QUALITY-02's guard already exists (`internal/cloud/aws/bootstrap/identity_test.go:242-246`, added by the same commit `8c3a4c6` that CONCERNS.md says left "no visible automated guard") — the real work is extending it to the two unguarded boundaries and giving it a dedicated test name. QUALITY-01's deleted `cost_test.go` tested `newCostReport`/`newLiveCostReport`, functions that **no longer exist**; the deletion was a legitimate consequence of moving cost behind `platform.CostEstimator`, and the genuine gap is that `internal/cli/cost.go` and `platform.ModuleCostEstimator` have *zero* test coverage today. Conversely QUALITY-05 cannot be satisfied by tests alone: **AWS and GCP subnet carving have no index cap to test** — the caps must be added first — and Scaleway carves no subnets at all, so criterion 3's "for every provider" is not achievable as written.

**3. `runtime.go` can be split with a hard guarantee that `runtime_test.go` passes unchanged**, because every identifier the test uses is package-scoped. An intra-package file split is invisible to the test compiler. Criterion 2's 400-line cap is comfortably achievable with a 7-file split, and the SigV4 path is preserved by construction. One caveat: `runtime_test.go` is currently **not gofmt-clean**, so "passes unchanged" must be measured against a gofmt'd baseline established in Wave 0.

**Primary recommendation:** Wave 0 restores a green baseline (gofmt the three drifted files, then partition lint and prove the `go` job green on a throwaway PR). Only then run QUALITY-01..05, 07, 08 and TRUST-01/02 as mostly-independent parallel tracks. Do not let any test-writing task land before the `go` job can actually execute `go test -race ./...` in CI — that command has never run there.

---

## Project Constraints (from AGENTS.md, PROJECT.md, ADRs)

These have the same authority as locked decisions. No plan may contradict them.

| Constraint | Source | Consequence for this phase |
|---|---|---|
| **Never build in parallel locally.** Parallel `go build`/goreleaser has caused kernel panics on this 16 GB Mac. | `AGENTS.md`, `docs/knowledge/reference/Build and release on 16GB…md` | Any local verification task must use `GOMAXPROCS=1 GOFLAGS=-p=1`, run in a plain Terminal, and must never be a multi-platform matrix. Never recommend `go build ./...` unbounded. |
| Cloud adapter internals must not leak into `internal/cli` or `internal/deploy`; only `sdk/v1` types and `internal/platform` interfaces cross. | ADR 0002/0004, `PROJECT.md` Constraints | The TRUST-01 tier warning must read the tier from `platform.PlannedStack.CertificationTier()` — never from a cloud package. Any shared CIDR test helper must not live in a place that couples adapters. |
| `internal/cli` must stay free of Pulumi cloud SDK imports (`gendocs` OOM'd at ~8.5 GB peak RSS otherwise). | `docs/knowledge/lessons/MageLift gendocs must not link Pulumi cloud SDKs.md` | CLI tests must keep using `stub_module_test.go` stubs, not real modules. A tier-warning test must add a **stub** experimental module, not import `ovh/stack`. |
| Fully offline phase. Verification only via `make verify`, `go test -race ./...`, `make floci-test`, `make php-test`. | ROADMAP Cloud Spend Map | No task may require an AWS/GCP credential. The AOSS OCU rule and the IAM boundary rule can only be *locked in*, not *validated against AWS*. |
| Solo maintainer, no human reviewer. | `PROJECT.md` Constraints | Prefer checks that fail loudly in CI over conventions or doc notes. |
| `gofmt` only; no goimports/gofumpt. Narrow lint set; expand deliberately. | `.planning/codebase/CONVENTIONS.md`, `docs/lint-policy.md` | Do not add linters as part of QUALITY-06. Do not reformat beyond `gofmt`. |
| Stdlib `testing` only — no testify, no gomock. Pulumi `WithMocks` for infra. | `.planning/codebase/TESTING.md` | Every new test in this phase uses `if got != want { t.Fatalf }` and `pulumi.WithMocks`. Adding an assertion library would be a new dependency and is out of scope. |

---

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| QUALITY-01 | Restore `internal/cli/cost.go` coverage — flag parsing, error paths, `ErrNotSupported` on non-AWS | §Q1: the deleted test targeted removed functions; the real gap is `costCommand` + `costEstimator()` + `platform.ModuleCostEstimator`. Test harness pattern in `root_test.go:26-37` + `stub_module_test.go`. |
| QUALITY-02 | Test asserts rendered IAM permissions boundary stays under AWS's 6 KiB limit | §Q2: guard already exists at `identity_test.go:242-246`; 6,144-char managed-policy limit and 10,240-char inline-role limit confirmed against AWS docs, whitespace **not** counted. Two boundaries still unguarded. |
| QUALITY-03 | Table-driven tests enumerate the AOSS collection-group OCU values AWS accepts | §Q3: **AWS does not document the rule MageLift enforces.** The test can only lock the empirically observed rule from commit `b8b957e`; provenance must be recorded, not claimed as AWS-verified. |
| QUALITY-04 | Combination tests cover `queueMode` × `searchMode` × `webRuntime` together | §Q4: exact per-cell coverage inventory below. The cells live in `internal/cloud/aws/stack`, not `runtime` — the combination test belongs there. 6 concrete untested interactions identified. |
| QUALITY-05 | Every provider's subnet/CIDR carving has explicit boundary tests (min, max, max+1) | §Q5: OVH has a cap and tests; **AWS and GCP have no cap at all**; Scaleway carves nothing. Requires code changes plus tests, and criterion 3 needs rewording. |
| QUALITY-06 | golangci-lint completes without OOM or timeout by partitioning per provider | §Q6: root cause established from live CI logs + the 8.5 GB knowledge note. Partition scheme designed; `cmd/magelift` is the irreducible aggregate. |
| QUALITY-07 | Split `internal/cloud/aws/runtime/runtime.go` by concern | §Q7: 7-file split with exact line ranges; behaviour-preserving by construction; `runtime_test.go` compiles unchanged. |
| QUALITY-08 | Explicit regression tests for the two fixed OVH bugs | §Q8: the node-pool fix is in `internal/cloud/ovh/runtime/runtime.go` (**not** `stack/ops.go` as CONCERNS.md states) and that package has no test file. `MockResourceArgs.RegisterRPC.Dependencies` makes DependsOn assertable. |
| TRUST-01 | Experimental-tier warning from the CLI before any mutating command | §T1: `(*options).planStack` at `internal/cli/lifecycle.go:269` is the single choke point — 18 call sites, all pre-Pulumi. Tier is already on `PlannedStack`. |
| TRUST-02 | Unimplemented day-2 commands fail loudly with capability + tier | §T2: 15+15 sites confirmed exactly. `notSupported()` at `ports.go:75` already names capability + target but **not tier**. Three genuine silent-success paths found that the criterion's 15 sites do not cover. |

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|---|---|---|---|
| Lint partitioning, cache priming | CI / GitHub Actions | Repo config (`.golangci.yml`) | Peak RSS is a runner property; the config only controls concurrency. Partition must be expressed as a job matrix, not a config key. |
| Tier warning before mutate | CLI (`internal/cli`) | `internal/platform` (tier source) | ADR 0002/0004 forbids the CLI reading cloud packages; `PlannedStack.CertificationTier()` is the sanctioned crossing. |
| Tier-named unsupported errors | CLI (`internal/cli/ports.go`) | Cloud adapters (`unsupported{}`) | The adapter returns a sentinel; only the CLI knows the planned stack and can name the tier. Do not push tier strings into adapters. |
| Catalog-cell combination validation | AWS stack adapter (`internal/cloud/aws/stack`) | AWS runtime adapter | `searchMode`/`queueMode` names exist only in `stack`; `runtime` sees projections (`SearchProxyImage`, `Capabilities.QueueMode`). Testing combinations in `runtime` tests a downstream artefact. |
| CIDR carving | Per-provider network package | — | ADR 0008 keeps cloud-shaped concerns per-provider. Each provider's carve semantics differ materially (see §Q5); a shared helper would be a false abstraction. |
| IAM policy size guard | AWS bootstrap adapter | — | Cloud-specific quota, cloud-specific document. |
| Runtime construction split | AWS runtime adapter, intra-package | — | Files, not packages. Crossing a package boundary would break `runtime_test.go`'s access to unexported symbols. |

---

## Standard Stack

### Core — no new dependencies

The correct answer for this phase is **zero new libraries**. Every requirement is satisfiable with what is already in `go.mod` plus existing repo conventions.

| Library | Version | Purpose | Why Standard |
|---|---|---|---|
| `testing` (stdlib) | go1.26.5 | All new tests | `.planning/codebase/TESTING.md`: stdlib only, no testify anywhere in the repo `[VERIFIED: grep -rl testify returns nothing]` |
| `github.com/pulumi/pulumi/sdk/v3` | v3.253.0 | `pulumi.WithMocks` harness for all infra assertions | Already the repo's only infra mock mechanism `[VERIFIED: go.mod]` |
| `github.com/pulumi/pulumi/sdk/v3/go/common/resource` | v3.253.0 | `resource.PropertyMap` assertions | Existing pattern in every `*_test.go` under `internal/cloud/` |
| `golangci-lint` | v2.12.2 | Lint | Pinned in both `Makefile:18` and `ci.yml:89`; keep pinned `[VERIFIED: files read]` |
| `golangci/golangci-lint-action` | v9.3.0 (`ba0d7d2e…`) | CI lint runner | Already pinned by SHA `[VERIFIED: ci.yml:86]` |

### Supporting — Go tooling for the refactor (no dependency, developer tooling)

| Tool | Purpose | When to Use |
|---|---|---|
| `gopls` code actions (Move / Extract) | Behaviour-preserving relocation of declarations between files in the same package | QUALITY-07. Available via the `golang-gopls` skill / Serena. Prefer over manual cut-paste. |
| `gofmt -w` | Sole formatter | Wave 0 drift fix and after every file move |
| `go test -run '^TestRuntime' ./internal/cloud/aws/runtime/ -count=1` | Fast per-task verification of the split | Every QUALITY-07 subtask |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|---|---|---|
| Lint partitioning (QUALITY-06) | GitHub **larger runners** (more RAM) | **Not available.** Larger runners require GitHub Team or Enterprise Cloud `[CITED: docs.github.com/en/actions/reference/runners/github-hosted-runners]`. This is a solo-maintainer Free-plan personal repo. Rejected. |
| Lint partitioning | Wait for the repo to go public (2 vCPU/7 GB → 4 vCPU/16 GB) | Real effect (§Q6) but not a fix: it does not scale with providers, it cannot be tested before the tag, and QUALITY-06 explicitly asks for partitioning. Use as a *risk buffer*, never as the mechanism. |
| Lint partitioning | Raise the timeout again | Already tried three times (5m → 15m → 20m → 30m) and still fails. Dead end — see §Q6 dead-ends table. |
| `pulumi.WithMocks` + `RegisterRPC` for DependsOn | Registration-order assertions | Pulumi Go registers resources concurrently; order in the mock slice is not a dependency guarantee. Use `RegisterRPC.Dependencies` (URNs) instead. |
| Shared CIDR test helper across providers | Per-provider in-place tests | Shared helper violates ADR 0008's "cloud-shaped ports stay per-provider" and would encode four incompatible carve semantics into one shape. Rejected — see §Q5. |

**Installation:** none.

---

## Package Legitimacy Audit

**Not applicable — this phase installs no external packages.** Every requirement is met with the existing `go.mod` graph, stdlib `testing`, and already-pinned CI actions. No `npm`/`pip`/`go get` step should appear in any plan for this phase. If a plan proposes a new module, that is a signal the plan has drifted from this research.

Existing pins that plans must not change silently:
- `golangci-lint` **v2.12.2** in `Makefile:18` and `ci.yml:89` — keep the two in lockstep (commit `3a0ec0f` exists solely because they drifted).
- All GitHub Actions are SHA-pinned in `ci.yml`. Any new matrix job must reuse the same pinned SHAs.

---

## Q6 — golangci-lint on a multi-provider Pulumi graph (highest value)

### Root cause, from evidence not inference

| Evidence | Source | Confidence |
|---|---|---|
| `golangci-lint run ./...` produced **zero output** in 30m30s, then `##[error]The runner has received a shutdown signal` | CI run `30221546264`, `go` job log lines 401-402 | **HIGH** `[VERIFIED: gh run view --log]` |
| `actions/setup-go` logged `Cache is not found` — Go build cache and module cache cold | same log, line 178 | **HIGH** `[VERIFIED]` |
| golangci-lint's own cache also cold: `Cache not found for input keys: golangci-lint.cache-Linux-2951-…` | same log, line 388 | **HIGH** `[VERIFIED]` |
| Linking all four provider SDKs into one binary produced **~593 MB binary and ~8.5 GB peak RSS**; GitHub runners "OOM/shutdown on that link (exit 143 / hang 12-30+ min)" | `docs/knowledge/lessons/MageLift gendocs must not link Pulumi cloud SDKs.md` | **HIGH** — same failure signature as today's lint failure |
| Private repo ⇒ `ubuntu-latest` is **2 vCPU / 7 GB RAM**. Public repo ⇒ **4 vCPU / 16 GB** | `gh repo view` → `isPrivate: true`; GitHub docs | **HIGH** `[VERIFIED + CITED: docs.github.com]` |
| CodeQL `autobuild` extracted the **whole** Go graph (incl. `pulumi-gcp/.../memorystore`, `k8s.io/apimachinery`) in ~13 min on the same runner class, failing only on a config error ("Code scanning is not enabled") | CodeQL run `30243793821` | **HIGH** `[VERIFIED]` — proves compilation fits; golangci-lint's own analysis does not |
| `go` job has never succeeded — 38 total CI runs, none with a green `go` job | `gh api …/workflows/ci.yml/runs` + per-run job scan | **HIGH** `[VERIFIED]` |

**Conclusion.** The dominant cost is not compilation; it is golangci-lint holding type information plus 8 analysers' facts for ~84 packages spanning five provider SDK graphs in **one process**, with `run.concurrency: 1` forcing `GOMAXPROCS=1` on a 2-vCPU box so that a cold, single-threaded package load cannot finish inside 30 minutes before the 7 GB VM is reaped. The `concurrency: 1` workaround is *counterproductive for the timeout failure mode* while only partially addressing the memory one.

### What has already been tried and failed — do not repeat

| Attempt | Commit | Outcome |
|---|---|---|
| Align `make lint` to v2.12.2 | `3a0ec0f` | Not a fix (version hygiene) |
| Timeout 5m → 15m | `a8173af` | Still failed |
| `go build` warm-up of all non-main packages + timeout 20m | `91b8ecb` | **OOM'd the runner** |
| Revert warm-up; `concurrency: 4 → 1`; timeout 20m → 30m | `b5deb3d` | **Still failing today** (run `30221546264`) |

The reverted warm-up is instructive: `go build "${pkgs[@]}"` with default `-p` = 2 on a 2-vCPU runner still exhausted 7 GB. Any new warm-up step must be bounded (`GOFLAGS=-p=1`) or split.

### Which linters are whole-program / type-check-heavy

| Linter | Needs type info | Reports beyond the named packages? | Partition-safe? |
|---|---|---|---|
| `govet` | yes | no | yes |
| `staticcheck` | yes | no | yes |
| `errcheck` | yes | no | yes |
| `unused` | yes | **see below** | yes, with `tests: true` |
| `gocritic` (diagnostic tag) | yes | no | yes |
| `revive` (narrow rules) | partly | no | yes |
| `ineffassign` | no | no | yes |
| `misspell` | no | no | yes |
| `gofmt` (formatter) | no | no | yes |

**`unused` and partitioning — the decisive question.** Go visibility makes an unexported identifier reachable only from its own package (including its `_test.go` files in the same package). `.golangci.yml` sets `run.tests: true`, so test-only helpers count as used. Under those conditions, analysing a *subset* of packages cannot produce cross-partition false positives for unexported identifiers, because no other partition could have used them. `[ASSUMED]` — I could not find an authoritative golangci-lint statement that its `unused` treats exported identifiers as used (staticcheck's standalone whole-program mode is a separate, historically problematic feature — `golangci/golangci-lint#2771` and `dominikh/go-tools#671` document caching-related false positives when analysing subsets in whole-program mode). **Recommended de-risking step, cheap and decisive:** before committing the partition scheme, run the partitioned matrix once against a known-clean tree and diff the issue set against a single `./...` run performed locally with `GOMAXPROCS=1`. If they match, the partition is sound. Do not skip this — it is the one assumption in this section that could silently weaken the lint gate.

### Supported partitioning mechanisms in golangci-lint v2

| Mechanism | Supported | Notes |
|---|---|---|
| Explicit package patterns (`golangci-lint run ./internal/cloud/aws/...`) | **yes** | The `args` input of `golangci-lint-action` passes them straight through. This is the mechanism to use. |
| `run.concurrency` (= `GOMAXPROCS`) | yes | Default `0` = "automatically set to match Linux container CPU quota and fall back to the number of logical CPUs" `[CITED: golangci-lint.run/docs/configuration/file/]` |
| `run.timeout` | yes | Default in v2 is **`0` (disabled)** `[CITED: same]`. **Removing `timeout: 30m` therefore removes a failure mode rather than adding one.** Add a job-level `timeout-minutes:` so a hung job cannot burn hours. |
| `run.build-tags` | yes | Not useful here — no provider build tags exist in this repo. |
| Separate config files per partition | yes (`--config`) | **Avoid.** Multiple `.golangci.yml` variants drift and would break `golangci-lint config verify` hygiene. One config, different package args. |
| `GOGC` / `GOMEMLIMIT` env | not a golangci-lint option, but honoured by the Go runtime | No official golangci-lint recommendation exists; the FAQ and performance pages say nothing about memory `[VERIFIED: fetched both, /docs/product/performance/ returns 404]`. Community practice (`golangci-lint#5031`, `#3582`) is `GOGC=50`/`GOGC=20` plus explicit concurrency. Treat as a tuning knob of last resort, not the fix. `[ASSUMED]` |
| golangci-lint cache | `~/.cache/golangci-lint`, key includes `{working_directory}`; `skip-save-cache: true` avoids collisions in a matrix `[CITED: golangci-lint-action v9.3.0 README]` | With a matrix, set `skip-save-cache: true` on all but one partition, or accept last-writer-wins. |

### Recommended partition scheme

Derived from the actual import graph. `cmd/magelift/main.go` is the **only** file that imports all four providers (`awsops`, `awseksops`, `gcpops`, `ovhstack`, `scwstack`) `[VERIFIED: read cmd/magelift/main.go]`. `internal/cli` is deliberately provider-free.

| Job | Args | Heaviest SDKs loaded |
|---|---|---|
| `lint (aws)` | `./internal/cloud/aws/...` | pulumi-aws v7 + pulumi-kubernetes v4 (via `aws/eks`, `aws/eksops`) |
| `lint (gcp)` | `./internal/cloud/gcp/...` | pulumi-gcp v9 + pulumi-kubernetes v4 |
| `lint (ovh)` | `./internal/cloud/ovh/...` | pulumi-ovh v2 + pulumi-kubernetes v4 |
| `lint (scaleway)` | `./internal/cloud/scaleway/...` | pulumiverse-scaleway + pulumi-kubernetes v4 |
| `lint (core)` | `./internal/... ./sdk/... ./tests/...` **minus** `./internal/cloud/...`, plus `./cmd/genconfig/... ./cmd/gendocs/...` | pulumi/sdk core only — light |
| `lint (aggregate)` | `./cmd/magelift/...` | **all five** — irreducible |

**Honest limitation the planner must know:** the `cmd/magelift` partition still type-checks the full multi-provider graph, because that single ~30-line file imports every module. It cannot be partitioned per provider. Three ways to handle it, in order of preference:

1. Give it its own matrix job. It runs in parallel with the others, so wall time is `max(job)` not `sum(job)`, and it has the whole 7 GB to itself with `concurrency` unpinned (2 threads). This is the honest reading of criterion 1: five per-provider jobs plus one aggregate job, each green.
2. Prime the cache first: a `cache` job that runs `go build ./cmd/magelift` with `GOFLAGS=-p=1` and saves `~/.cache/go-build` via an explicit `actions/cache` step with `if: always()`, which the lint matrix then restores. **This is the step that breaks the self-reinforcing cold-cache loop** identified above — worth doing regardless of partitioning.
3. If (1) still fails, lint `./cmd/magelift/...` with a reduced linter set via `--default=none --enable=govet,errcheck` on that one job and record the narrowing in `docs/lint-policy.md`. Least preferred, but honest and loud.

Also set `run.tests: true` (already set) and keep the single `.golangci.yml`. Keep `make lint` running `./...` locally (a 16 GB Mac with a warm cache manages it) but document in `docs/lint-policy.md` that CI partitions and why.

### Verdict on ROADMAP criterion 1

> *"CI lint runs as per-provider partitioned jobs that each complete green, and `.golangci.yml` no longer needs `run.concurrency: 1` or the 30-minute timeout to pass"*

**Achievable, but under-specified in two ways the planner should fix:**

- "per-provider partitioned jobs" omits the two non-provider partitions (`core`, `aggregate`) that the import graph forces. Reword to *"partitioned lint jobs, one per provider plus core and aggregate, each green."*
- It says "no longer needs … the 30-minute timeout **to pass**". In golangci-lint v2 the timeout default is *disabled*, so removing the key removes a constraint. The meaningful assertion is a **job-level `timeout-minutes` that each partition completes well inside** — propose 15. State it that way so the criterion is falsifiable.

Add a criterion the ROADMAP is missing: **the `go` job as a whole must be green on a PR, including `go test -race ./...`, `govulncheck`, and `go-licenses`.** None of those has ever executed successfully in CI (§Open Questions Q-1).

---

## Q2 — AWS IAM policy size, and what QUALITY-02 actually needs

### The documented rule

| Fact | Value | Source |
|---|---|---|
| Customer **managed** policy (what `CreatePolicy` creates, and what a permissions boundary is) | **6,144 characters** | `[CITED: docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html]` + `[CITED: repost.aws/knowledge-center/iam-increase-policy-size]` |
| Aggregate **inline** policy size per **role** | **10,240 characters** | same |
| Aggregate inline per user / per group | 2,048 / 5,120 | same |
| Whitespace | **"IAM doesn't count white space when calculating the size of a policy against these limits."** — stated for both inline and managed | `[CITED: reference_iam-quotas.html]` |
| Wire limit on the `PolicyDocument` parameter | 131,072 (min 1) — **not** the enforced quota | `[CITED: API_CreatePolicy.html, API_PutRolePolicy.html]` |
| `CreatePolicy` doc note | *"The maximum length of the policy document that you can pass in this operation, **including whitespace**, is listed below. To view the maximum character counts of a managed policy **with no whitespaces**, see IAM and AWS STS character quotas."* | `[CITED: API_CreatePolicy.html]` |
| Neither limit is adjustable | listed under "You can't request an increase for the following limits" | `[CITED: reference_iam-quotas.html]` |

So "6 KiB" in the requirement means **6,144 characters of whitespace-free JSON**, and 6,144 characters ≠ 6 KiB of arbitrary bytes — but for these ASCII-only documents the distinction is immaterial.

### What already exists

`internal/cloud/aws/bootstrap/identity_test.go:238-247`, added by commit `8c3a4c6`, already asserts:

```go
boundary, err := ciPermissionsBoundaryPolicy()
if len(boundary) > 6144 { t.Fatalf("CI permissions boundary exceeds the IAM managed-policy limit: %d", len(boundary)) }
if len(plan.CIPermissionsPolicy) > 10240 { t.Fatalf("CI inline permissions exceed the IAM role-policy limit: %d", len(plan.CIPermissionsPolicy)) }
```

`[VERIFIED: read identity_test.go and git show 8c3a4c6]`. **CONCERNS.md's claim that there is "no visible automated guard" is wrong.** Do not re-derive it.

`len()` on the marshalled string is a **sound and conservative** proxy: `policyDocument` → `canonicalJSON` → `json.Marshal`, which emits compact JSON with no indentation `[VERIFIED: internal/cloud/aws/bootstrap/policy.go:150-152, identity.go:161-167]`. The only whitespace that could survive is spaces inside string values; none of these documents contain any (actions, ARNs, service prefixes). So `len(compact) ≥ non-whitespace count`, always. **Do not add a whitespace-stripping step — it would be dead complexity.**

### The real gaps

Three managed policies are created via `ensureManagedPolicy` (→ `CreatePolicy`, 6,144 limit) and three inline role policies via `ensureRoleSpec` (→ `PutRolePolicy`, 10,240 limit) `[VERIFIED: identity.go:269-300, 390]`:

| Document | Submitted as | Current guard |
|---|---|---|
| `ciPermissionsBoundaryPolicy()` | managed (CI boundary) | ✅ 6,144 |
| `plan.PermissionsPolicy` (= `statePermissionsPolicy`) | managed, via `ensureBoundary` at `identity.go:269-271` | ❌ **none** |
| `plan.BuildPermissionsPolicy` | managed, via `ensureBuildBoundary` at `identity.go:289-291` | ❌ **none** |
| `plan.CIPermissionsPolicy` | inline (`magelift-ci`) | ✅ 10,240 |
| `plan.StatePermissionsPolicy` | inline (`magelift-state`) | ❌ none |
| `plan.BuildPermissionsPolicy` | inline (`magelift-build`) | ❌ none |
| `plan.CITrustPolicy` / `TrustPolicy` / `BuildTrustPolicy` | role trust policy | ❌ none — **default quota 2,048 characters**, raisable to 8,192 `[CITED: reference_iam-quotas.html adjustable-quota table]` |

**Recommendation.** One dedicated, table-driven test — `TestIAMPolicyDocumentsStayUnderAWSCharacterQuotas` — in `internal/cloud/aws/bootstrap/identity_test.go`, enumerating every rendered document with its submission mechanism and quota, and asserting a margin. Move the two existing assertions out of `TestIdentityPlanIsRepoScopedAndLeastPrivilege` into it so a size failure names itself. Use a **10 % headroom warning and a hard fail at the quota**: fail hard at 6,144 / 10,240 / 2,048, and additionally fail if a document exceeds 90 % of its quota, with a message telling the maintainer to move detail inline (the exact strategy commit `8c3a4c6` used). Rationale: the failure mode this requirement targets is *silent growth*, and a bare at-the-limit test gives no warning until it is already too late to add a permission.

---

## Q3 — AOSS collection-group OCU: the rule MageLift enforces is undocumented by AWS

MageLift validates `ServerlessCapacity` at `internal/cloud/aws/search/search.go:226-236` using `validServerlessOCU` at `:279-285`:

```go
func validServerlessOCU(value float64) bool {
	switch value { case 1, 2, 4, 8, 16: return true }
	return value >= 32 && math.Mod(value, 16) == 0
}
```

with a comment claiming *"AOSS collection-group capacity no longer allows 0; AWS accepts 1, 2, 4, 8, 16, or multiples of 16."* Commit `b8b957e` says the same. `[VERIFIED: file read + git log]`

### What AWS actually documents

| Field | Documented constraint | Source |
|---|---|---|
| `minIndexingCapacityInOCU` | Type Float, **Valid Range: Minimum value of 0.** Required: No | `[CITED: docs.aws.amazon.com/opensearch-service/latest/ServerlessAPIReference/API_CollectionGroupCapacityLimits.html]` |
| `minSearchCapacityInOCU` | Float, **minimum 0.** Required: No | same |
| `maxIndexingCapacityInOCU` | Float, **minimum 1.** No maximum, no step documented | same |
| `maxSearchCapacityInOCU` | Float, **minimum 1.** No maximum, no step | same |
| `CreateCollectionGroup` errors | `ValidationException` (400) "when the HTTP request contains invalid input", `ServiceQuotaExceededException` (400) | `[CITED: API_CreateCollectionGroup.html]` |
| **Account-level** (a *different* API, `UpdateAccountSettings`) | min 1 OCU (0.5 × 2) indexing and search; default max 10 each; max 1,700 each; *"You can configure the OCU count to be any number from **2** to the maximum allowed capacity, **in multiples of 2**."* | `[CITED: developerguide/serverless-scaling.html]` |
| Collection-group concept page | Documents that limits exist; states **no numeric values or granularity** | `[CITED: developerguide/serverless-collection-groups.html]` |

### Consequences for the plan — read this carefully

1. **AWS's public documentation nowhere states the "1, 2, 4, 8, 16, or multiples of 16" rule.** `[VERIFIED: four AWS pages fetched]` The nearest documented granularity is *multiples of 2*, and that is for a different API surface.
2. The documented model says `minIndexingCapacityInOCU` **allows 0**, directly contradicting the comment and commit message asserting that AWS rejected `min 0`.
3. Therefore **QUALITY-03's test cannot prove what AWS accepts.** It can only lock MageLift's own validator against silent drift. Writing the test as if it encodes AWS behaviour would be exactly the over-claiming TRUST-03/TRUST-04 exist to prevent.

**Recommendation — three deliverables, not one:**

- **(a)** `TestServerlessOCUValidationAcceptsAndRejects` in `internal/cloud/aws/search/search_test.go`: a table over `{value, wantValid}` covering `0, 0.5, 1, 2, 3, 4, 6, 8, 12, 16, 17, 24, 32, 48, 33, -1, NaN, +Inf, 1700, 1712` and the min>max / max<min range cases. Assert against `validServerlessOCU` and `validCapacityRange` directly (both unexported, same package — fine).
- **(b)** A comment above `validServerlessOCU` naming the provenance explicitly: *"Empirically derived from a `CreateCollectionGroup` rejection on 2026-07-19 (commit b8b957e). AWS documents only `min ≥ 0` / `max ≥ 1` for `CollectionGroupCapacityLimits`; the step rule is not published. Re-verify on any paid pass."* Plus a `docs/knowledge/` lesson note recording the same, so the next agent does not "fix" the validator against the docs.
- **(c)** Feed this to Phase 3: the AOSS OCU rule is an **unverifiable-offline cell** in TRUST-04's sense. Record it as such in `docs/capability-matrix.md` when Phase 3 builds the evidence tiering. Flag it now so it is not forgotten.

Also worth fixing while in the file: the code rejects `min = 0` but permits `min = max` and any `max` with no ceiling. The documented account-level maximum is 1,700 OCU. Adding an upper bound of 1,700 is cheap and strictly more honest than unbounded. `[CITED: serverless-scaling.html]`

---

## Q4 — Catalog-cell combination coverage (QUALITY-04)

### Where the cells actually live

`queueMode` and `searchMode` are **`internal/cloud/aws/stack` concepts**, not `runtime` ones `[VERIFIED: read spec.go:99-121, component.go:191-214, config.go:138-158]`:

```
Spec.Catalog.SearchMode ∈ {disabled, serverless, provisioned}   (spec.go:99-101)
Spec.Catalog.QueueMode  ∈ {db, amazon-mq, ecs-rabbitmq, ecs-artemis}  (spec.go:103-106)
Spec.Application.WebRuntime ∈ {nginx-fpm, frankenphp-classic}    (spec.go:251)
```

They are **projected** into `runtime.Args` at `component.go:191-214`:

| Stack cell | Runtime projection | Line |
|---|---|---|
| `SearchMode != disabled` | `Args.SearchProxyImage = sigV4ProxyImage` | `component.go:191-193` |
| `QueueMode` | `Args.Capabilities.QueueMode = component.Queue.QueueMode` | `component.go:198` |
| `QueueMode != db` | `Args.QueueConsumerCount = 2` (else 0) | `component.go:386-398` |
| `WebRuntime` | `Args.WebRuntime` | `component.go:212` |

**`internal/cloud/aws/runtime` cannot see the cell names at all.** A combination test written only in `runtime_test.go` tests projections, not cells, and would not catch a mis-projection in `component.go`. The combination matrix therefore belongs primarily in **`internal/cloud/aws/stack/component_test.go`** (which already drives `Program(spec)` under mocks), with a secondary projection-level matrix in `runtime_test.go`.

### What `runtime_test.go` covers today — verified inventory

CONCERNS.md marked this "unverified without deeper inspection". Here it is, verified by reading all 586 lines.

| Test | webRuntime | mode | search proxy | queue consumers | Capabilities |
|---|---|---|---|---|---|
| `TestRuntimeResourceGraphAndSecurityContract:36` | nginx-fpm | integrated | off | 0 | nil |
| `TestRuntimeAddsSigV4ProxyForMagentoOpenSearch:169` | nginx-fpm | integrated | **on** | 0 | rabbitmq |
| `TestRuntimeOmitsSigV4ProxyWhenSearchDisabled:223` | nginx-fpm | integrated | off | 0 | rabbitmq |
| `TestRuntimeSigV4ProxySignsAOSSServiceName:247` | nginx-fpm | integrated | on (aoss host) | 0 | rabbitmq |
| `TestRuntimeCreatesQueueConsumerServiceWhenRequested:299` | nginx-fpm | integrated | off | **2** | nil |
| `TestRuntimeFrankenPHPClassicUsesSingleHTTPContainer:313` | **frankenphp** | **headless** | off | 0 | nil |
| `TestRuntimeInjectsNonSecretCapabilityReferences:331` | **both** (subtests) | integrated | off | 0 | rabbitmq |
| `TestRuntimeAttachesExistingSecurityGroupAndTargetGroup:401` | nginx-fpm | integrated | off | 0 | nil |

**Genuinely uncovered interactions — each one is a live code path:**

| # | Combination | Uncovered code | Risk |
|---|---|---|---|
| U1 | `frankenphp-classic` × search proxy on | `appendSearchProxy` branch `args.WebRuntime != "nginx-fpm"` sets `DependsOn` on the single `web` container (`runtime.go:740-742`) | The only sidecar-dependency branch never exercised. Directly adjacent to the certified-intent SigV4 path. |
| U2 | `frankenphp-classic` × `integrated` (Varnish) | `containerDefinitionsFor` appends Varnish when `name == "web"` (`runtime.go:712-717`); FrankenPHP reaches it via `runtime.go:615` | Varnish + FrankenPHP wiring never tested. `TestRuntimeFrankenPHPClassic…` uses `headless` and `VarnishImage: ""`. |
| U3 | queue consumers > 0 × search proxy on | queue task def also goes through `containerDefinitionsForInput` → `appendSearchProxy` (`runtime.go:321`, `685-705`) | A queue consumer silently gets (or loses) the SigV4 sidecar; no test asserts either way. |
| U4 | deploy / cron task × search proxy on | same path, `name ∈ {deploy, cron}` (`runtime.go:257, 294`) | `setup:upgrade` and reindex both need search. If the proxy is absent there, migrations fail at runtime only. |
| U5 | `queueMode: db` (`QueueMode` output `"db"`) | `capabilityEnvironment` sets `queueConnection = "db"` when mode ≠ `"rabbitmq"` (`runtime.go:905-908`) | Only `"rabbitmq"` is ever asserted. The default preview cell's env is untested. |
| U6 | `amazon-mq` / `ecs-artemis` queue endpoint shapes | `amqpSettings` covers only `amqps://…:5671` and one invalid string (`runtime_test.go:475-482`) | `amqp://` default port 5672, no-port, and Artemis endpoint shapes untested. |

### Valid combination space (excluding designed-invalid cells)

`searchMode` × `queueMode` × `webRuntime` = 3 × 4 × 2 = **24 raw combinations**. Constraints from `docs/capability-matrix.md` and `internal/cloud/aws/stack/config.go`:

- `queueMode: amazon-mq` × `preview` preset is **incompatible by design** — `CLUSTER_MULTI_AZ` needs 3 AZs, `preview` is 2-AZ `[VERIFIED: docs/capability-matrix.md:75; REQUIREMENTS.md Out of Scope]`. A matrix must never assert on it under `preview`.
- `searchMode` defaults are preset-derived: `preview → serverless`, otherwise `provisioned` (`config.go:227-235`); `queueMode` defaults `preview → db`, otherwise `amazon-mq` (`config.go:237-245`).
- `searchMode: provisioned` requires exactly 2 subnets on `standard` and 3 on `high-availability` (`search.go:249-256`), so it cannot be exercised under `preview`.
- `webRuntime: frankenphp-classic` needs a matching image digest — not a free toggle (`docs/capability-matrix.md:77`), but that is a *deploy-time* constraint; the test can supply any valid digest.
- `ecs-artemis` is experimental but uses the same network path as `ecs-rabbitmq` (`docs/capability-matrix.md:74`).

**Resulting legal matrix:** enumerate per preset, not as a flat 24.

| Preset | searchMode | queueMode | webRuntime | cells |
|---|---|---|---|---|
| `preview` | `disabled`, `serverless` | `db`, `ecs-rabbitmq`, `ecs-artemis` | both | 2 × 3 × 2 = **12** |
| `standard` | `disabled`, `provisioned` | `db`, `amazon-mq`, `ecs-rabbitmq`, `ecs-artemis` | both | 2 × 4 × 2 = **16** |

28 legal cells is too many for one dense test at Pulumi-mock cost. **Recommendation:** a two-level strategy.

- **Level 1 — stack-level projection matrix** (cheap, no Pulumi run): a table-driven test over `(preset, searchMode, queueMode, webRuntime)` calling only the pure projection helpers — `queueConsumerCount(spec)`, `effectiveQueueMode(spec)`, `searchProxyImage` selection, `varnishImageFor(mode)` — and asserting the derived `runtime.Args` shape. Cover **all 28** legal cells here plus an explicit `wantRejected: true` row for `preview` × `amazon-mq`. This is where mis-projection bugs live and it costs milliseconds.
- **Level 2 — runtime container-graph matrix** (Pulumi mocks, one `deploy()` per row): cover exactly the **six** uncovered interactions U1-U6 above, plus the two already-covered anchors as regression sentinels. Use the existing `validArgs()` + targeted-mutation convention (`.planning/codebase/TESTING.md`) and `t.Parallel()` on subtests as the file already does.

Shape (matches `internal/cloud/aws/queue/component_test.go:77-107` and this repo's conventions):

```go
func TestRuntimeCatalogCellCombinations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		webRuntime       string
		applicationMode  string
		searchProxy      bool
		queueConsumers   int
		queueMode        string
		wantContainers   []string   // names present in the web task definition
		wantProxyDependent string   // container that must DependsOn search-proxy, "" for none
	}{
		{name: "frankenphp_integrated_search_on", webRuntime: "frankenphp-classic", applicationMode: "integrated",
			searchProxy: true, queueMode: "rabbitmq",
			wantContainers: []string{"web", "search-proxy", "varnish"}, wantProxyDependent: "web"},
		// U1..U6 rows
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := validArgs()
			// deep-copy slice fields before mutating to avoid cross-case pollution
			args.Secrets = append([]SecretReference(nil), args.Secrets...)
			// ... apply tt
			m := deploy(t, args)
			// assert on decodeDefinitions(t, ...) / definitionsByName(...)
		})
	}
}
```

Note the slice deep-copy: `validArgs()` returns a fresh `Args` each call, but `TestRuntimeRejectsUnsafeInputsBeforeRegistration:435-436` mutates `args.Secrets[0]` in place, so preserving the documented deep-copy habit avoids reintroducing the class of bug the convention exists to prevent.

---

## Q5 — CIDR carving per provider (QUALITY-05)

Read all four implementations. They are **not four variants of one algorithm** — they are four different designs, and only one has a cap.

| Provider | Function | Carve semantics | Index cap enforced? | Tests today |
|---|---|---|---|---|
| **AWS** | `subnetCIDRs(prefix, count)` — `internal/cloud/aws/network/network.go:435-446`, called from `validate` at `:391` with `count = len(args.AvailabilityZones) * 3` | `bits = prefix.Bits() + 4`; carves `count` consecutive `/(bits)` blocks by `base + index*step` | ❌ **No cap.** `2^4 = 16` slots exist. `count = zones*3`, so `zones ≥ 6` silently emits CIDRs **outside the VPC**. Canonical prefix and `Bits() ≤ 24` *are* validated (`:388-390`). | `network_test.go:143 TestNetworkRejectsInvalidInputsBeforeRegistration` — no boundary case |
| **OVH** | `validateSubnetCarve(cidr, zones)` `:102-123` + `subnetCIDR(prefix, index)` `:125-138` | one `/24` per zone; `available = min(1<<(24-bits), 16)` | ✅ `index < 0 \|\| index > 15` rejected; zones-vs-available rejected. This is the `780a969` fix. | `network_test.go` has 4 tests: `…Overflow:8`, `…NarrowerThanSlash24:19`, `…OK:30`, `…CapsWidePrefixes:41` — **the closest thing to a boundary suite that exists** |
| **GCP** | `subnetCIDRs(prefix, index)` — `internal/cloud/gcp/network/network.go:140-153`, called at `:72` | rejects `Bits() > 20`; per index carves **two** `/24`s (even private, odd public) at `base + (index*2)<<8`; uses `.Masked()` | ❌ **No index cap.** `/20` holds 16 `/24`s ⇒ 8 legal indices (0-7). `index = 8` silently lands outside the parent prefix. | **No test file at all** in `internal/cloud/gcp/network/` |
| **Scaleway** | none — `internal/cloud/scaleway/network/network.go` | single Private Network carrying the whole `NetworkCIDR`; `PrivateSubnetIDs = [pn.ID()]` | **N/A — no carving, no index** | `network_test.go:60`, `:93` cover VPC+PN creation and input rejection |

### Verdict on ROADMAP criterion 3's CIDR clause

> *"subnet index at cap and cap+1 for every provider"*

**Not achievable as written, for two independent reasons:**

1. **Scaleway has no subnet index.** There is nothing to put at cap or cap+1. The honest test is a *negative* assertion: `TestScalewayCarvesNoPerZoneSubnets` — one Private Network, `len(PrivateSubnetIDs) == 1` regardless of zone count — documenting the single-PN model so a future change is loud.
2. **AWS and GCP have no cap to test.** QUALITY-05 as written implies "add tests"; the code says "add the guard, then test it." That is a behaviour change, not a test addition, and the plan must budget for it.

**Recommended reword:** *"Every provider that carves subnets rejects an out-of-range index before Pulumi registration, with tests at min index, max index, and max index + 1; the provider that does not carve has a test asserting it creates exactly one network range."*

### Recommended shape — per-provider, in place. No shared helper.

A shared helper is the wrong call here, and not only because of the project's no-unrequested-abstractions stance:

- ADR 0008 keeps cloud-shaped concerns per-provider; a shared `internal/network/carve` package that four adapters import would be new cross-adapter coupling of exactly the kind ADR 0002/0004 exists to prevent.
- The four semantics genuinely differ: AWS carves `zones*3` blocks at `bits+4`; OVH carves one `/24` per zone; GCP carves two `/24`s per index with even/odd roles; Scaleway carves nothing. A helper covering all four would take a strategy parameter and be harder to read than four 15-line functions.
- The OVH `780a969` bug was an *off-by-one in a cap*, which a shared helper would not have prevented — the cap value is provider policy.

So: four independent, small test additions, each mirroring the existing OVH test pattern (`internal/cloud/ovh/network/network_test.go` — plain functions, no Pulumi mocks needed for the pure carve functions). What *should* be shared is the **naming and case list**, as a convention documented in `docs/lint-policy.md`'s sibling or in `CONTRIBUTING.md`: every carve function gets `TestXxxSubnetCarveBoundaries` with rows `min`, `max`, `max+1`, `narrower-than-minimum-prefix`, `non-canonical-prefix`. Convention, not code.

**Also fix, in the same tasks:**
- AWS: add an explicit `len(AvailabilityZones)*3 <= 1<<4` check in `validate` (`network.go:387-392`) returning an error naming the limit, before `subnetCIDRs` is called. Today the preset presets bound zones to ≤ 3 so it is latent, not live — but it is one config field away from emitting subnets outside the VPC.
- GCP: add an index cap of `(1 << (24 - prefix.Bits())) / 2` to `subnetCIDRs` (`gcp/network/network.go:140`) and reject at the caller `:72`.
- GCP: `internal/cloud/gcp/network/` gains its first test file.

---

## Q7 — Splitting `runtime.go` (QUALITY-07)

`internal/cloud/aws/runtime/runtime.go` is **991 lines** `[VERIFIED: wc -l]`; the package contains only it and `runtime_test.go` (586 lines).

### Why `runtime_test.go` passes unchanged — by construction

Every identifier `runtime_test.go` references is **package-scoped**: `TypeToken`, `ApplicationPort`, `VarnishPort`, `Args`, `SecretReference`, `CapabilityConfig`, `New`, `FrontendPort`, `searchProxyTarget`, `amqpSettings` `[VERIFIED: read all 586 lines]`. Moving declarations between files **inside the same package** is invisible to the compiler and to the test. There is no cgo, no `init()`, no build tag, no `go:generate` in the package. The only package-level variables are four independent `regexp.MustCompile` values (`runtime.go:29-32`) that reference nothing else, so Go's package-variable initialisation order is irrelevant to them. **Nothing blocks a pure move.**

**One caveat that must be handled first.** `runtime_test.go` is currently **not gofmt-clean** — `gofmt -d` shows a struct-field alignment diff at `runtime_test.go:278` (`name string` → `name     string` in `TestSearchProxyTargetServiceName`) `[VERIFIED: gofmt -d]`. So "the pre-existing suite passes unchanged" is literally false today: CI's `gofmt -w` + `git diff --exit-code` step (`ci.yml:79-82`) fails on it. **Wave 0 must gofmt it, and that gofmt'd file is the immutable baseline for criterion 2.** State this in the plan so nobody later argues the criterion was violated.

### Proposed split — 7 files, all under 400 lines

Line ranges are from the current file, verified by reading.

| New file | Moves | Approx. lines |
|---|---|---|
| `types.go` | package doc, consts `20-27`, regexes `29-32`, `SecretReference`+`ValueFrom` `34-45`, `CapabilityConfig` `47-58`, `IdentityArgs`/`Identity` `60-74`, `Args` `76-100`, `Component` `102-120` | ~105 |
| `identity.go` | `NewIdentity` `122-155`, `createIdentity` `157-163`, `provisionIdentity` `165-189`, `role` `486-488`, `executionPolicy` `490-523`, `attachExecutionLogPolicy` `525-541`, `executionLogPolicy` `543-558` | ~140 |
| `component.go` | `New` `191-354`, `targetGroupInput` `356-361`, `tags` `977-991` | ~185 |
| `validate.go` | `validate` `363-446`, `validateSecretReferences` `448-463`, `sameSecretReferences` `465-475`, `hasSecretReference` `477-484` | ~125 |
| `containers.go` | container struct types `618-683`, `containerDefinitionsInput` `560-580`, `searchEndpointInput` `582-587`, `FrontendPort` `589-597`, `containerDefinitions` `599-616`, `containerDefinitionsForInput` `685-705`, `containerDefinitionsFor` `707-720`, `baseContainer` `846-864`, `nginxHealthCheck` `835-844`, `awslogsConfig` `866-888` | ~200 |
| `sidecars.go` | `appendSearchProxy` `722-745`, `appendVarnish` `747-785`, `searchProxyTarget` `787-799` | ~80 |
| `env.go` | `appendDatabaseSecret` `801-819`, `appendEncryptionSecret` `821-829`, `deploymentCommand` `831-833`, `capabilityEnvironment` `890-947`, `containerEnvFromBindings` `949-955`, `amqpSettings` `957-975` | ~110 |

Total ≈ 945 lines of moved code plus 7 package clauses and import blocks. Largest file ≈ 200 lines — **half the 400-line cap**, leaving genuine headroom for the ECS Managed Instances work deferred to v2.

Note `runtime.go` itself disappears; the constructor lives in `component.go`, matching the repo's own convention (`internal/cloud/aws/queue/component.go`, `security/component.go`, `stack/component.go` all use `component.go` for `New`) `[VERIFIED: CONVENTIONS.md + file listing]`.

### Verdict on ROADMAP criterion 2

> *"No file under `internal/cloud/aws/runtime/` exceeds 400 lines, and the pre-existing `runtime_test.go` suite passes unchanged after the split"*

**Achievable and well-specified**, with one amendment: `runtime_test.go` (586 lines) itself exceeds 400. Read literally, the criterion fails on the *test* file the moment the split lands. Either scope the cap to non-test files, or split the test file too (which would violate "passes unchanged"). **Recommend rewording to "no non-test file under `internal/cloud/aws/runtime/` exceeds 400 lines"** and note the test file is intentionally exempt.

### How to make it behaviour-preserving rather than hand-moved

1. `gofmt` the drifted files first (Wave 0); commit that alone.
2. Record the baseline: `go test -race ./internal/cloud/aws/runtime/ -count=1 -v > /tmp/runtime-baseline.txt` (single package, cheap, safe on this Mac).
3. Move declarations with **gopls's `Move` / `Extract` code actions** (available through the `golang-gopls` skill or Serena's symbolic edit tools) rather than cut-and-paste. Tool-driven moves cannot silently drop a receiver or reorder a `const` block.
4. **One file per commit.** After each: `gofmt -l`, then `go test -race ./internal/cloud/aws/runtime/ -count=1`, then diff the `-v` output against the baseline. The test-name/PASS list must be byte-identical.
5. Do **not** rename, reorder parameters, change visibility, or "tidy" anything during the split. If a genuine improvement appears, note it and do it in a separate follow-up commit after criterion 2 is met — otherwise the "unchanged tests" evidence is worthless.
6. Final gate: `git diff --stat` on the whole split should show only file creations/deletions and zero net line change beyond import blocks. A large net delta means logic moved, not code.

---

## Q8 — OVH regression guards (QUALITY-08)

### The concerns audit points at the wrong file

CONCERNS.md attributes both OVH bugs to `internal/cloud/ovh/stack/ops.go`. Verified against git:

| Bug | Commit | Actual file | Test coverage today |
|---|---|---|---|
| MKS node pool not wired into k8s workload dependencies | `37081b7` | **`internal/cloud/ovh/runtime/runtime.go`** (`pool` handle captured at `:106`; `DependsOn{cluster, pool}` at `:125` and `:129`) | **None — `internal/cloud/ovh/runtime/` has no `_test.go` file at all** |
| Subnet carve off the `/24` index cap | `780a969` | `internal/cloud/ovh/network/network.go:102-138` | 4 tests in `network_test.go`, incl. `TestValidateSubnetCarveOverflow` and `…CapsWidePrefixes` — see §Q5 for whether they hit cap/cap+1 exactly |

`ops.go` is a pure `unsupported{}` stub shell and was not touched by either fix `[VERIFIED: git show 37081b7, git show --stat 780a969]`.

### DependsOn *is* assertable under Pulumi mocks — key enabler

`pulumi.MockResourceArgs` exposes `RegisterRPC *pulumirpc.RegisterResourceRequest` `[VERIFIED: sdk/v3@v3.253.0/go/pulumi/mocks.go:107]`, populated from the incoming request at `mocks.go:326`, and `RegisterResourceRequest` carries `Dependencies []string` (URNs, protobuf field 15) `[VERIFIED: proto/go/provider.pb.go:2827]`. Also verified: `pulumi.DependsOn` **accumulates** (`ro.DependsOn = append(...)` at `resource.go:859`), so the extra `DependsOn{web}` on the Service at `ovh/runtime/runtime.go:170` adds to `{cluster, pool}` rather than replacing it — the fix is intact.

**No existing test in this repo uses `RegisterRPC`** `[VERIFIED: grep across all _test.go]`, so this is a new pattern for the codebase. It is the only sound way to assert dependency ordering — do **not** assert on the order of the recorded-resources slice, because the Go SDK registers concurrently.

Recommended harness addition (new file `internal/cloud/ovh/runtime/runtime_test.go`):

```go
type node struct {
	typeToken, name string
	urn             string
	dependencies    []string
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	n := node{typeToken: args.TypeToken, name: args.Name}
	if args.RegisterRPC != nil {
		n.dependencies = args.RegisterRPC.GetDependencies()
	}
	m.nodes = append(m.nodes, n)
	m.mu.Unlock()
	return args.Name + "-id", args.Inputs, nil
}
```

Then assert that the `kubernetes:apps/v1:Deployment` for `-web`, the `kubernetes:core/v1:Service`, and the `pulumi:providers:kubernetes` node each list a dependency URN containing `-pool`. Guard the assertion by *substring on the URN*, not exact URN, so a stack-name change does not break it.

Note the pool's own `DependsOn{cluster}` at `runtime.go:114` should also be asserted — that ordering matters too and is equally untested.

**Cross-provider bonus, cheap:** the same assertion applies to `internal/cloud/scaleway/runtime/` (whose Kapsule adapter is the pattern `37081b7` mirrored) and `internal/cloud/gcp/runtime/`. Their existing test files only cover `kube.BuildStaticTokenKubeconfig` `[VERIFIED: read both]`. Adding the pool-dependency assertion to all three closes the class of bug rather than the instance, and directly de-risks Phase 6's shared Kubernetes layer.

---

## T1 — Experimental-tier warning before mutate (TRUST-01)

### The choke point, identified by reading the code

`(*options).planStack(allowExpiredPreview bool)` at **`internal/cli/lifecycle.go:269-282`**. It is the single funnel: **18 call sites**, all of which run *before* any backend is created `[VERIFIED: grep + read every call site]`.

| File:line | Command(s) |
|---|---|
| `lifecycle.go:81` | `preview`, `deploy`, `destroy` (via `executeInfrastructure`) |
| `lifecycle.go:249` | `outputs` |
| `bootstrap.go:17` | `bootstrap` |
| `exec.go:168` (`plannedOutputs`) | `exec`, `ssh`, `cache-flush`, `reindex`, `cron-run`, `queue-status` |
| `secrets.go:37,64,97` | `secret list`, `secret set`, `secret remove` |
| `state.go:22,46,67,103` | `state status`, `state unlock`, `state backup`, `state restore` |
| `releases.go:105` | `promote`, `rollback` (→ `runDeploymentWithOptions`) |
| `logs.go:31` | `logs` |
| `health.go:61` | `health` |
| `cost.go:20` | `cost` |
| `login.go:11` | `login` |

Ordering is verified safe: `executeInfrastructure` calls `planStack` at `:81` and only reaches `o.newBackend(...)` at `:104`; `plannedOutputs` calls `planStack` at `:168` before `newBackend` at `:175`. **A warning emitted inside `planStack` is guaranteed to precede every Pulumi invocation.**

`platform.PlannedStack` already exposes `CertificationTier() CertificationTier` `[VERIFIED: internal/platform/module.go:36]`, and `TierCertified`/`TierExperimental` are declared at `module.go:16-17`. No new interface method is needed. ADR 0002/0004 is respected: the CLI reads the tier off the port, never off a cloud package.

### Which commands are genuinely mutating — enumerated from the actual tree

| Mutating (touches cloud state) | Read-only |
|---|---|
| `deploy`, `destroy`, `bootstrap`, `secret set`, `secret remove`, `state unlock`, `state backup`, `state restore`, `promote`, `rollback`, `env destroy`, `env sweep`, `exec`, `ssh`, `cache-flush`, `reindex`, `cron-run` | `preview`, `outputs`, `status`, `cost`, `health`, `logs`, `secret list`, `state status`, `queue-status`, `history`, `env list`, `config *`, `version`, `doctor` |

`env create` and `env protect` only edit `magelift.yaml` and never call `planStack` `[VERIFIED: read env.go:46-160, :321]` — correctly out of scope. `env destroy`/`env sweep` reach `planStack` transitively via `destroyEnvironment`.

### Recommendation: warn in `planStack`, unconditionally, once

Add the tier warning inside `planStack` for **all** commands, not just mutating ones, and write it to `o.stderr`.

Why unconditional rather than a `mutating bool` parameter:
- It cannot have a gap. A `mutating` flag would need to be threaded through 18 call sites and every future one; the first forgotten `false` is a silent honesty regression, which is exactly what TRUST-01 exists to prevent.
- `o.stderr` never pollutes `-o json`/`-o yaml` output, which goes to `o.stdout` via `o.write` (`root.go:403-423`) `[VERIFIED]`. Machine consumers are unaffected.
- Over-warning on `magelift logs --provider ovh` is correct behaviour for an experimental target, not noise.
- It is a ~6-line change in one function.

The warning must name **the tier, the provider/runtime, and the doc page** — reuse the project's own `usererr` vocabulary for consistency (`internal/usererr`), or a plain `fmt.Fprintf(o.stderr, ...)` since this is a warning and not an error. Include a `--no-interaction`-independent, non-blocking form: TRUST-01 asks for a warning, not a confirmation prompt. Do **not** add a prompt — that would break CI usage and contradict `--no-interaction` semantics.

### Testability, and the stub gap the plan must close

The existing harness is `newCommandWithOptions(o)` with `testOptions(&out, terminal)` (`root_test.go:26-37`) plus `registerTestModules` (`stub_module_test.go:80-84`). Assertions are `strings.Contains(out.String(), …)`. Zero Pulumi, zero credentials — exactly what criterion 4 demands.

**Blocker:** `stubPlanned.CertificationTier()` returns a hard-coded `platform.TierCertified` (`stub_module_test.go:66-68`) and `stubAWSModule.Descriptor()` is fixed to `aws`/`ecs-fargate`. The registry rejects duplicate provider/runtime pairs (`module.go:102-104`), so the plan must add a **second stub** — e.g. `stubExperimentalModule` with `Descriptor{ID: "ovh.mks", Provider: "ovh", Runtime: "mks"}` and `CertificationTier() → TierExperimental`, plus a tier field on `stubPlanned`. Keep it in `stub_module_test.go` so `internal/cli` stays free of cloud imports (the 8.5 GB `gendocs` lesson).

The test then writes a `magelift.yaml` with `target: {provider: ovh, runtime: mks}` into `t.TempDir()`, runs `deploy` (or any mutating command), and asserts the stderr buffer contains the tier string **and** that no backend was constructed. Also assert the negative: an `aws`/`ecs-fargate` config produces **no** warning — otherwise the test passes trivially.

### Verdict on ROADMAP criterion 4

> *"Any mutating command against `gcp`, `ovh`, or `scaleway` prints an experimental-tier warning naming the tier before Pulumi is invoked"*

**Achievable, and the criterion is narrower than the code.** `aws`/`eks-autopilot` is *also* `TierExperimental` (`internal/cloud/aws/eksops/module.go:26-27, 51`) `[VERIFIED]`. Gating on the provider name would let EKS Autopilot mutate silently. **Gate on `CertificationTier() == TierExperimental`, not on a provider allowlist**, and reword the criterion to *"any mutating command against a target whose certification tier is experimental (today: `gcp/gke-autopilot`, `ovh/mks`, `scaleway/kapsule`, `aws/eks-autopilot`)"*. Add the EKS case as a test row.

---

## T2 — Loud failure for unimplemented day-2 (TRUST-02)

### The 15+15 count is exactly right

| File | `ErrNotSupported` occurrences | Lines |
|---|---|---|
| `internal/cloud/ovh/stack/ops.go` | **15** | 74 |
| `internal/cloud/scaleway/stack/ops.go` | **15** | 75 |
| `internal/cloud/gcp/ops/day2.go` | 1 | 330 |
| `internal/cloud/aws/eksops/ops.go` | 5 | 258 |

`[VERIFIED: grep -c]`. In OVH/Scaleway the 15 are the 14 `unsupported{}` methods (`VerifyAccount`, `Ensure`, `Status`, `Lock`, `Unlock`, `Backup`, `Restore`, `List`, `Set`, `Remove`, `TailLogs`, `CheckRuntime`, `PrepareExec`, `Estimate`) plus `Ops.NewDeploySteps`. The two files are byte-for-byte parallel apart from a comment. An enumeration test is straightforward — and because the two files are identical in shape, **one table shared by two subtests** is the right form, not two copies.

### Three genuine silent-success paths the criterion's 15 sites do not cover

These are the actual TRUST-02 violations, and none of them is an `ErrNotSupported` site:

1. **`magelift deploy` silently degrades to infra-only on experimental targets.** `internal/cli/lifecycle.go:176-180`:
   ```go
   if stepsErr != nil && !errors.Is(stepsErr, platform.ErrNotSupported) { ... }
   // ErrNotSupported or nil steps: infrastructure graph update only.
   ```
   An operator running `magelift deploy` against OVH gets a Pulumi update and **exit 0**, with no Magento migrate/cutover/health and no message saying so. This is precisely "a silent no-op or a false success." Fix: emit a loud, tier-named notice to stderr naming what was skipped, and make it a hard failure unless `--infra-only` was passed explicitly (the flag already exists at `lifecycle.go:66`).

2. **No-op deployment locks return success.** `Ops.AcquireLock` returns `func(context.Context) error { return nil }, nil` in **three** adapters: `ovh/stack/ops.go:68-70`, `scaleway/stack/ops.go:69-71`, `aws/eksops/ops.go:214-216` `[VERIFIED]`. The port's own doc comment sanctions it — *"Experimental targets may return a no-op release when they have no DIY lock yet"* (`internal/platform/ops.go:18-20`) — so this is **a documented contradiction of TRUST-02**, not an oversight. The planner must resolve it deliberately: either (a) warn loudly at acquire time naming the tier and the absence of locking, or (b) fail. Recommend (a) for this phase, with the resolution recorded as a decision, because (b) would break the OVH/Scaleway `preview`/`apply` paths that PROJECT.md lists as working today. Phase 6 (KUBE-05) makes real locking available and should flip it to (b).

3. **Three day-2 surfaces bypass the tier-naming helper.** `notSupported(err, planned, surface)` at `internal/cli/ports.go:75-80` already produces `"<surface> is not supported for target <provider>/<runtime> yet"` and is used by `bootstrap`, `exec`, `secrets` (×3), `state` (×4), `login` `[VERIFIED: grep]`. But `logs.go:56`, `health.go:101`, and `cost.go:34` each roll their own `errors.Is(err, platform.ErrNotSupported)` message, and the three `*Port()` helpers at `ports.go:40, 55, 70` emit bare `"… are not supported for this target yet"` with no provider, runtime, or tier at all.

### Recommendation

- Add `planned.CertificationTier()` to `notSupported`'s message — a one-line change that upgrades **eleven** call sites at once.
- Route `logs.go`, `health.go`, `cost.go` through `notSupported`; delete their bespoke messages.
- Give the three `*Port()` helpers the provider/runtime/tier treatment (they have the module, so they can reach the tier via `module.CertificationTier()`).
- Add the enumeration test as a **table over method names** driving each `unsupported{}` method by hand (Go has no reflection-free way to enumerate interface methods cleanly, and reflection over unexported types is fragile). 15 rows × 2 providers, each asserting `errors.Is(err, platform.ErrNotSupported)` **and** that any returned value is the zero value — that second assertion is what makes "no nil-success path" real rather than decorative.
- Add a **grep-style gate** as a second, cheaper guard: a test that reads `ops.go` for each provider and fails if any method body returns `nil` as its error without also returning `ErrNotSupported`. Crude but it survives refactoring and catches the case where someone adds a 16th stub method and forgets. Phase 6's KUBE-07 criterion already anticipates such a grep gate — build it here so Phase 6 inherits it.

### Verdict on ROADMAP criterion 5

> *"Every unimplemented day-2 command on an experimental target exits non-zero with a message naming the capability and its certification tier; a test enumerates the `ErrNotSupported` sites (15 each …) and asserts none of them returns a nil-success path"*

**Achievable, and the counts are correct.** But note it is satisfiable *while leaving all three real silent-success paths in place*, because none of them is one of the 30 enumerated sites. Add a sixth criterion, or extend criterion 5: *"…and no `Ops` method on an experimental target returns success without performing the operation — `AcquireLock`'s no-op release and `deploy`'s silent infra-only fallback are named explicitly."*

---

## Q1 — `internal/cli/cost.go` coverage (QUALITY-01)

### The deleted test cannot be restored

`git show HEAD:internal/cli/cost_test.go` tested `newCostReport(cfg, env)`, `newLiveCostReport(ctx, cfg, env, estimator)`, and an `awspricing.Query`-shaped estimator interface `[VERIFIED]`. **None of those symbols exists in `internal/cli` any more.** Cost moved behind `platform.CostEstimator` (`internal/platform/cost.go`, currently untracked) with the AWS implementation in `internal/cloud/aws/cost/estimate.go` (305 lines, 143 lines of tests). The deletion was correct; CONCERNS.md's framing ("deleted with no replacement … should block the change") mis-reads a refactor as a regression. Do not attempt to restore the old test.

### The genuine gap

`internal/cli/cost.go` (66 lines) has **zero** tests, and so does `platform.ModuleCostEstimator`. `grep -rln "costCommand\|testCostEstimator\|CostEstimator" --include="*_test.go" .` returns **nothing** `[VERIFIED]`.

The `options` struct already has the seam: `testCostEstimator platform.CostEstimator` (`root.go:75`), consulted first by `(*options).costEstimator()` (`cost.go:46-49`). So the test needs no new production code.

### Behaviours to cover, mapped to the requirement's three clauses

| Clause | Behaviour | Where |
|---|---|---|
| flag parsing | `--live` sets `platform.CostOptions{Live: true}`; absent → `Live: false` | `cost.go:32, 42` |
| flag parsing | `cobra.NoArgs` — a positional argument is rejected | `cost.go:18` |
| output | `-o json` / `-o yaml` / `-o table` render the `platform.CostReport`; an unsupported `-o` yields exit code 2 | `cost.go:39` → `root.go:403-423` |
| error paths | estimator returns `ErrNotSupported` → exit **2**, message `"cost estimation is not supported for target <p>/<r> yet"` | `cost.go:34-36` |
| error paths | estimator returns any other error → exit **3**, wrapped in `usererr` with cause/next | `cost.go:37` |
| error paths | module registered but implements no `CostEstimator` → exit 2 with the "not supported … yet" message | `cost.go:61-64` |
| `ErrNotSupported` on non-AWS | a registered experimental module whose `CostEstimator()` returns an `unsupported{}` → exit 2, tier named (see §T2) | `cost.go:34`, `ports.go` after the T2 fix |
| planning gate | an unregistered target fails before the estimator is consulted | `cost.go:20-23` |

Two files: `internal/cli/cost_test.go` (command behaviour, using `newCommandWithOptions` + `testCostEstimator`) and a small `internal/platform/cost_test.go` for `ModuleCostEstimator` (nil module → nil; module without `HasCostEstimator` → nil; module with it → the estimator). The latter is three assertions and closes an untested exported function on the stable `platform` surface — cheap, and RELEASE-02 will freeze that surface in Phase 2.

Assert **exit codes via `ExitCode(err)`**, matching `root_test.go:68-70`. That is what distinguishes "fails loudly" from "fails".

---

## Architecture Patterns

### CI lint data flow after partitioning

```
                          push / pull_request
                                  |
                                  v
                        +---------------------+
                        |  changes (filter)   |  dorny/paths-filter
                        +---------------------+
                                  | go == true
                                  v
                   +-----------------------------------+
                   |  cache-prime  (NEW)               |
                   |  setup-go + go build ./cmd/...    |
                   |  GOFLAGS=-p=1                     |
                   |  actions/cache save  if: always() |  <-- breaks the cold-cache loop
                   +-----------------------------------+
                                  |
        +----------+----------+----+-----+----------+-----------+
        v          v          v          v          v           v
   lint(aws)  lint(gcp)  lint(ovh)  lint(scw)  lint(core)  lint(aggregate)
   ./internal/cloud/aws/...        ...         ./internal/...  ./cmd/magelift/...
        |          |          |          |          |           |
        +----------+----------+----+-----+----------+-----------+
                                  |  all green
                                  v
                  +--------------------------------+
                  |  go-verify                     |
                  |  gofmt drift | genconfig       |
                  |  gendocs | go test -race       |
                  |  govulncheck | go-licenses     |
                  +--------------------------------+
                                  |
                                  v
                          result ("CI passed")
```

Two structural points. First, `cache-prime` must save with `if: always()` via an explicit `actions/cache` step, because `actions/setup-go`'s implicit post-step save never runs when the runner is reaped — which is why every run today starts cold. Second, `gofmt`/`genconfig`/`gendocs`/`go test` currently share one job with lint; splitting them means a lint failure no longer hides whether the tests pass. Keep both jobs in the `result` gate's `needs:` list so skipped-by-filter still counts as OK (`ci.yml:332-366`).

### `runtime.go` construction flow (what the split must preserve)

```
New(ctx, name, Args)                                    [component.go]
  |
  +-> validate(name, args) -----------------------------> []SecretReference
  |     (rejects BEFORE RegisterComponentResourceV2)     [validate.go]
  |
  +-> RegisterComponentResourceV2
  |
  +-> ecs.NewCluster
  |
  +-> identity: args.Identity ?? createIdentity(...)     [identity.go]
  |     +-> 3 x iam.NewRole  +-> executionPolicy
  |     +-> attachExecutionLogPolicy
  |
  +-> security group: args.WebSecurityGroupID ?? new
  |
  +-> web task def:    containerDefinitionsInput         [containers.go]
  |     +-> pulumi.All(capabilityEnvironment, dbARN, keyARN, searchEndpoint).ApplyT
  |           +-> containerDefinitions
  |                 nginx-fpm  -> [php-fpm, web] + appendSearchProxy + appendVarnish
  |                 frankenphp -> containerDefinitionsFor("web", exposePort) [same appends]
  |                                                      [sidecars.go]
  +-> deploy task def: containerDefinitionsForInput("deploy", MagentoMigrationShell())
  +-> web service      (LB targets varnish when integrated, else web)
  +-> cron task + service
  +-> queue task + service      IF QueueConsumerCount > 0
  +-> RegisterResourceOutputs
```

The `ApplyT` closures in `containerDefinitionsInput` / `containerDefinitionsForInput` capture `args` and `secrets` by value. Moving them to another file changes nothing about that capture. This is the flow the six uncovered combinations in §Q4 traverse.

### Pattern: assert dependency ordering via `RegisterRPC`

**What:** capture `args.RegisterRPC.GetDependencies()` in the mock and assert on dependency URNs.
**When to use:** any time `pulumi.DependsOn` carries correctness (QUALITY-08; Phase 6's four Kubernetes targets).
**Why not the alternative:** the recorded-resource slice order reflects goroutine scheduling, not dependencies.

```go
// Source: pulumi/sdk/v3@v3.253.0/go/pulumi/mocks.go:107,326 + proto/go/provider.pb.go:2827
if args.RegisterRPC != nil {
    deps := args.RegisterRPC.GetDependencies() // []string of URNs
}
```

### Pattern: tier warning at the single plan choke point

**What:** emit the experimental warning inside `(*options).planStack`, keyed on `PlannedStack.CertificationTier()`.
**When to use:** TRUST-01, and any future "before you touch the cloud" gate.
**Why here:** 18 call sites, all pre-backend; ADR-legal tier source; impossible to bypass without adding a 19th path that skips planning entirely (which would fail for other reasons).

### Anti-Patterns to Avoid

- **Reading the tier from a cloud package in `internal/cli`.** Violates ADR 0002/0004 and re-links Pulumi SDKs into `internal/cli`, reviving the 8.5 GB `gendocs` OOM.
- **Asserting Pulumi resource *registration order* as a proxy for dependencies.** Concurrent registration makes it flaky.
- **Building a shared cross-provider CIDR helper.** Four different semantics, cross-adapter coupling, and it would not have caught the bug it is meant to prevent.
- **Raising the lint timeout again.** Tried three times. The runner is killed, not merely slow.
- **A `go build ./...`-style warm-up without `-p=1`.** Exactly what `91b8ecb` did and `b5deb3d` reverted for OOM.
- **Renaming or tidying during the `runtime.go` split.** Destroys the "tests unchanged" evidence that is the whole point of criterion 2.
- **Writing the AOSS OCU test as though it encodes AWS behaviour.** AWS does not document the rule; claiming otherwise is the over-claiming TRUST-03/04 exist to stop.
- **A confirmation prompt for the tier warning.** TRUST-01 asks for a warning. A prompt breaks CI and `--no-interaction`.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---|---|---|---|
| Asserting `DependsOn` | URN string parsing, registration-order heuristics, a custom monitor | `MockResourceArgs.RegisterRPC.GetDependencies()` | The SDK already surfaces the exact wire dependencies. |
| Moving declarations between files | Manual cut-and-paste | gopls `Move`/`Extract` code actions (`golang-gopls` skill / Serena) | Tool-driven moves cannot drop a receiver or reorder a `const` block; that is the entire basis of criterion 2's evidence. |
| Measuring policy size without whitespace | A JSON re-marshal / whitespace stripper | `len()` on the existing `json.Marshal` output | `canonicalJSON` already emits compact JSON (`identity.go:161-167`); these documents contain no intra-string spaces, so `len` is a sound upper bound. |
| CLI output assertions | A new test harness | `newCommandWithOptions(o)` + `bytes.Buffer` + `ExitCode(err)` (`root_test.go:26-37`) | Established, Pulumi-free, credential-free. |
| Test doubles for day-2 ports | New mock framework | The `testBootstrap`/`testState`/`testSecrets`/`testRuntimeObserve`/`testCostEstimator` fields already on `options` (`root.go:71-75`) | The seams exist and are unused by any test today. |
| Enumerating stub methods | `reflect` over unexported `unsupported{}` | An explicit table of `{name string, call func() error}` | Reflection over unexported types is fragile; an explicit table also documents the surface. |
| Lint memory tuning | Custom `GOGC`/`GOMEMLIMIT` guesswork | Partition the package set first | No official golangci-lint memory guidance exists; partitioning attacks the cause (one process holding five SDK graphs), env knobs only shift the symptom. |
| Provider-name allowlists for tier gating | `switch provider { case "gcp", "ovh", "scaleway": }` | `CertificationTier() == TierExperimental` | The provider list already omits `aws/eks-autopilot`, which is experimental. Allowlists rot; the tier does not. |

**Key insight:** almost every mechanism this phase needs already exists in the repo or the pinned SDK and is simply unused — the `options` test seams, `PlannedStack.CertificationTier()`, `notSupported()`, `MockResourceArgs.RegisterRPC`, the OVH boundary-test pattern. The work is wiring and honesty, not construction. Any plan that introduces a new abstraction for one of these rows has drifted.

---

## Runtime State Inventory

QUALITY-07 is a refactor and QUALITY-06 changes CI wiring, so this section applies. Every category answered explicitly.

| Category | Items Found | Action Required |
|---|---|---|
| **Stored data** | **None.** The phase touches no datastore. `runtime.go`'s split changes no Pulumi resource name, no `TypeToken` (`magelift:aws:EcsRuntime` stays in `types.go`), no logical resource name, and no output key — so no Pulumi state migration or alias is needed. Verified: the split moves declarations only; `New`'s `ctx.RegisterComponentResourceV2(TypeToken, name, …)` call is unchanged. | None |
| **Live service config** | **GitHub branch protection / required status checks.** `ci.yml` uses a single required check named **`CI passed`** (`ci.yml:333-334`) whose `needs:` list is the gate. Splitting the `go` job into a lint matrix plus a verify job **changes that `needs:` list**, and any repository-side required-check configuration referencing job names other than `CI passed` would break. | Verify the repo's required-status-check setting names only `CI passed`; update `needs:` in `ci.yml` to include every new job. **Do not rename `CI passed`.** |
| **OS-registered state** | **None.** No Task Scheduler / launchd / systemd / pm2 registration in this repo. | None |
| **Secrets / env vars** | **None changed.** The phase adds no secret and renames none. CI uses only `github.token` for `paths-filter`. `PULUMI_BACKEND_URL` is read at `lifecycle.go:103` and unaffected. | None |
| **Build artifacts / caches** | **GitHub Actions caches are stale/absent and must be treated as a first-class item.** `actions/setup-go` reported `Cache is not found` and `golangci-lint-action` reported `Cache not found for input keys: golangci-lint.cache-Linux-2951-…`. The `2951` segment is derived from the working directory; changing `working-directory` or adding a matrix changes the cache key. Also: no cache was ever *saved* on `main`, because the runner is killed before the post-step. | Add an explicit `actions/cache` save with `if: always()` in the new `cache-prime` job. Expect the first two or three CI runs after the change to still be slow (cold) — do not read that as failure. |

**The one non-obvious risk:** the required-status-check name. Restructuring jobs while a branch-protection rule references an old job name silently blocks merges. Check it before the first partitioned PR.

---

## Common Pitfalls

### Pitfall 1: Declaring QUALITY-06 done because the lint matrix is green
**What goes wrong:** the partitioned lint jobs pass, but the `go` job's *other* steps — `go test -race ./...`, `govulncheck`, `go-licenses` — have still never run in CI. The phase ships with criterion 3 unproven.
**Why it happens:** lint is step 4 of 7 in the `go` job; it has always failed there, so steps 5-7 have never executed. `[VERIFIED: no green go job in 38 runs]`
**How to avoid:** treat "the whole `go` job green on a real PR" as the acceptance gate, not "lint green". Watch the run and read the `go test -race` output.
**Warning signs:** any plan that verifies QUALITY-06 by reading `.golangci.yml` rather than a CI run URL.

### Pitfall 2: `go test -race ./...` does not fit the runner
**What goes wrong:** `-race` roughly doubles memory. On 2 vCPU / 7 GB, over a graph that peaks at ~8.5 GB when *linked*, `go test -race ./...` may OOM the same way lint does — and we have **no evidence either way**, because it has never executed in CI.
**Why it happens:** the same root cause as QUALITY-06, in a different tool.
**How to avoid:** prove it locally first, serially — `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./... -count=1` in a plain Terminal, not from Cursor (AGENTS.md). If it fails in CI, partition `go test` by the same six package groups. Budget a task for this contingency.
**Warning signs:** a `go` job that dies with no test output after ~15-30 minutes.

### Pitfall 3: The `runtime.go` split "passes unchanged" against an unformatted baseline
**What goes wrong:** `runtime_test.go` is not gofmt-clean today, so CI's `gofmt -w` + `git diff --exit-code` fails on it. If the split lands first, the gofmt fix touches the test file and criterion 2's "unchanged" claim is void.
**Why it happens:** `gofmt -l` currently reports three files — `internal/platform/cost.go`, `internal/cloud/aws/cost/estimate_test.go`, `internal/cloud/aws/runtime/runtime_test.go` `[VERIFIED]`.
**How to avoid:** Wave 0 commits the gofmt fix alone, before any other task. Capture the `go test -v` baseline from the gofmt'd tree.
**Warning signs:** `gofmt -l` output that is not empty at any point after Wave 0.

### Pitfall 4: Partitioned lint silently reports fewer issues
**What goes wrong:** a package pattern typo (or a directory added later that no partition covers) drops packages from linting. The matrix is green because nothing was checked.
**Why it happens:** `./internal/...` excluding `./internal/cloud/...` is not expressible in one Go pattern; it needs an explicit list or a shell-generated one, and a new top-level directory would be missed.
**How to avoid:** add a test or CI step that asserts the union of the matrix's package patterns equals `go list ./...`. This is the single most important guard in QUALITY-06 — without it, the requirement can be "met" by linting less. Also diff the partitioned issue set against a local `./...` run once, per §Q6.
**Warning signs:** total issue count drops after partitioning with no code change.

### Pitfall 5: The tier warning is asserted by a test that would pass without the feature
**What goes wrong:** a test asserting `strings.Contains(out, "experimental")` against a config whose stub module is `TierCertified` — the warning never fires and the assertion is on some other text, or the test checks only the positive case and would also pass if the warning fired unconditionally.
**Why it happens:** `stubPlanned.CertificationTier()` is hard-coded to `TierCertified` (`stub_module_test.go:66-68`), so a naive test cannot even reach the experimental branch.
**How to avoid:** add the experimental stub, and assert **both** directions — warning present for `ovh/mks`, warning **absent** for `aws/ecs-fargate`.
**Warning signs:** a single-case tier test; a test that passes before the production change lands.

### Pitfall 6: Combination tests assert on designed-invalid cells
**What goes wrong:** the matrix includes `preview` × `amazon-mq`, which is incompatible by design (2 AZ vs `CLUSTER_MULTI_AZ`'s 3). The test either fails or, worse, is "fixed" by weakening the guard that makes it fail.
**Why it happens:** 3 × 4 × 2 = 24 is a tempting flat enumeration.
**How to avoid:** enumerate per preset (see §Q4's table) and add an explicit `wantRejected: true` row for the incompatible cell so the guard itself is under test.
**Warning signs:** any test change that touches `internal/cloud/aws/queue/component.go`'s AZ validation.

### Pitfall 7: Treating the repo's private→public visibility change as the lint fix
**What goes wrong:** the OOM eases when `ubuntu-latest` goes from 2 vCPU / 7 GB to 4 vCPU / 16 GB at publication, the partitioning task is dropped as unnecessary, and the problem returns with the next provider.
**Why it happens:** the improvement is real and roughly 2.3× on memory.
**How to avoid:** land and verify the partitioning **while the repo is still private**, so the evidence is taken under the harder constraint. Record in `docs/lint-policy.md` that CI measurements predate the visibility change.
**Warning signs:** a plan that mentions "will improve when public".

---

## Code Examples

### Assert node-pool dependency ordering (QUALITY-08)

```go
// Source: pulumi/sdk/v3@v3.253.0/go/pulumi/mocks.go:92-110 (MockResourceArgs.RegisterRPC),
//         mocks.go:326 (RegisterRPC populated), proto/go/provider.pb.go:2827 (Dependencies field 15)
type node struct {
	typeToken, name string
	dependencies    []string // URNs
}

type mocks struct {
	mu    sync.Mutex
	nodes []node
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	n := node{typeToken: args.TypeToken, name: args.Name}
	if args.RegisterRPC != nil {
		n.dependencies = args.RegisterRPC.GetDependencies()
	}
	m.mu.Lock()
	m.nodes = append(m.nodes, n)
	m.mu.Unlock()
	return args.Name + "-id", args.Inputs, nil
}
func (*mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) { return args.Args, nil }

func (m *mocks) dependsOnSubstring(t *testing.T, token, name, want string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		if n.typeToken != token || n.name != name {
			continue
		}
		for _, dep := range n.dependencies {
			if strings.Contains(dep, want) {
				return
			}
		}
		t.Fatalf("%s/%s dependencies %v do not include %q", token, name, n.dependencies, want)
	}
	t.Fatalf("resource %s/%s not found", token, name)
}
```

### Tier warning at the single choke point (TRUST-01)

```go
// internal/cli/lifecycle.go — inside (*options).planStack, after modules.Plan succeeds.
// Tier comes from platform.PlannedStack (internal/platform/module.go:36); no cloud import.
func (o *options) planStack(allowExpiredPreview bool) (string, platform.PlannedStack, error) {
	// ... existing resolve + modules.Plan ...
	o.warnExperimentalTier(planned)
	return environment, planned, nil
}

func (o *options) warnExperimentalTier(planned platform.PlannedStack) {
	if planned == nil || planned.CertificationTier() != platform.TierExperimental {
		return
	}
	_, _ = fmt.Fprintf(o.stderr,
		"warning: target %s/%s is certification tier %q. Day-2 operations may be unimplemented "+
			"and this target has no real-account acceptance evidence. See docs/capability-matrix.md.\n",
		planned.Provider(), planned.Runtime(), planned.CertificationTier())
}
```

Warning goes to `o.stderr`; `o.write` sends payloads to `o.stdout` (`root.go:403-423`), so `-o json` stays parseable.

### Experimental stub module for CLI tests (TRUST-01 testability)

```go
// internal/cli/stub_module_test.go — keeps internal/cli free of Pulumi cloud SDKs
// (docs/knowledge/lessons/MageLift gendocs must not link Pulumi cloud SDKs.md).
type stubExperimentalModule struct{}

func (stubExperimentalModule) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "ovh.mks", Provider: "ovh", Runtime: "mks"}
}
func (stubExperimentalModule) CertificationTier() platform.CertificationTier {
	return platform.TierExperimental
}
func (stubExperimentalModule) OutputKeys() []string                        { return platform.RequiredOutputKeys() }
func (stubExperimentalModule) Program(platform.PlannedStack) (pulumi.RunFunc, error) { return nil, nil }
func (stubExperimentalModule) Plan(cfg config.Config, environment string, _ platform.PlanOptions) (platform.PlannedStack, error) {
	return stubPlanned{
		stackName: cfg.Project.Name + "-" + environment, provider: "ovh", runtime: "mks",
		project: cfg.Project.Name, environment: environment, region: cfg.Defaults.Region,
		tier: platform.TierExperimental, // stubPlanned.CertificationTier() must read this field,
		                                 // not the hard-coded TierCertified at stub_module_test.go:66-68
	}, nil
}
```

### IAM policy size guard, quota-aware and margin-aware (QUALITY-02)

```go
// Managed policies: 6,144 characters. Inline role policies: 10,240 aggregate.
// Role trust policies: 2,048 default (raisable to 8,192).
// "IAM doesn't count white space when calculating the size of a policy against these limits."
// Source: https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html
// canonicalJSON uses json.Marshal (compact), so len() is a sound upper bound.
func TestIAMPolicyDocumentsStayUnderAWSCharacterQuotas(t *testing.T) {
	plan := testIdentityPlan(t)
	boundary, err := ciPermissionsBoundaryPolicy()
	if err != nil {
		t.Fatal(err)
	}
	const managed, inlineRole, trust = 6144, 10240, 2048
	for _, tc := range []struct {
		name, document string
		quota          int
	}{
		{"ci boundary (managed)", boundary, managed},
		{"state boundary (managed)", plan.PermissionsPolicy, managed},
		{"build boundary (managed)", plan.BuildPermissionsPolicy, managed},
		{"ci inline", plan.CIPermissionsPolicy, inlineRole},
		{"state inline", plan.StatePermissionsPolicy, inlineRole},
		{"build inline", plan.BuildPermissionsPolicy, inlineRole},
		{"ci trust", plan.CITrustPolicy, trust},
		{"state trust", plan.TrustPolicy, trust},
		{"build trust", plan.BuildTrustPolicy, trust},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if size := len(tc.document); size > tc.quota {
				t.Fatalf("%s is %d characters, over the AWS quota of %d", tc.name, size, tc.quota)
			} else if size*10 > tc.quota*9 {
				t.Fatalf("%s is %d characters, over 90%% of the %d quota — move detail to an inline policy now",
					tc.name, size, tc.quota)
			}
		})
	}
}
```

### Partitioned lint job, with the coverage guard

```yaml
# .github/workflows/ci.yml — replaces the single golangci-lint step.
  lint:
    needs: [changes, cache-prime]
    if: needs.changes.outputs.go == 'true'
    runs-on: ubuntu-latest
    timeout-minutes: 15          # falsifiable replacement for .golangci.yml timeout: 30m
    strategy:
      fail-fast: false
      matrix:
        include:
          - {name: aws,       packages: './internal/cloud/aws/...'}
          - {name: gcp,       packages: './internal/cloud/gcp/...'}
          - {name: ovh,       packages: './internal/cloud/ovh/...'}
          - {name: scaleway,  packages: './internal/cloud/scaleway/...'}
          - {name: core,      packages: './internal/cli/... ./internal/platform/... ./sdk/... ./cmd/genconfig/... ./cmd/gendocs/...'}
          - {name: aggregate, packages: './cmd/magelift/...'}
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0
        with: {persist-credentials: false}
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with: {go-version-file: go.mod}
      - uses: golangci/golangci-lint-action@ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a # v9.3.0
        with:
          version: v2.12.2
          args: ${{ matrix.packages }}
          skip-save-cache: ${{ matrix.name != 'aws' }}   # one writer; key includes working-directory
```

The `core` list is explicit rather than `./internal/...` because Go patterns cannot subtract `./internal/cloud/...`. That explicitness is precisely why the union guard below is mandatory:

```go
// A new top-level package that no partition covers must fail loudly, not lint silently.
func TestLintMatrixCoversEveryPackage(t *testing.T) {
	// parse .github/workflows/ci.yml, expand each matrix pattern with `go list`,
	// and assert the union equals `go list ./...`.
}
```

---

## State of the Art

| Old approach | Current approach | When changed | Impact on this phase |
|---|---|---|---|
| `golangci-lint` v1 config schema, `run.timeout` default 1m | v2 schema (`version: "2"`), `run.timeout` default **0 = disabled**, `run.concurrency` default 0 = container CPU quota | v2 series | Removing `timeout: 30m` removes a constraint. Use a job-level `timeout-minutes` instead. `[CITED: golangci-lint.run/docs/configuration/file/]` |
| `ubuntu-latest` = 2 vCPU / 7 GB everywhere | Public repos get 4 vCPU / 16 GB free; private repos stay 2 vCPU / 7 GB | 2024 open-source upgrade | The repo is private today, so all measurements are on the 7 GB tier. `[VERIFIED: gh repo view; CITED: docs.github.com, github.blog]` |
| AOSS capacity configured only account-wide via `UpdateAccountSettings` | **Collection groups** add per-group `capacityLimits` | recent AOSS feature | The rule MageLift enforces belongs to the newer, less-documented surface — hence §Q3's LOW confidence. `[CITED: developerguide/serverless-collection-groups.html]` |
| staticcheck `unused` whole-program mode | Removed / discouraged; caching made it unsound when analysing subsets | `dominikh/go-tools#671` | Supports partitioning being safe, but I could not confirm golangci-lint's exact treatment of exported identifiers. `[ASSUMED]` |

**Deprecated / outdated in the project's own docs:**
- `.planning/codebase/CONCERNS.md` line 57 ("no visible automated guard against future policy growth") — **false**; the guard exists at `identity_test.go:242`.
- `.planning/codebase/CONCERNS.md` lines 33-43 attribute both OVH bugs to `stack/ops.go`; they are in `ovh/runtime/runtime.go` and `ovh/network/network.go`.
- `.planning/codebase/CONCERNS.md` lines 45-49 frame the `cost_test.go` deletion as a coverage regression that "should block the change"; it was a consequence of a legitimate refactor.
- `.planning/codebase/CONCERNS.md` line 75 attributes the lint failure to concurrent type-checking; the live logs show a killed runner with zero linter output, and the 8.5 GB knowledge note gives the real magnitude.
- `.planning/codebase/CONVENTIONS.md` line 39 states lint is single-threaded "with a 30-minute timeout — do not parallelize lint invocations locally". QUALITY-06 changes this; update the note in the same phase or it becomes a trap for the next agent.

---

## Environment Availability

| Dependency | Required by | Available | Version | Fallback |
|---|---|---|---|---|
| Go toolchain | everything | ✓ | go1.26.5 (go.mod `go 1.26.0`) | — |
| `gofmt` | Wave 0 drift fix, `make fmt-check` | ✓ | bundled | — |
| `gh` CLI (authenticated) | reading CI logs to verify QUALITY-06 | ✓ | authenticated against `acourtiol/magelift` | none — without it, criterion 1 cannot be verified |
| `golangci-lint` v2.12.2 | `make lint` | via `go run` (network on first use) | v2.12.2 | — |
| `gopls` / Serena | behaviour-preserving moves in QUALITY-07 | ✓ (project uses Serena; `golang-gopls` skill available) | — | manual moves + per-file test gate (slower, riskier) |
| Docker | `make image-test`, `make floci-test`, `make varnish-test` | not probed | — | not needed by any Phase 1 requirement |
| PHP + Composer | `make php-test` (part of `make verify`) | not probed | — | **`make verify` includes `php-test`** — if PHP is absent locally, `make verify` cannot be the local gate; use the individual targets |
| `mkdocs` | `make docs` (part of `make verify`) | not probed | — | same caveat as PHP |
| AWS / GCP credentials | **none** | n/a | — | phase is fully offline by design |

**Missing dependencies with no fallback:** none identified for the Go work.
**Caveat the planner must handle:** `make verify` chains `generate-check cli-docs-check fmt-check lint test license-check php-test docs workflow-check` (`Makefile:78`). For a Go-only phase, prescribe the narrower gate `make fmt-check && make lint && make test` and treat full `make verify` as a Phase 2 concern (RELEASE-06 owns "clone to green `make verify`"). Otherwise a missing PHP or MkDocs toolchain will look like a Phase 1 failure.

---

## Validation Architecture

`workflow.nyquist_validation` is `true` in `.planning/config.json`.

### Test Framework

| Property | Value |
|---|---|
| Framework | Go stdlib `testing` (go1.26.5). No testify/ginkgo/gomock anywhere. |
| Config file | none — Go needs none |
| Quick run command | `go test -race ./internal/cloud/aws/runtime/ -count=1` (single package, seconds) |
| Full suite command | `go test -race ./... -count=1` — **must be run serially on this Mac**: `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./... -count=1` in a plain Terminal |
| Infra mocks | `pulumi.WithMocks("project", "stack", m)` with a per-package `mocks` type |
| CLI harness | `newCommandWithOptions(o)` + `bytes.Buffer` + `ExitCode(err)` |

### Phase Requirements → Test Map

| Req | Behaviour | Type | Automated command | File exists? |
|---|---|---|---|---|
| QUALITY-01 | `cost` flag parsing, output formats, `ErrNotSupported` → exit 2, other error → exit 3 | unit | `go test -race ./internal/cli/ -run TestCost -count=1` | ❌ Wave 0 (`internal/cli/cost_test.go`) |
| QUALITY-01 | `platform.ModuleCostEstimator` nil / absent / present | unit | `go test -race ./internal/platform/ -run TestModuleCostEstimator -count=1` | ❌ Wave 0 (`internal/platform/cost_test.go`) |
| QUALITY-02 | all 9 rendered policy documents under quota, with 90 % margin warning | unit | `go test -race ./internal/cloud/aws/bootstrap/ -run TestIAMPolicyDocumentsStayUnderAWSCharacterQuotas -count=1` | ⚠️ partial — 2 of 9 asserted inside `TestIdentityPlanIsRepoScopedAndLeastPrivilege` |
| QUALITY-03 | `validServerlessOCU` / `validCapacityRange` accept-and-reject table | unit | `go test -race ./internal/cloud/aws/search/ -run TestServerlessOCU -count=1` | ❌ Wave 0 (extend `search_test.go`) |
| QUALITY-04 | 28 legal cells at the projection level; 6 uncovered interactions at the container level | unit | `go test -race ./internal/cloud/aws/stack/ ./internal/cloud/aws/runtime/ -run 'Combination\|CatalogCell' -count=1` | ❌ Wave 0 (extend both `component_test.go` and `runtime_test.go`) |
| QUALITY-05 | AWS/GCP/OVH carve boundaries at min, max, max+1; Scaleway single-range assertion | unit | `go test -race ./internal/cloud/aws/network/ ./internal/cloud/gcp/network/ ./internal/cloud/ovh/network/ ./internal/cloud/scaleway/network/ -run Carve -count=1` | ⚠️ OVH partial; **GCP has no test file at all** |
| QUALITY-06 | partitioned lint matrix green; union of patterns == `go list ./...` | integration (CI) + unit | CI run URL for the matrix; `go test -race ./internal/... -run TestLintMatrixCoversEveryPackage` | ❌ Wave 0 |
| QUALITY-07 | no non-test file > 400 lines; `runtime_test.go` output byte-identical to baseline | unit + guard | `go test -race ./internal/cloud/aws/runtime/ -count=1 -v` diffed against baseline; plus a file-size guard test | ❌ Wave 0 (add a size-guard test so the cap cannot silently regress) |
| QUALITY-08 | k8s workloads and provider depend on the MKS node pool; pool depends on cluster | unit | `go test -race ./internal/cloud/ovh/runtime/ -count=1` | ❌ Wave 0 — **package has no test file** |
| QUALITY-08 | OVH subnet index at cap (15) and cap+1 (16) | unit | `go test -race ./internal/cloud/ovh/network/ -run Carve -count=1` | ⚠️ partial |
| TRUST-01 | experimental target warns before Pulumi; certified target does not | unit | `go test -race ./internal/cli/ -run TestExperimentalTierWarning -count=1` | ❌ Wave 0 |
| TRUST-02 | 15 sites × 2 providers return `ErrNotSupported` and zero values; error message names capability + tier | unit | `go test -race ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ ./internal/cli/ -run 'NotSupported\|Unsupported' -count=1` | ❌ Wave 0 |
| TRUST-02 | `deploy` does not silently degrade to infra-only | unit | `go test -race ./internal/cli/ -run TestDeployRefusesSilentInfraOnly -count=1` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** the single-package quick command for that task, plus `gofmt -l` (must be empty).
- **Per wave merge:** `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./... -count=1` serially in a plain Terminal, plus `make lint` (or the partitioned equivalent).
- **Phase gate:** a **green CI `go` job on a real PR** — not a local run. This is the only evidence that satisfies criteria 1 and 3, and it has never happened.

### Wave 0 Gaps

- [ ] `gofmt -w` on `internal/platform/cost.go`, `internal/cloud/aws/cost/estimate_test.go`, `internal/cloud/aws/runtime/runtime_test.go` — **blocks CI and blocks QUALITY-07's baseline**
- [ ] `internal/cloud/gcp/network/network_test.go` — package has no tests (QUALITY-05)
- [ ] `internal/cloud/ovh/runtime/runtime_test.go` — package has no tests (QUALITY-08)
- [ ] `internal/cli/cost_test.go` — QUALITY-01
- [ ] `internal/platform/cost_test.go` — QUALITY-01
- [ ] A shared `RegisterRPC`-capturing mock helper for the three k8s runtime packages (QUALITY-08 + Phase 6 groundwork)
- [ ] `stubExperimentalModule` + a `tier` field on `stubPlanned` in `internal/cli/stub_module_test.go` — TRUST-01 is untestable without it
- [ ] Framework install: none needed

---

## Security Domain

`security_enforcement` is not set to `false`, so this section applies. The phase is defensive by nature — it adds guards rather than attack surface — but three ASVS categories are directly in play.

### Applicable ASVS Categories

| ASVS category | Applies | Standard control in this phase |
|---|---|---|
| V1 Architecture | yes | ADR 0002/0004 boundary must hold: the tier warning reads `platform.PlannedStack`, never a cloud package. A violation re-links Pulumi SDKs into `internal/cli`. |
| V2 Authentication | no | No auth code in scope. OIDC trust policies are *rendered* (and now size-guarded) but not changed. |
| V3 Session Management | no | — |
| V4 Access Control | **yes** | QUALITY-02 guards the **IAM permissions boundary**, a least-privilege control. A boundary that silently fails to apply is a privilege-escalation risk, and `ciPermissionsBoundaryPolicy` is already a broad `service:*` allowlist whose only real constraint is the inline policy. The 90 %-margin warning matters more here than the hard limit. |
| V5 Input Validation | **yes** | QUALITY-03 (OCU values), QUALITY-05 (CIDR indices), and the existing reject-before-register discipline are all input validation. The AWS/GCP missing index caps are unvalidated-input defects, not merely untested ones. |
| V6 Cryptography | no | `MAGENTO_DC_CRYPT__KEY` handling is unchanged. QUALITY-07 must not alter `appendEncryptionSecret` or the `validate` checks at `runtime.go:427-441`. |
| V7 Error Handling / Logging | **yes** | TRUST-02 is an error-handling requirement. The tier warning must not leak account IDs, ARNs, or `PULUMI_BACKEND_URL` — it should name only provider, runtime, and tier. |
| V14 Configuration | yes | Partitioned lint must not reduce coverage (Pitfall 4). Keeping every SHA pin intact in the new matrix jobs is a supply-chain control. |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard mitigation | Status in this phase |
|---|---|---|---|
| Permissions boundary silently not applied because the document exceeds 6,144 chars | Elevation of Privilege | Size assertion in CI on every rendered document | QUALITY-02 — extend from 2 documents to 9 |
| Subnet carved outside the parent CIDR (AWS `zones ≥ 6`; GCP `index ≥ 8`) | Information Disclosure / Tampering | Reject out-of-range index before Pulumi registration | QUALITY-05 — **guard missing today**, must be added |
| Silent infra-only deploy leaves Magento un-migrated while reporting success | Repudiation | Fail loudly, or require an explicit `--infra-only` | TRUST-02 — `lifecycle.go:176-180` |
| No-op deployment lock permits concurrent mutation of the same stack | Tampering | Warn loudly naming the tier; real lock in Phase 6 (KUBE-05) | TRUST-02 — three adapters affected |
| Lint coverage silently reduced by a partition typo | Tampering (of the quality gate) | Assert the union of matrix patterns equals `go list ./...` | QUALITY-06 Pitfall 4 |
| Plaintext secret material in ECS container definitions | Information Disclosure | Existing assertions (`runtime_test.go:139-141, 376-378`) must survive the split byte-for-byte | QUALITY-07 — do not touch `baseContainer`'s `Secrets` mapping |
| Unpinned action introduced by a new matrix job | Supply chain | Reuse the existing SHA pins verbatim | QUALITY-06 |

---

## Assumptions Log

| # | Claim | Section | Risk if wrong |
|---|---|---|---|
| A1 | golangci-lint's `unused` reports only package-local unexported dead code, so directory partitioning cannot introduce false positives | §Q6 | **Highest-impact assumption in this document.** If wrong, the partitioned matrix either emits false positives (noisy, likely `//nolint`-suppressed and thus a real quality loss) or misses real dead code. **Mitigation is cheap and mandatory:** diff the partitioned issue set against one local `./...` run before committing the scheme. |
| A2 | The 30-minute lint failure is memory-driven (runner reaped), not purely golangci-lint's internal timeout | §Q6 | If it is purely the internal timeout, partitioning still helps (smaller graphs finish faster) but the cache-priming job is the more important half. Either way the recommended plan holds; only the emphasis shifts. `##[error]The runner has received a shutdown signal` plus the 8.5 GB knowledge note make memory the stronger reading. |
| A3 | `GOGC` / `GOMEMLIMIT` are community practice, not official golangci-lint guidance | §Q6 | Low. They are listed as last-resort knobs only. I fetched `/docs/product/performance/` (404) and the FAQ (silent on memory), so no official recommendation exists to cite. |
| A4 | The AOSS rule "1, 2, 4, 8, 16, or multiples of 16" reflects a real `CreateCollectionGroup` rejection observed on 2026-07-19 | §Q3 | Moderate. Sole evidence is commit `b8b957e`'s message. AWS's published model contradicts part of it (`min ≥ 0` is documented as valid). The test must therefore lock MageLift's rule with recorded provenance and be marked unverifiable-offline for Phase 3 — not presented as AWS behaviour. |
| A5 | `go test -race ./...` fits in 7 GB on the CI runner | §Pitfall 2, §Open Questions | Moderate. **Never executed in CI.** If it does not fit, criterion 3 needs `go test` partitioning too — a scope increase the plan should pre-budget rather than discover. |
| A6 | The repository's required status check is named only `CI passed` | §Runtime State Inventory | Low but sharp: if branch protection names the `go` job directly, restructuring jobs blocks merges until the rule is edited. One `gh api` call settles it. |
| A7 | AWS preset AZ counts keep `len(AvailabilityZones) ≤ 5`, so the missing AWS carve cap is latent rather than live | §Q5 | Low. Presets are `preview` 2, `standard` 2-3, `high-availability` 3. A user-supplied AZ list is one config field away from tripping it, which is exactly why the guard should be added rather than reasoned about. |
| A8 | The `cmd/magelift` lint partition will complete within 15 minutes once the Go build cache is primed | §Q6 | Moderate. CodeQL extracted the full graph in ~13 min *cold*, which is encouraging, but golangci-lint's analyser facts are additional. Fallback (3) in §Q6 exists for this. |

---

## Open Questions

1. **Q-1: Does `go test -race ./...` complete on a 2 vCPU / 7 GB runner?**
   - What we know: it has never run in CI (lint kills the job first, 38 runs, zero green `go` jobs). Linking all providers peaks at ~8.5 GB; `-race` roughly doubles per-package memory; CodeQL's non-race extraction of the whole graph succeeded in ~13 min.
   - What's unclear: whether `go test -race ./...` peaks above 7 GB.
   - Recommendation: **prove it locally first**, serially — `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./... -count=1` in a plain Terminal (never from Cursor, per AGENTS.md) — and record wall time and peak RSS. Then make the first partitioned-lint PR also the first PR that reaches the `go test` step, and read the log. Pre-budget a contingency task for partitioning `go test` by the same six groups.

2. **Q-2: Should Phase 1 remove `run.concurrency` entirely or set it explicitly?**
   - What we know: v2's default (`0`) matches the container CPU quota. On the private-repo runner that is 2; after publication, 4.
   - What's unclear: whether 2 concurrent analyser threads fit in 7 GB per partition.
   - Recommendation: remove the key (criterion 1 requires it) and add a job-level `timeout-minutes: 15`. If a partition OOMs, set `concurrency: 1` **on that partition only** via `args: --concurrency=1 <packages>` and record why in `docs/lint-policy.md`. Do not restore a global `concurrency: 1`.

3. **Q-3: Is the no-op `AcquireLock` acceptable for this RC?**
   - What we know: `internal/platform/ops.go:18-20` explicitly sanctions it; three adapters use it; TRUST-02 says "never a silent no-op".
   - What's unclear: whether making it fail would break OVH/Scaleway `preview`/`apply`, which PROJECT.md lists as working.
   - Recommendation: **this needs a maintainer decision, not a research answer.** Default to a loud tier-named warning in Phase 1 and a hard failure in Phase 6 once KUBE-05 lands real S3-compatible locking. Record the decision either way — leaving it undecided means TRUST-02 is not really closed.

4. **Q-4: Should the AWS/GCP carve caps land in Phase 1 or be deferred?**
   - What we know: QUALITY-05 is worded as a testing requirement, but there is nothing to test until the caps exist.
   - What's unclear: whether adding validation counts as scope creep against a "publishable baseline" phase.
   - Recommendation: land them. They are ~5 lines each, they are reject-before-register validations matching the codebase's existing discipline, and a test without them would assert nothing. Note the scope in the plan so it is a decision, not a surprise.

5. **Q-5: Does the visibility change belong in this phase?**
   - What we know: going public takes `ubuntu-latest` from 2 vCPU / 7 GB to 4 vCPU / 16 GB, a ~2.3× memory improvement that would materially ease QUALITY-06.
   - What's unclear: whether the maintainer wants to flip visibility before the `v1.0.0-rc.1` tag.
   - Recommendation: **do not** make QUALITY-06 depend on it. Verify partitioning under the harder private-repo constraint, and note in `docs/lint-policy.md` that the measurements predate publication so the numbers are read correctly later.

---

## Recommended Task Ordering

### Wave 0 — restore a green baseline (strictly sequential, nothing else may start)

1. **`gofmt -w`** on the three drifted files. One commit, nothing else. Unblocks CI's `gofmt` step and establishes QUALITY-07's immutable baseline.
2. **Confirm the required status check** is named only `CI passed` (`gh api repos/acourtiol/magelift/branches/main/protection` or the repo settings). One command; prevents a merge-blocking surprise.
3. **QUALITY-06 part A — cache-prime job + explicit `actions/cache` save with `if: always()`.** Breaks the self-reinforcing cold-cache loop that guarantees today's failure.
4. **QUALITY-06 part B — partitioned lint matrix**; remove `run.concurrency: 1` and `timeout: 30m`; add job-level `timeout-minutes: 15`; add the union-coverage guard test; diff the partitioned issue set against one local `./...` run (resolves assumption A1).
5. **Gate: a green CI `go` job on a throwaway PR**, including `go test -race ./...`. If `go test` fails on memory, insert the Q-1 contingency here.

Wave 0 is the whole critical path. Everything downstream is verified by CI that does not currently work.

### Wave 1 — fully independent, safe to parallelise (different files, no shared symbols)

| Track | Requirement | Files touched |
|---|---|---|
| A | QUALITY-02 | `internal/cloud/aws/bootstrap/identity_test.go` |
| B | QUALITY-03 | `internal/cloud/aws/search/search{.go,_test.go}`, one `docs/knowledge/` note |
| C | QUALITY-05 (AWS) | `internal/cloud/aws/network/network{.go,_test.go}` |
| D | QUALITY-05 (GCP) | `internal/cloud/gcp/network/network.go` + **new** `network_test.go` |
| E | QUALITY-05 (Scaleway) + QUALITY-08 (subnet cap) | `internal/cloud/scaleway/network/network_test.go`, `internal/cloud/ovh/network/network_test.go` |
| F | QUALITY-08 (node pool) | **new** `internal/cloud/ovh/runtime/runtime_test.go` (+ optional Scaleway/GCP siblings) |
| G | QUALITY-01 | **new** `internal/cli/cost_test.go`, **new** `internal/platform/cost_test.go` |

Seven tracks, zero file overlap. Track F's `RegisterRPC` mock helper is the only thing another track might want later (Phase 6), so land F early if parallelism is limited.

### Wave 2 — sequenced, because they share files

1. **TRUST-02 first** (`internal/cli/ports.go`, `logs.go`, `health.go`, `cost.go`, `lifecycle.go`, plus the two `ops.go` enumeration tests).
2. **TRUST-01 second** (`internal/cli/lifecycle.go`, `stub_module_test.go`). **Must follow TRUST-02** — both edit `lifecycle.go`, and TRUST-01's test needs the `stubExperimentalModule` and `stubPlanned.tier` field that TRUST-02's tier-naming work also wants.
3. **QUALITY-04 stack-level projection matrix** (`internal/cloud/aws/stack/component_test.go`) — independent of TRUST work, can run in parallel with 1-2.

### Wave 3 — last, alone

**QUALITY-07** — the `runtime.go` split. Sequence it last and give it the tree to itself:
- It is the only task whose success criterion is *"the pre-existing tests pass unchanged"*, so any concurrent edit to `internal/cloud/aws/runtime/` destroys the evidence.
- QUALITY-04's runtime-level combination tests (the six uncovered interactions) **must land before** the split, so they are part of the "unchanged" baseline that proves behaviour preservation. This is the one genuine ordering dependency in the phase and the reason QUALITY-07 goes last rather than first.
- One file per commit, with a test-output diff after each (see §Q7).
- Add the file-size guard test in the same wave so the 400-line cap cannot silently regress.

### Dependency summary

```
Wave 0 (sequential, critical path)
   gofmt -> branch-protection check -> cache-prime -> lint matrix -> GREEN go job
                                                                          |
                    +-----------------------------------------------------+
                    |                                                     |
              Wave 1 (7 parallel tracks A-G)              Wave 2 (TRUST-02 -> TRUST-01; Q4-stack ||)
                    |                                                     |
                    +----------------------+------------------------------+
                                           |
                                    Q4 runtime-level tests
                                           |
                                  Wave 3: QUALITY-07 split (alone)
```

**Do not parallelise:** Wave 0 internally; TRUST-02 before TRUST-01; QUALITY-04's runtime tests before QUALITY-07.
**Safe to parallelise:** all of Wave 1; QUALITY-04's stack-level matrix against Wave 2.

---

## ROADMAP Criteria — Feasibility Verdicts (summary)

| # | Criterion | Verdict | Required amendment |
|---|---|---|---|
| 1 | Partitioned lint jobs green; `concurrency: 1` and 30m timeout removed | **Achievable, under-specified** | Say "one per provider **plus core and aggregate**"; replace "no longer needs the 30-minute timeout" with a falsifiable job-level `timeout-minutes: 15`; **add "the whole `go` job green on a PR"** — it never has been |
| 2 | No file under `internal/cloud/aws/runtime/` > 400 lines; `runtime_test.go` passes unchanged | **Achievable, one wording defect** | Scope the cap to **non-test** files (`runtime_test.go` is 586 lines and would fail its own criterion); note the gofmt'd file is the baseline |
| 3 | `go test -race ./...` guards, incl. "subnet index at cap and cap+1 for **every provider**" | **Partly unachievable as written** | Scaleway carves no subnets — replace with a single-range negative assertion. AWS and GCP have **no cap to test** — the criterion implies code changes it does not name. Also: `go test -race ./...` has never run in CI |
| 4 | Mutating command against `gcp`/`ovh`/`scaleway` warns before Pulumi | **Achievable, too narrow** | Gate on `CertificationTier() == TierExperimental`, not a provider allowlist — `aws/eks-autopilot` is experimental too and would slip through |
| 5 | Unimplemented day-2 exits non-zero naming capability + tier; test enumerates 15 sites each, no nil-success path | **Achievable; counts verified exact** | Satisfiable while leaving all three real silent-success paths intact. Extend to name `Ops.AcquireLock`'s no-op release (3 adapters) and `deploy`'s silent infra-only fallback (`lifecycle.go:176-180`) |

---

## Sources

### Primary (HIGH confidence — verified in this session)

**Codebase, read directly**
- `internal/cloud/aws/runtime/runtime.go` (991 lines, full read) and `runtime_test.go` (586 lines, full read)
- `internal/cli/{root.go, lifecycle.go, cost.go, ports.go, exec.go, env.go, root_test.go, stub_module_test.go}`
- `internal/platform/{module.go, ops.go, cost.go}`
- `internal/cloud/{ovh,scaleway}/stack/ops.go`; `internal/cloud/ovh/runtime/runtime.go`
- `internal/cloud/{aws,gcp,ovh,scaleway}/network/network.go` + existing test files
- `internal/cloud/aws/search/search.go`; `internal/cloud/aws/bootstrap/{identity.go, identity_test.go, policy.go}`
- `internal/cloud/aws/stack/{spec.go, config.go, component.go}` + test files
- `cmd/magelift/main.go`; `.golangci.yml`; `.github/workflows/{ci.yml, codeql.yml}`; `Makefile`; `go.mod`; `AGENTS.md`; `docs/{lint-policy.md, capability-matrix.md}`
- `docs/knowledge/lessons/MageLift gendocs must not link Pulumi cloud SDKs.md` — **the ~8.5 GB peak-RSS figure**
- `docs/knowledge/reference/Build and release on 16GB - use serial goreleaser to avoid OOM.md`

**Live CI evidence (`gh`)**
- Run `30221546264` `go` job log — `golangci-lint run ./...` 21:42:12 → runner shutdown 22:12:42; `Cache is not found`; golangci-lint cache miss
- Run `30243793821` CodeQL — full Go autobuild succeeded in ~13 min; failure was a config error
- `gh api …/actions/workflows/ci.yml/runs` → `total_count: 38`; per-run scan → **no green `go` job ever**
- `gh repo view` → `isPrivate: true`
- `git show` of `8c3a4c6`, `b8b957e`, `37081b7`, `780a969`, `3a0ec0f`, `a8173af`, `91b8ecb`, `b5deb3d`; `git show HEAD:internal/cli/cost_test.go`
- `gofmt -l` → three drifted files; `gofmt -d internal/cloud/aws/runtime/runtime_test.go`

**Pinned SDK source (module cache, v3.253.0)**
- `go/pulumi/mocks.go:92-110, 326` — `MockResourceArgs.RegisterRPC`
- `go/pulumi/resource.go:855-864` — `DependsOn` accumulates via `append`
- `proto/go/provider.pb.go:2827` — `Dependencies []string` field 15

### Secondary (MEDIUM-HIGH — official documentation)

- https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html — whitespace not counted; role trust policy 2,048 → 8,192
- https://docs.aws.amazon.com/IAM/latest/APIReference/API_CreatePolicy.html — "including whitespace … with no whitespaces"; 131,072 wire limit
- https://docs.aws.amazon.com/IAM/latest/APIReference/API_PutRolePolicy.html
- https://repost.aws/knowledge-center/iam-increase-policy-size — 6,144 managed / 10,240 inline-role
- https://docs.aws.amazon.com/opensearch-service/latest/ServerlessAPIReference/API_CollectionGroupCapacityLimits.html — min ≥ 0, max ≥ 1, **no step documented**
- https://docs.aws.amazon.com/opensearch-service/latest/ServerlessAPIReference/API_CreateCollectionGroup.html
- https://docs.aws.amazon.com/opensearch-service/latest/developerguide/serverless-scaling.html — account-level: "any number from 2 … in multiples of 2"; max 1,700
- https://docs.aws.amazon.com/opensearch-service/latest/developerguide/serverless-collection-groups.html
- https://golangci-lint.run/docs/configuration/file/ — `run.concurrency` default 0; `run.timeout` default 0 (disabled)
- https://golangci-lint.run/docs/welcome/faq/ — confirmed **silent** on memory/OOM/GOGC
- https://github.com/golangci/golangci-lint-action (v9.3.0 README) — inputs, cache key, `skip-save-cache`
- https://docs.github.com/en/actions/reference/runners/github-hosted-runners — public `ubuntu-latest` 4 vCPU / 16 GB; larger runners require Team/Enterprise Cloud

### Tertiary (LOW — community, flagged in-line)

- `golangci/golangci-lint` issues #5031, #3582, #3565, #5449 — memory reports, `GOGC`/concurrency practice
- `golangci/golangci-lint` #2771, #4218; `dominikh/go-tools` #671 — `unused` whole-program caching false positives (basis of assumption A1)
- https://github.blog/news-insights/product-news/github-hosted-runners-double-the-power-for-open-source/ — private 2 vCPU / 7 GB vs public 4 vCPU / 16 GB
- `golangci-lint.run/docs/product/performance/` — **404, does not exist**

---

## Metadata

**Confidence breakdown:**
- Codebase facts (file/line/symbol claims): **HIGH** — every claim read directly; four CONCERNS.md errors corrected
- QUALITY-06 root cause: **HIGH** — live CI logs plus the repo's own 8.5 GB knowledge note
- QUALITY-06 partition *safety* for `unused`: **MEDIUM** — reasoned from Go visibility; assumption A1 with a named cheap mitigation
- AWS IAM limits: **HIGH** — official docs, three pages cross-checked
- AOSS OCU rule: **LOW** — **AWS does not document the rule MageLift enforces**, and the published model contradicts part of the commit message
- GitHub runner specs and larger-runner availability: **HIGH** — official docs plus `gh repo view`
- QUALITY-07 split safety: **HIGH** — verified every identifier `runtime_test.go` touches is package-scoped
- QUALITY-04 coverage inventory: **HIGH** — all 586 test lines read; six gaps traced to specific production lines
- QUALITY-05 per-provider carve semantics: **HIGH** — all four implementations read
- TRUST-01 choke point: **HIGH** — all 18 `planStack` call sites read, pre-backend ordering verified
- TRUST-02 counts and silent-success paths: **HIGH** — grep counts plus reading each handler

**Research date:** 2026-07-27
**Valid until:** 2026-08-10 (14 days). Shorter than the usual 30 because CI is actively red and the visibility change would invalidate the QUALITY-06 measurements. **Re-verify before planning if:** the repo goes public, `golangci-lint` is bumped, or any CI run changes the `go` job's outcome.
