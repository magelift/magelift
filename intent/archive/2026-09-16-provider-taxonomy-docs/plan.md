---
status: done
slug: provider-taxonomy-docs
spec: spec.md
---

# Plan: provider taxonomy and docs

## Files that change

### Move (via `git mv`, package names unchanged — only import paths change)

| From | To | Files |
| --- | --- | --- |
| `internal/cloud/recovery/` | `internal/shared/recovery/` | 10 (`object_archive.go`, `object_archive_test.go`, `object_engine.go`, `operation.go`, `projection.go`, `projection_test.go`, `s3_object_store.go`, `s3_object_store_test.go`, `secret_archive.go`, `secret_archive_test.go`; all `package recovery`) |
| `internal/cloud/resilience/` | `internal/shared/resilience/` | 6 (`adapter.go`, `adapter_test.go`, `operations.go`, `operations_test.go`, `projection.go`, `projection_test.go`; `package resilience` / `resilience_test`) |
| `internal/cloud/statearchive/` | `internal/shared/statearchive/` | 2 (`archive.go`, `archive_test.go`; `package statearchive`) |

`internal/shared/` does not exist yet; the moves create it. Doc comments in the moved dirs were checked: none name the old import path (only `import` lines do), so the rewrite is import-lines-only; if any path-naming comment surfaces during the move, update it in place. Keep existing local import aliases (`cloudrecovery`, `cloudresilience`) unchanged — 44 files use those identifiers at hundreds of use-sites, and renaming them is out-of-scope churn; the spec requires import-path-only.

### Delete (via `git rm`)

- `cmd/magelift-aws/main.go` (registers `awsops.Module` only; `package main`, no importers)
- `cmd/magelift-gcp/main.go` (registers `gcpops.Module` Autopilot + Standard; `package main`, no importers)
- `cmd/magelift-ovh/main.go` (registers `ovhstack.Module` only; `package main`, no importers)
- `cmd/magelift-scaleway/main.go` (registers `scwstack.Module` only; `package main`, no importers)

Verified: absent from `.goreleaser.yaml` (only `./cmd/magelift` and `./cmd/magelift-provider-gcp` build), zero Go importers, zero Makefile/CI refs.

### Edit — script (1)

- `scripts/gcp-acceptance-local.sh:699` — `./cmd/magelift-gcp` → `./cmd/magelift`; keep serial flags and `MAGELIFT_GCP_ACCEPTANCE_BIN` resume override. Accept the heavier full-CLI link as the validated cost of deletion.

### Edit — docs + skills (6)

- `docs/adding-a-provider.md` — new § Provider roots rule table (spec verbatim); extend opening paragraph (lines 3–6); SaaS-homes + adapter-less subsections under step 6 (lines 36–62); kube-exception pointer to ADR 0003.
- `docs/adr/0003-portable-contracts-vs-topology.md` — extend § Decision (lines 10–18) with `internal/external/` + `internal/shared/` + adapter-less rule and kube carve-out paragraph (spec verbatim); extend § Consequences (lines 20–22, recertifies nothing).
- `AGENTS.md` — Map (lines 68–81): replace single `internal/cloud/<p>/` row (line 75) with the three-row version (spec verbatim); leave the Never rule (lines 23–24) intact.
- `docs/gcp-acceptance.md:326-328` — slim-main wording swap (spec verbatim direction); drop the memory-bounded slim-link claim.
- `docs/capability-matrix.md` — Cross-cutting edge and observability § (lines 327–344) home/adapter-less annotations only (Fastly row 335, Cloudflare row 340, SES row 341, New Relic row 344). No tier-word changes (certified rows at lines 33, 35).
- `contrib/skills/magelift-provider/SKILL.md` — Boundary § (lines 14–19) + Leave-behind (lines 54–58) mirror the rule; no checklist renumbering.

### Import-path rewrites in 50 Go files (32 non-test + 18 test), grouped by package

- `internal/cloud/scaleway/resilience/` — 11 files
- `internal/cloud/ovh/resilience/` — 10 files
- `internal/cloud/gcp/resilience/` — 10 files
- `internal/cloud/aws/resilience/` — 10 files
- `internal/cloud/resilience/` — 4 files (inside the moved tree; moves with it, self-imports rewritten)
- `internal/cloud/kube/` — 2 files (`projection.go`, `projection_test.go`; kube itself stays)
- `internal/cloud/gcp/state/` — 1 file (`lock.go`, statearchive path)
- `internal/cloud/aws/state/` — 1 file (`archive.go`, statearchive path)
- `cmd/gcp-secret-recovery-acceptance/` — 1 file (`main.go`)

