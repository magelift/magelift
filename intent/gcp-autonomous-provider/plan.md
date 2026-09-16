---
status: planned
slug: gcp-autonomous-provider
spec: spec.md
---

# Plan: autonomous GCP provider (the first real plugin)

## Files that change

Exact paths. New vs edit. One line each on what changes.

### New module scaffold

- `providers/gcp/go.mod` (new): module `github.com/magelift/magelift/providers/gcp`, go toolchain pins, requires root plus sdk plus GCP cloud deps (no other providers).
- `providers/gcp/go.sum` (new): committed tidy output.
- `providers/gcp/Makefile` (new): provider-owned harness targets (floci plus acceptance), same UX names as root forwards.
- `providers/gcp/cmd/magelift-provider-gcp/main.go` (new): net/rpc plugin main (handshake, serve, versioned API).
- `providers/gcp/auth/` (new): token-source auth package plus expiry/refresh/restart/CI tests.

### SDK protocol (new files in `sdk/`)

- `sdk/gcp_protocol.go` (new): versioned typed operations (handshake, lifecycle, day-2, cost inputs), typed errors, opaque state.
- `sdk/gcp_protocol_test.go` (new): version constants, validation, error taxonomy tests.

### Moves (git mv, then rewire)

- `internal/cloud/gcp/*` → `providers/gcp/*` (all 95 Go files incl. tests), EXCEPT `queue/` and `search/`, which move to `internal/cloud/kube/queue/` and `internal/cloud/kube/search/` as shared k8s workload components.
- `cmd/gcp-*-acceptance/*` (8 dirs) → `providers/gcp/cmd/*`.
- `scripts/gcp-*.sh` (9) plus `scripts/floci-gcp-test.sh` → `providers/gcp/scripts/*` with internal path fixes.
- `tests/floci-gcp/*` → `providers/gcp/floci/*` with import fixes.
- `cmd/magelift-provider-gcp/*` (delete): replaced by the new plugin main.

### Vendored copies (new files under `providers/gcp/`, fork notes citing sources)

- `providers/gcp/nrdot/` (new): NRDOT helm subset vendored from `external/newrelic/nrdot.go` (plus minimal decls if split across files) with a fork note; its `kube` plus `sdk` imports stay (both allowed).

### Core rewiring (edits)

- `internal/registry/registry.go` (edit): drop the two `gcpops` modules.
- `internal/registry/hooks.go` (edit): drop GCP provider hook bodies (served over the protocol now).
- `internal/cli/subprocess.go` (edit): GCP discovery plus verify plus dial plus negotiate with fail-closed errors; fallback deleted.
- `internal/cli/subprocess_test.go` (edit): v2 fail-closed asserts (checksum, incompatibility, dial failure, tamper).
- `internal/providerhost/` (mixed): keep `verify.go` plus host discovery entry; bump `lock.go` to schema v2 with required `protocol` marker (v1 refused); add net/rpc v2 client plus negotiation (`netrpc.go`, `negotiate.go`) with a plugin-backed module shim for registry dispatch; add v2 tests; delete JSON op path (`execute.go`, `grpc.go`, `subprocess_backend.go`, `hostproto/`, v1-only tests) once unreferenced.
- `internal/cleanup/plugin.go` (new): plugin-backed Inventory plus Delete over protocol ops, replacing `internal/cleanup/gcp.go` (delete).
- `internal/config/model.go` (edit): `target.gcp` becomes raw (presence-only) for provider-side schema.
- `internal/config/config.go` (edit): slim GCP case (presence plus generic single-target-block rule); per-provider GCP semantics move out.
- `schema/magelift.schema.json` (regenerate via generator only).
- `internal/config/*_test.go` (edit as needed): expectations matching the slimmed core.
- `internal/cloud/aws/eks/runtime.go` (edit): `kubequeue`/`kubesearch` import path updates only.
- `Makefile` (edit): gcp harness targets forward to `providers/gcp` (`$(MAKE) -C`), plus `provider-gcp-build` and `core-leanness` gate targets.
- `internal/cleanup/gcp.go` (edit or move): GCP sweep logic moves behind the protocol Teardown/ledger op; core keeps ledger plus dispatch (exact split at implementation; plan updated if the file moves).

### Follower updates (edits)

- `tests/synthetic/suite_test.go` (edit): GCP registry-routing cases become fake-module routing; missing-required expectation moves (core passes, provider asserts); expired-preview plan case moves to the provider suite.
- `internal/cloud/kube/identity_test.go` (edit): GCP provider case adapts to plugin-or-absent (no in-process gcpops).
- `internal/platform/ops_test.go`, `internal/shared/resilience/adapter_test.go` (edit if red): GCP references updated or removed.
- `internal/cleanup/*_test.go` (edit if red): ledger tests keep passing with provider-side sweep.

### Docs (edits)

- `docs/capability-matrix.md` (edit): GCP rows name the `providers/gcp` home.
- `docs/adding-a-provider.md` (edit): plugin path describes the built provider (drop until-Order-5 caveats).
- `docs/architecture.md` (edit): same tense fixes for provider homes.
- `docs/gcp-acceptance.md` (edit): harness paths point at `providers/gcp`.
- `docs/gcp-experimental.md` (edit): homes updated (content checked first).
- `docs/lint-policy.md` (edit only if its `internal/cloud/gcp` ref goes stale).
- `contrib/skills/magelift-provider/SKILL.md` (edit only on contradiction).

## Order of work

