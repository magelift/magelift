---
status: done
slug: lean-core-provider-boundary
spec: spec.md
---

# Plan: lean core with one proven subprocess adapter

Auto-approved per the standing `/goal` instruction. Implementation follows
this plan; departures update the plan in the same change.

## Files that change

- EDIT `internal/providerhost/hostproto/ping.proto`: add `Execute` RPC plus
  `ExecuteRequest`/`ExecuteResponse` JSON-carrier messages.
- REGEN `internal/providerhost/hostproto/ping.pb.go`,
  `ping_grpc.pb.go` via `go generate` (pins protoc path used).
- NEW `internal/providerhost/execute.go`: `ExecuteOperation` enum
  (`preview`, `up`, `destroy`, `outputs`, `validate-request`),
  `ExecuteRequest`/`ExecuteResult` JSON types with size caps.
- EDIT `internal/providerhost/identity.go`: extend `API` with `Execute`.
- EDIT `internal/providerhost/grpc.go`: server plus client `Execute`
  plumbing with the same decode caps as `Plan` (`1<<20`).
- EDIT `internal/providerhost/session.go`: `Session.Execute` method.
- NEW `internal/providerhost/subprocess_backend.go`: `automation.Backend`
  plus `RequestGuard` adapter over a `Session` (Preview/Update/Destroy map
  to `Execute`; `Outputs` maps to `outputs` op; `ValidateRequest` maps to
  `validate-request` op).
- EDIT `cmd/magelift-provider-gcp/main.go`: implement `Execute` by running
  the Pulumi Automation API with the existing `gcpstack` Program
  in-process inside the subprocess; `validate-request` runs the same
  preview-ownership guard the host uses.
- EDIT `internal/cli/root.go`: `newBackend` returns the subprocess backend
  for `gcp`/`gke-autopilot` when `providerhost.Load` verifies an installed
  artifact, else the existing in-process backend with a stderr notice.
- EDIT `internal/cli/extensions.go` (or the file owning `extensions list`):
  report subprocess provenance (version, digest, mode) for the proof
  provider when active.
- EDIT `.goreleaser.yaml`: second build id `magelift-provider-gcp`
  (same os/arch matrix as `magelift`); provider artifact plus Cosign
  bundle published beside the CLI archives. Single version: artifact
  version equals CLI version.
- NEW `docs/adr/0011-subprocess-dial-proof.md`: context, decision,
  consequences, alternatives, provenance. EDIT `docs/adr/README.md` index.
- EDIT `docs/adding-a-provider.md`: Dial proof status for GCP, day-2 gap,
  post-v1 extraction order.
- EDIT `docs/versioning.md`: RC stability statement covers lockfile schema
  `1`, SDK API `"v1"`, and the `Execute` RPC shape.
- NEW/EDIT tests: extend `internal/providerhost/host_test.go` (refusals,
  handshake mismatch, Execute round trip over a real Dialed test
  subprocess); new `subprocess_backend_test.go` with a fake `API`;
  clean-room no-`internal/`-imports check for
  `examples/custom-extension-contract` (extend `extension-test` path or a
  `go list` assertion in the providerhost or cli test).
- EDIT `scripts/release-smoke-local.sh` only if the provider build breaks
  the single-target smoke (keep smoke single-target).

## Order of work

- [x] 1.1 Extend `ping.proto` with `Execute` and regenerate hostproto
  (install pinned toolchain first: protoc v5.29.3, protoc-gen-go v1.36.10,
  protoc-gen-go-grpc v1.5.1, per existing generated headers) —
  verify: `go generate ./internal/providerhost/hostproto/ && GOMAXPROCS=1
  GOFLAGS=-p=1 go test ./internal/providerhost/ -count=1`
- [x] 1.2 Add `Execute` types, `API` method, gRPC plumbing, `Session.Execute`
  with size caps — verify: unit test covers cap refusal plus a fake-API
  round trip; same `go test ./internal/providerhost/` green
- [x] 1.3 Lockfile and handshake refusal coverage (checksum mismatch,
  unsigned, unknown provider, non-`v1` Ping) — verify: existing plus new
  cases in `host_test.go` pass; no subprocess starts on refusal paths