Per-path split: `recovery` 33 files, `resilience` 27 files, `statearchive` 2 files (some files import two paths).

### Generated (only via `make generate`, never hand-edit)

- `schema/`, CLI reference, certification coverage, `agents/manifest.json` — only if drift appears after the rename.

### Explicitly NOT touched

- `docs/versioning.md:47-54` (kube path unchanged, shared-kube reservation still accurate)
- `docs/evidence/README.md` (no tier change)
- `docs/architecture.md:40-41` (generic rule already correct; optional one-line pointer only if the humanizer pass finds it ambiguous)
- `website/` (verified: `grep -r "internal/cloud" website/` returns empty)
- `contrib/skills/magelift-contribute` (verified: zero `internal/cloud|internal/external|internal/edge` refs)
- `agents/skills/` shipped user skills (contributor-facing taxonomy; no YAML contract change)

## Order of work

- [x] 1.1 `git mv` the three dirs to `internal/shared/` (18 files; no content edits in this step) — verify: `ls internal/shared/recovery internal/shared/resilience internal/shared/statearchive` exits 0
- [x] 1.2 Rewrite import paths `internal/cloud/recovery|resilience|statearchive` → `internal/shared/...` across the 50 Go files with `goimports -w` (or `gofmt -r`); keep local aliases (`cloudrecovery`, `cloudresilience`) unchanged — verify: `grep -r "internal/cloud/recovery" --include='*.go' .` returns empty AND `grep -r "internal/cloud/resilience" --include='*.go' .` returns empty AND `grep -r "internal/cloud/statearchive" --include='*.go' .` returns empty
- [x] 1.3 Sweep the moved dirs for doc comments naming the old path (none expected — verified import-lines-only); update any found in place — verify: `grep -rn 'internal/cloud/' internal/shared/recovery/ internal/shared/resilience/ internal/shared/statearchive/` returns empty
- [x] 1.4 Run the focused shared suites in serial form — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/shared/recovery/ ./internal/shared/resilience/ ./internal/shared/statearchive/ -count=1` exits 0
- [x] 1.5 Run `make generate` (drift refresh only, no hand-edits to generated files) plus the config suite — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/config/ -count=1` exits 0
- [x] 2.1 `git rm` the four slim mains (`cmd/magelift-{aws,gcp,ovh,scaleway}/main.go`) — verify: `ls cmd/ | grep "magelift-aws"` returns empty AND `ls cmd/ | grep "magelift-gcp"` returns empty AND `ls cmd/ | grep "magelift-ovh"` returns empty AND `ls cmd/ | grep "magelift-scaleway"` returns empty
- [x] 2.2 Retarget `scripts/gcp-acceptance-local.sh:699` to build `./cmd/magelift`; keep `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB`, `-trimpath -ldflags='-s -w'`, and the `MAGELIFT_GCP_ACCEPTANCE_BIN`/guard at 697–702 (heavier link accepted; resume via prebuilt bin) — verify: `grep -F "./cmd/magelift" scripts/gcp-acceptance-local.sh` exits 0
- [x] 2.3 Sweep for dangling slim-main refs in live code (exclude `intent/`, whose spec text legitimately names the deleted paths) — verify: `grep -r "cmd/magelift-gcp" --include='*.sh' --include='*.md' --include='Makefile' --include='*.yml' --include='*.go' . | grep -v '^./intent/'` returns empty AND `grep -r "cmd/magelift-aws" --include='*.sh' --include='*.md' --include='Makefile' --include='*.yml' --include='*.go' . | grep -v '^./intent/'` returns empty AND `grep -r "cmd/magelift-ovh" --include='*.sh' --include='*.md' --include='Makefile' --include='*.yml' --include='*.go' . | grep -v '^./intent/'` returns empty AND `grep -r "cmd/magelift-scaleway" --include='*.sh' --include='*.md' --include='Makefile' --include='*.yml' --include='*.go' . | grep -v '^./intent/'` returns empty
- [x] 3.1 Edit `docs/adding-a-provider.md`: add § Provider roots rule table (spec verbatim), extend the opening paragraph to name `internal/external/`, `internal/shared/`, `internal/edge/waf`, and both adapter-less cases, add the kube-exception pointer, and add SaaS-homes + adapter-less subsections under step 6 — verify: `grep -F "internal/cloud/<provider>" docs/adding-a-provider.md` exits 0 AND `grep -F "internal/external/" docs/adding-a-provider.md` exits 0 AND `grep -F "internal/external/fastly" docs/adding-a-provider.md` exits 0 AND `grep -F "internal/external/newrelic" docs/adding-a-provider.md` exits 0 AND `grep -F "internal/external/observability" docs/adding-a-provider.md` exits 0 AND `grep -F "internal/edge/waf" docs/adding-a-provider.md` exits 0 AND `grep -Fw "SES" docs/adding-a-provider.md | grep -i "adapter-less"` exits 0 AND `grep -F "Cloudflare" docs/adding-a-provider.md | grep -i "adapter-less"` exits 0 AND `grep -F "email.mode" docs/adding-a-provider.md` exits 0 AND `grep -F "scripts/acceptance/lib-cloudflare-dns.sh" docs/adding-a-provider.md` exits 0
- [x] 3.2 Edit `docs/adr/0003-portable-contracts-vs-topology.md`: extend § Decision with the rule + kube carve-out paragraph (spec verbatim), extend § Consequences (move recertifies nothing) — verify: `grep -F "internal/cloud/<provider>" docs/adr/0003-portable-contracts-vs-topology.md` exits 0 AND `grep -F "internal/external/" docs/adr/0003-portable-contracts-vs-topology.md` exits 0 AND `grep -F "internal/cloud/kube" docs/adr/0003-portable-contracts-vs-topology.md` exits 0 AND `grep -Fi "carve-out" docs/adr/0003-portable-contracts-vs-topology.md` exits 0 AND `grep -F "pulumi-kubernetes" docs/adr/0003-portable-contracts-vs-topology.md` exits 0
- [x] 3.3 Edit `AGENTS.md` Map: replace the single `internal/cloud/<p>/` row with the three-row version (spec verbatim); Never rule untouched — verify: `grep -F "internal/cloud/<p>" AGENTS.md` exits 0 AND `grep -F "internal/external/" AGENTS.md` exits 0
- [x] 3.4 Reword `docs/gcp-acceptance.md:326-328` to the shipped-CLI + `MAGELIFT_GCP_ACCEPTANCE_BIN` resume text; drop the slim-link memory claim — verify: `grep -F "cmd/magelift-gcp" docs/gcp-acceptance.md` returns empty
- [x] 3.5 Annotate `docs/capability-matrix.md` Cross-cutting § (lines 327–344) with homes/adapter-less notes only; touch no tier word — verify: `grep -F "certified for the evidenced preview tuple" docs/capability-matrix.md` exits 0 AND `grep -F "certified for evidenced runtime cells" docs/capability-matrix.md` exits 0 AND `git diff -- docs/capability-matrix.md | grep -E "^[-+].*\bcertified\b"` returns empty unless the matching `+`/`-` pair is a context-neutral reword with identical tier meaning recorded in the PR description
- [x] 3.6 Edit `contrib/skills/magelift-provider/SKILL.md` Boundary § + Leave-behind to mirror the rule (SaaS → `internal/external/<vendor>/`, ports → `internal/shared/<port>/`, kube carve-out, adapter-less SES + Cloudflare); no checklist renumbering — verify: `grep -F "internal/shared/<port>" contrib/skills/magelift-provider/SKILL.md` exits 0 AND `grep -F "internal/external/<vendor>" contrib/skills/magelift-provider/SKILL.md` exits 0
- [x] 3.7 Run the `humanizer` skill, then `remove-ai-marks`, on `docs/adding-a-provider.md` — verify: both passes completed on `docs/adding-a-provider.md` and the step 3.1 greps still exit 0
- [x] 3.8 Run the `humanizer` skill, then `remove-ai-marks`, on `docs/adr/0003-portable-contracts-vs-topology.md` — verify: both passes completed on `docs/adr/0003-portable-contracts-vs-topology.md` and the step 3.2 greps still exit 0
- [x] 3.9 Run the `humanizer` skill, then `remove-ai-marks`, on `docs/gcp-acceptance.md` — verify: both passes completed on `docs/gcp-acceptance.md` and the step 3.4 grep still returns empty
- [x] 3.10 Run the `humanizer` skill, then `remove-ai-marks`, on the `docs/capability-matrix.md` annotations — verify: both passes completed on `docs/capability-matrix.md` and the step 3.5 tier-word greps still exit 0 with the tier diff still empty
- [x] 3.11 Run the `humanizer` skill, then `remove-ai-marks`, on the `AGENTS.md` Map rows — verify: both passes completed on `AGENTS.md` and the step 3.3 greps still exit 0
- [x] 3.12 Confirm no Go adapter was created for email vendors or Cloudflare — verify: `ls internal/external/ses internal/external/cloudflare internal/cloudflare 2>&1` reports no such directories AND `grep -rl "package cloudflare" --include='*.go' internal/ cmd/` returns empty
- [x] 4.1 Run the generated-drift gates — verify: `make generate-check` exits 0 AND `go run ./cmd/gendocs --check` exits 0
- [x] 4.2 Run the strict docs gate — verify: `make docs` exits 0
- [x] 4.3 Re-grep website refs and tier words (catches scope drift since research) — verify: `grep -r "internal/cloud" website/` returns empty AND `grep -F "certified for the evidenced preview tuple" docs/capability-matrix.md` exits 0 AND `grep -F "certified for evidenced runtime cells" docs/capability-matrix.md` exits 0
- [x] 4.4 Confirm the working tree shows only intended paths (3 moved dirs, 4 deleted mains, 1 script, 6 doc/skill edits, possible `make generate` outputs) — verify: `git status --porcelain` shows only intended paths