Build and verify order, not a task dump. Group by area, number within the group.
Each box carries the check that closes it.

- [x] 1.1 Add the SDK protocol types plus wire values with standalone tests — verify: `GOWORK=off go test ./...` from `sdk/` exits 0 and `go.mod` still shows zero requires
- [x] 1.2 Scaffold `providers/gcp` (go.mod with placeholder root/sdk requires, layout, plugin main skeleton, auth package shell) with the import gate — verify: workspace `go build ./...` from `providers/gcp` exits 0 and the forbidden-import grep (internal/cloud/*, internal/config, internal/cli) exits 1 (`GOWORK=off` proof belongs to Order 7: no versions exist pre-tag)
- [x] 2.1 Move the GCP tree, acceptance cmds, scripts, and floci suites per the list above (tree red by design from here through 3.x) — verify: `git status` shows the moves as renames and nothing remains under `internal/cloud/gcp/` or `cmd/gcp-*/`
- [x] 2.2 Move `queue/` plus `search/` to `internal/cloud/kube/` and update EKS plus moved-tree imports — verify: `go build ./internal/cloud/aws/eks/` exits 0 and no `cloud/gcp/queue` or `cloud/gcp/search` import remains
- [x] 2.3 Vendor the NRDOT helm subset with a fork note; verify vendored scope plus the sibling rule over the moved tree — verify: the vendored files carry source citations and import only allowed paths, and no package imports sibling providers (full-tree config/cli/cmd gates land with the 3.1 severing)
- [x] 3.1 Rewire moved entries to the protocol (ValidateConfig over moved GCPTarget schema, Plan from intents plus raw bytes, drop infra registration and shared-port adapters for native ops) — verify: `providers/gcp` builds standalone and the import gate from 1.2 still passes
- [x] 3.2 Implement the net/rpc protocol server plus all 30 operations (lifecycle, bootstrap, state, secrets, observe, tunnel, cost, cleanup, leftover destroy, adapter proxies) with per-operation timeouts — verify: `go test ./providers/gcp/...` (module tests) pass with fakes for every operation
- [x] 4.1 Implement the core v2 client (lockfile v2 discovery, digest plus Cosign verify, dial, Describe negotiation, fail-closed errors, selected-implementation logging, plugin-backed module shim) — verify: unit tests prove tamper refusal, version-mismatch refusal, dial-failure refusal, v1-lockfile refusal, shim dispatch, and the selection log line
- [x] 4.2 Replace the CLI GCP path (registry drop, subprocess replacement, hooks slimming) and delete the dead JSON path on zero references — verify: `go build ./...` from root exits 0, `go list` over `cmd/magelift` shows no provider SDKs, and grep finds no live references to the deleted symbols
- [x] 4.3 Slim core GCP config (presence plus single-block rule), regenerate schema, update config tests — verify: config suite green and `make generate-check` (or the brief substitute) green
- [x] 5.1 Implement token-source auth with the four expiry tests (expiry-refresh, refresh-failure typed error, restart statelessness, CI-shaped ADC) plus no-secrets-in-logs asserts — verify: `go test ./providers/gcp/auth/ -count=1` passes
- [x] 5.2 Apply follower updates (identity, ops, shared, cleanup suites; synthetic missing-required flipped with 4.3) — all absorbed by 4.2/4.3 with updated expectations; verified green here — verify: every suite in the follower list greens with updated expectations
- [x] 6.1 Wire Makefile targets and forwards; update docs and skill; run humanizer plus marks — verify: `make provider-gcp-build`, `make core-leanness`, and `make docs` exit 0; prose passes recorded
- [ ] 6.2 Run the full proof matrix — verify: the workspace provider build plus both import-closure gates from the spec pass verbatim, all affected suites green, `git status` shows only listed paths

## Risks

What could break, and the check for each.

- Tree red mid-arc (boxes 2.x–3.x by design): boxes verify by inventory plus per-module builds, not full green; full green returns at 4.2 and is proven again at 6.2.
- Vendor drift (copies diverge from sources): fork notes cite source plus date; copies are behavior-frozen (no feature work inside them); tests pin behavior at copy time.
- Shared-adapter entanglement (resilience/state ports): box 3.1 goes native-only; if a shared port proves load-bearing, the box fails loudly and the plan gains an explicit promotion-vs-vendor decision instead of silent coupling.
- go-plugin net/rpc sharp edges (gob version skew, no streaming): handshake plus Describe versions gate compat; Status polling replaces streams; abort kills the process; falsifying test: version-mismatch refusal in 4.1.
- Acceptance-harness path rot (moved scripts/cmds): box 6.1 runs every forwarded target in dry-run or list mode where supported; live runs stay forbidden here.
- Credential ambient leakage in tests (ADC picked up from the dev box): box 5.1 tests pin fake token sources with env scrubbed; suite refuses ambient ADC via explicit test config, never via network.
- Live GCP temptation: forbidden in this intent without maintainer approval at plan review; box 6.2 asserts no live commands ran (this plan is approved autonomous without it — see report).

## Proof

The end-to-end evidence that the whole spec is met, not the per-step verifies
above. Tests, commands, or screenshots.

- The workspace provider build plus both import-closure gates from the spec pass verbatim (separated provider build; SDK-free core closure).
- `go test ./...` scoped suites green on both modules (root plus providers/gcp).
- `make docs` exits 0; prose pipeline recorded.
- No cloud resources created, changed, or destroyed (no live commands run).