- [x] 2.1 Implement `Execute` in `cmd/magelift-provider-gcp` via Automation
  API (preview/up/destroy/outputs/validate-request), ambient credentials,
  no secret values in results — verify: `go build ./cmd/magelift-provider-gcp`
  plus an Execute-preview test under the repo Pulumi mock pattern
- [x] 2.2 Real Dial round trip: build provider binary, Dial, Ping/Describe/
  Plan/Execute-preview over gRPC — verify: new test following the
  `TestDialPingsVerifiedProvider` pattern passes locally
- [x] 3.1 Subprocess `automation.Backend` adapter with `RequestGuard` —
  verify: `subprocess_backend_test.go` with fake `API` covers
  Preview/Update/Destroy/Outputs/ValidateRequest mapping and error
  propagation
- [x] 3.2 Wire `newBackend` in `root.go`: verified artifact to subprocess,
  else in-process with stderr notice naming the artifact and install path —
  verify: CLI unit test asserts both branches without cloud credentials
- [x] 3.3 `extensions list` shows subprocess provenance (version, digest,
  mode) when active — verify: CLI unit test on the extensions output
- [x] 4.1 GoReleaser provider build plus lockfile artifact flow documented —
  verify: `make release-smoke` stays green single-target; CI matrix owns
  the full provider matrix (no local full-matrix run)
- [x] 4.2 ADR 0011 plus `adding-a-provider.md` plus `versioning.md` updates
  in the same change, humanizer applied to prose — verify: `make docs`
  strict build green
- [x] 4.3 Clean-room extension check: example builds with empty caches and
  reports zero `internal/` imports — verify:
  `./scripts/custom-extension-clean-room.sh` (i.e. `make extension-test`)
  green plus the new `go list` assertion
- [x] 5.1 Full local proof: Dial e2e plus `make verify` serial subset that
  fits this session (`fmt-check`, affected package tests, `docs`) —
  verify: commands exit 0; failures recorded, not edited around

## Deviations

- 4.1: `binary_signs` broke the local smoke (it signs at build time and
  needs Fulcio OIDC, which `goreleaser build --skip` cannot skip), so the
  provider bundle uses `signs` with `artifacts: binary` (release phase
  only). No smoke script change was needed after the switch.

- 2.1: used the documented executor seam from Risks instead of driving
  Automation API under Pulumi mocks. `WithMocks` only drives `pulumi.RunErr`,
  not Automation API stacks, so the provider selects a synthetic mock
  executor on `test://` backend URLs and Automation API otherwise. The Dial
  test `TestDialExecutesOverMockBackend` proves the subprocess execution
  path; real graph coverage stays in the in-process mock suites and real
  execution is proven by the Phase 2 live run.
- 1.3: the non-`v1` handshake refusal is enforced in `Dial` but only the
  `checkAPIVersion` helper carries a unit test; a Dial-level mismatch test
  would need a dedicated fake plugin binary. `TestDialPingsVerifiedProvider`
  covers the accept path. Recorded as a known gap, not a behavior gap.

## Risks

- `protoc` plus Go plugins missing locally: check first in 1.1; pin the
  versions used and record them in the plan if the environment provides
  them, else hand-write is forbidden and the step blocks with a named
  prerequisite.
- Automation API inside a go-plugin subprocess may fight test sandboxes:
  2.1 runs under the repo Pulumi mock pattern; if mocks cannot drive
  Automation API, add a documented executor seam in the provider binary
  and record the deviation.
- `Execute` result over `1<<20` on large previews: truncate diagnostics
  with a flag, never fail silently; covered by a cap test in 1.2.
- Preview-ownership parity between host guard and `validate-request` op:
  3.1 asserts identical verdicts on a shared fixture table.
- Release matrix doubling CI time: provider matrix stays CI-only; local
  smoke stays single-target per `magelift-serial-builds`.

## Proof

End-to-end evidence for the spec: real-Dial test output (Ping/Describe/
Plan/Execute-preview), `Load` refusal tests, `newBackend` branch tests,
`extensions list` output showing provenance, clean-room build log,
`make docs` green, ADR 0011 merged in the change. Live Dial proof
(preview plus deploy plus outputs plus destroy through the session on
Autopilot preview, destroy plus `assert_clean`) is Phase 2 on GCP and is
explicitly not claimed by this intent.