Serial-build discipline for every step above: `GOMAXPROCS=1 GOFLAGS=-p=1` (Makefile also exports `GOMEMLIMIT=1GiB`); never unbounded `go test -race ./...` or parallel heavy `go build` from an IDE session (`magelift-serial-builds`).

## Risks

- **Import-churn missed file** — 50 Go files across 9 packages is easy to under-rewrite by hand; mitigate by doing the rewrite with `goimports -w`/`gofmt -r` over the whole tree, then the three empty-grep checks in step 1.2 plus a serial `go build ./...`-equivalent via the focused suites before claiming done.
- **Generated drift** — the rename can ripple into `schema/`, CLI reference, certification coverage, or `agents/manifest.json`; mitigate with `make generate` (step 1.5, never hand-edits) and the `make generate-check` + `go run ./cmd/gendocs --check` gates (step 4.1).
- **Slim-main resume-workflow slowdown** — the acceptance script now links the full CLI (heavier, slower) instead of the GCP-only binary; accepted cost of the validated delete decision, mitigated by the kept `MAGELIFT_GCP_ACCEPTANCE_BIN` resume override and the reworded resume note in `docs/gcp-acceptance.md`.
- **Tier-word accidental edit** — capability-matrix annotations sit next to certified/experimental cells; mitigate with the step 3.5/4.3 tier-word greps and the `git diff | grep -E "^[-+].*\bcertified\b"` emptiness check; PR description must state "no tier change".
- **Humanizer skipped** — all five human pages (adding-a-provider, ADR 0003, gcp-acceptance, capability-matrix, AGENTS Map) require humanizer then remove-ai-marks per repo rules; steps 3.7–3.11 make each page explicit with re-grep verifies.
- **Website ref missed** — research found zero `internal/cloud` refs in `website/`, but the tree may have moved; step 4.3 re-greps, and any hit enters scope with humanizer + remove-ai-marks.
- **Stale `cloud*` aliases left deliberately** — `cloudrecovery`/`cloudresilience` aliases now point at `internal/shared/...`; this is intentional (renaming 44 files of use-sites is out-of-scope churn), not a missed rewrite; do not "fix" them in this change.
- **Spec-grep self-hits** — scenario greps for `cmd/magelift-{aws,gcp,ovh,scaleway}` still match `intent/provider-taxonomy-docs/spec.md` itself (historical record); scope emptiness checks to exclude `intent/` (step 2.3). Old-path strings also linger in the `dist/` build-artifact binary; it regenerates on next build and is outside the `--include='*.go'` scenario scopes.

