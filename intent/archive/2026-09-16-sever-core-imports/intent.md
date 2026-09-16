---
status: done
slug: sever-core-imports
---

# Intent: sever core to provider imports

## Problem

The generic CLI entrypoint and internal CLI packages import provider packages directly, so the core cannot compile, link, or unit-test without every cloud present. Contributors pay the full Pulumi dependency union for any CLI change, `internal/cli` carries cloud SDK reachability it must not have, and provider lifecycle independence is impossible while core hardcodes provider hooks.

## Evidence

- `cli/cli.go` imports `internal/cloud/aws/secrets`, `internal/cloud/gcp/naming`, `internal/cloud/gcp/resilience`, `internal/cloud/gcp/secrets`, `internal/cloud/gcp/stack` for Composer-secret and Cloud SQL cleanup hooks.
- `internal/cli/cleanup.go` imports `internal/cloud/gcp/resilience`, `internal/cloud/ovh/resilience`, `internal/cloud/scaleway/resilience`.
- `internal/cli/env.go` imports `internal/cloud/aws/endpoint`.
- `internal/registry/registry.go` `NewDefault()` registers all six StackModules (aws, awseks, gcp autopilot, gcp standard, ovh, scaleway) in-process for the released CLI.
- `go list -m all` reports 448 modules; `internal/cloud/aws` ~35k LOC and `internal/cloud/gcp` ~21k LOC ride in every `cmd/magelift` link.
- `docs/adding-a-provider.md` requires keeping Pulumi SDKs out of `internal/cli` so `gendocs` and CLI unit tests stay linkable on small runners.

## Proposed outcome

Non-test Go code under `cli/` and `internal/cli/` imports zero `internal/cloud/*` packages. Provider-specific hooks (Composer secrets, leftover-backup destroy, cleanup providers, endpoint resolution) resolve through `platform` ports or explicit registration from provider packages. `magelift` behavior is unchanged; `make verify` and the offline gates stay green with a smaller link surface for CLI-only changes.

## Affected users and systems

CLI contributors; `cli/`, `internal/cli`, `internal/platform`, `internal/registry`, `internal/cloud/*` hook owners; `examples/custom-cli`; Floci suites that load modules in-process; `cmd/gendocs` linkability.

## Constraints

- Smallest correct change; no CLI behavior change; no new provider or catalog cell.
- Keep the v1 single-version promise (`v1-stable-cut`): this is import hygiene, not independent provider releases.
- Wire modules through `platform.ModuleRegistry`; `infra.RegisterTarget` alone does not ship.
- No Pulumi SDK reachability from `internal/cli`; keep `gendocs` linkable on small runners.
- `make verify` green; narrow loops with `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/<pkg>/ -count=1`.
- If a public contract or topology rule changes, update the ADR and the human page together (humanizer, then remove-ai-marks).

## Out of scope

- Multi-module split, `go.work`, per-provider `go.mod` or tags.
- Subprocess extraction beyond the existing GCP proof cell; day-2 over RPC.
- `internal/config` provider-struct decoupling; provider taxonomy moves or renames.
- SDK module extraction; Cloudflare/SES adapter work.

## Open questions

- Hook registration shape: explicit `RegisterHooks` called from `cmd/magelift`, provider `init`, or pure `platform` port interfaces with no registration call? Carried into spec with a default.
- Which port shapes cover cleanup (`leftover-backup destroy`, `CleanupProvider`) and env (`aws/endpoint`) without leaking SDK types?
- Does `docs/adding-a-provider.md` need a seam rule update in the same change, or is the existing "keep Pulumi SDKs out of `internal/cli`" wording already sufficient?