## Proof

```sh
# Rule text present in all three docs
grep -F "internal/cloud/<provider>" docs/adding-a-provider.md
grep -F "internal/external/" docs/adding-a-provider.md
grep -F "internal/cloud/<provider>" docs/adr/0003-portable-contracts-vs-topology.md
grep -F "internal/external/" docs/adr/0003-portable-contracts-vs-topology.md
grep -F "internal/cloud/<p>" AGENTS.md
grep -F "internal/external/" AGENTS.md

# Old import paths gone from Go sources
grep -r "internal/cloud/recovery" --include='*.go' .       # expect empty
grep -r "internal/cloud/resilience" --include='*.go' .     # expect empty
grep -r "internal/cloud/statearchive" --include='*.go' .   # expect empty

# New shared root exists and builds
ls internal/shared/recovery internal/shared/resilience internal/shared/statearchive
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/shared/recovery/ ./internal/shared/resilience/ ./internal/shared/statearchive/ -count=1

# SaaS homes greppable in the provider guide
grep -F "internal/external/fastly" docs/adding-a-provider.md
grep -F "internal/external/newrelic" docs/adding-a-provider.md
grep -F "internal/external/observability" docs/adding-a-provider.md
grep -F "internal/edge/waf" docs/adding-a-provider.md

# Adapter-less labels with reasons present
grep -Fw "SES" docs/adding-a-provider.md | grep -i "adapter-less"
grep -F "Cloudflare" docs/adding-a-provider.md | grep -i "adapter-less"
grep -F "email.mode" docs/adding-a-provider.md
grep -F "scripts/acceptance/lib-cloudflare-dns.sh" docs/adding-a-provider.md

# No Go adapter exists for email vendors or Cloudflare
ls internal/external/ses internal/external/cloudflare internal/cloudflare 2>&1  # expect no such directories
grep -rl "package cloudflare" --include='*.go' internal/ cmd/  # expect empty

# Carve-out wording present in ADR 0003
grep -F "internal/cloud/kube" docs/adr/0003-portable-contracts-vs-topology.md
grep -Fi "carve-out" docs/adr/0003-portable-contracts-vs-topology.md
grep -F "pulumi-kubernetes" docs/adr/0003-portable-contracts-vs-topology.md

# Slim mains gone from the tree
ls cmd/ | grep "magelift-aws"       # expect empty
ls cmd/ | grep "magelift-gcp"       # expect empty
ls cmd/ | grep "magelift-ovh"       # expect empty
ls cmd/ | grep "magelift-scaleway"  # expect empty

# No dangling slim-main references (intent/ excluded: spec text legitimately names them)
grep -r "cmd/magelift-gcp" --include='*.sh' --include='*.md' --include='Makefile' --include='*.yml' --include='*.go' . | grep -v '^./intent/'       # expect empty
grep -r "cmd/magelift-aws" --include='*.sh' --include='*.md' --include='Makefile' --include='*.yml' --include='*.go' . | grep -v '^./intent/'       # expect empty
grep -r "cmd/magelift-ovh" --include='*.sh' --include='*.md' --include='Makefile' --include='*.yml' --include='*.go' . | grep -v '^./intent/'       # expect empty
grep -r "cmd/magelift-scaleway" --include='*.sh' --include='*.md' --include='Makefile' --include='*.yml' --include='*.go' . | grep -v '^./intent/'  # expect empty

# Acceptance script builds the shipped CLI
grep -F "./cmd/magelift" scripts/gcp-acceptance-local.sh
grep -F "cmd/magelift-gcp" docs/gcp-acceptance.md  # expect empty

# Focused unit suites stay green
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/shared/recovery/ ./internal/shared/resilience/ ./internal/shared/statearchive/ -count=1
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/config/ -count=1

# Generated drift gates stay green
make generate-check
go run ./cmd/gendocs --check

# Docs build stays strict-green
make docs

# Certified rows unchanged
grep -F "certified for the evidenced preview tuple" docs/capability-matrix.md
grep -F "certified for evidenced runtime cells" docs/capability-matrix.md
git diff -- docs/capability-matrix.md | grep -E "^[-+].*\bcertified\b"  # expect empty

# Website still clean
grep -r "internal/cloud" website/  # expect empty

# Only intended paths changed
git status --porcelain
```
