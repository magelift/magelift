---
status: done
slug: lean-core-provider-boundary
intent: intent.md
---

# Spec: lean core with one proven subprocess adapter

Auto-approved per the standing `/goal` instruction (autonomous roadmap run,
maintainer decisions pre-authorized). Correct by reply; the file records the
defaults taken.

## Requirements

### Requirement: frozen public SDK boundary

The `sdk/v1` Module, Target, Capability, and Hook shapes plus the typed
EdgeIntent, ObservabilityIntent, and ResilienceIntent SHALL stay
backward-compatible through the RC series, and a community extension SHALL
build without importing any `internal/` package.

#### Scenario: clean-room extension builds from docs alone

- **WHEN** a contributor follows only `docs/adding-a-provider.md` plus
  `examples/custom-extension-contract` with empty module and build caches
- **THEN** `go build ./examples/custom-extension-contract` succeeds
- **AND** `go list -deps` on that binary shows zero
  `github.com/magelift/magelift/internal/` imports

#### Scenario: SDK API version handshake

- **WHEN** a subprocess answers `Ping` with an `SdkApiVersion` other than
  `"v1"`
- **THEN** the host refuses the session before `Describe` or `Plan` and names
  the expected version `"v1"` plus the received value

### Requirement: trust before Dial

The host SHALL verify lockfile schema, provider digest, and Cosign identity
plus issuer before starting any provider subprocess, and SHALL refuse unsigned,
digest-mismatched, or unknown providers without executing them.

#### Scenario: verified artifact Dials

- **WHEN** `magelift.providers.lock` holds `schemaVersion` `1`,
  `sdkAPIVersion` `"v1"`, and a `gcp` entry whose `digest` matches
  `^sha256:[a-f0-9]{64}$` of the binary on disk plus non-empty Cosign
  `identity` and `issuer`
- **THEN** `Load` with `ModeSubprocess` returns a session handle and `Dial`
  starts the subprocess

#### Scenario: tampered binary refused

- **WHEN** the binary bytes do not match the lockfile `digest`
- **THEN** `Load` fails with `ErrChecksumMismatch` and no subprocess starts

#### Scenario: unsigned entry refused

- **WHEN** the lockfile entry omits Cosign `identity` or `issuer`, or the
  bundle path or verifier is empty
- **THEN** `Load` fails with `ErrUnsigned` and no subprocess starts

### Requirement: subprocess Plan equals in-process Plan

`Plan` over gRPC for the proof adapter SHALL return the same stable
`ModulePlan` fields as the in-process module `Plan` for the same validated
configuration and environment.

#### Scenario: Autopilot plan round trip

- **WHEN** the host sends a `ModulePlanRequest` built from the
  `gke-autopilot` preview fixture through a Dialed `gcp` session
- **THEN** the returned plan carries identical `StackName`, `Provider`,
  `Runtime`, `Project`, `Environment`, `Region`, `Target`, and `Tier` to the
  in-process `Plan` for the same input

### Requirement: stack lifecycle executes inside the subprocess

The proof adapter SHALL run preview, update, destroy, and outputs retrieval
inside the provider subprocess through an `Execute` RPC, because a Pulumi
`RunFunc` cannot cross gRPC (`ErrProgramNotExecutable` stays the guard for
kind-only results).

#### Scenario: preview through the session

- **WHEN** the host sends `Execute` with operation `"preview"` and a validated
  plan for the `gke-autopilot` preview fixture under Pulumi mocks
- **THEN** the subprocess returns a structured change summary without
  mutating provider state

#### Scenario: lock stays with the host

- **WHEN** the CLI runs deploy or destroy through a subprocess session
- **THEN** the CLI acquires the provider deployment lock before the first
  `Execute` call and releases it after the last one, including on `Execute`
  failure

#### Scenario: secrets never cross as values

- **WHEN** any `Execute` request or response is logged at maximum verbosity
- **THEN** it contains only references and identity data, never secret values;
  the subprocess reads provider credentials from its ambient environment

### Requirement: CLI uses subprocess when verified, in-process otherwise

On the proof target (`gcp`/`gke-autopilot`), the CLI SHALL use the subprocess
session when a verified provider artifact is installed, and SHALL fall back to
the in-process module with a stderr notice naming the missing artifact when it
is not.

#### Scenario: fallback notice

- **WHEN** no verified `gcp` artifact is installed and the user runs preview
  on a `gke-autopilot` environment
- **THEN** the command proceeds in-process and stderr names the missing
  provider artifact plus the install path

### Requirement: day-2 gap stays explicit

Day-2 ports (Observe, Steps, State, Secrets) for the proof adapter SHALL stay
in-process for v1 and SHALL be recorded as the known subprocess gap in the ADR
and provider docs, not silently assumed covered.

#### Scenario: gap is documented

- **WHEN** a reader opens the Dial proof ADR plus `docs/adding-a-provider.md`
- **THEN** both state that stack lifecycle runs in-subprocess while day-2
  ports stay in-process for the proof adapter in v1

### Requirement: single-version v1 release

The v1 provider artifact version SHALL equal the CLI version it ships with;
independent per-provider releases SHALL NOT be required for v1.

#### Scenario: versions match

- **WHEN** `magelift version` prints `v1.0.0-rc.1` and `extensions list` shows
  the `gcp` provider entry
- **THEN** the provider entry reports version `v1.0.0-rc.1` with its digest
  and provenance

### Requirement: ADR plus docs land in the same change

The Dial proof decision SHALL be recorded as ADR 0011 with context, decision,
consequences, alternatives, and provenance, and `docs/adding-a-provider.md`
plus the `docs/versioning.md` RC stability statement SHALL reflect the frozen
boundary and proof scope in the same change.

#### Scenario: decision traceable

- **WHEN** a contributor asks why only one adapter runs out-of-process in v1
- **THEN** ADR 0011 answers with the Dial proof scope, the day-2 gap, and the
  post-v1 extraction order

## Design

Surfaces: extend `internal/providerhost` with an `Execute` RPC
(`hostproto` plus `API` interface plus `Session` method) carrying operation
(`preview`, `up`, `destroy`, `outputs`), validated `ModulePlan`, and
non-secret backend identity; result carries change summary or outputs plus
structured diagnostics. `cmd/magelift-provider-gcp` implements `Execute` by
running the Pulumi Automation API with the existing `gcpstack` Program
in-process inside the subprocess. CLI wiring lives where lifecycle commands
resolve the `StackModule`: `gcp`/`gke-autopilot` resolves a subprocess-backed
module when `Load` verifies, else the registered in-process module with
notice. `magelift extensions list` reports subprocess provenance (version,
digest, mode).

Data: existing `ModulePlanRequest`/`ModulePlan` JSON stays the plan
contract with current size caps (`1<<20` request/plan, `64<<10` identity and
program result); `Execute` results stay under `1<<20` with oversized
diagnostics truncated and flagged. Lockfile `magelift.providers.lock` keeps
schema `1` and SDK API `"v1"`.

Ownership: `providerhost` owns trust plus transport; `gcp/stack` owns plan
and program; the CLI owns approvals, deployment lock, and post-update health;
release engineering owns building, signing, and publishing the
`magelift-provider-gcp` artifact with its Cosign bundle.

## Gotchas / policy flags

- `ProgramResult.PulumiRunFunc()` must keep refusing kind-only results; the
  `Execute` RPC is the only subprocess execution path. No shell-out to an
  unverified binary anywhere.
- Subprocess crash mid-apply: the host surfaces the failure, releases the
  lock, and prints resume guidance; it never auto-retries a mutation.
- Keep `AutoMTLS`, gRPC-only protocol, `MAGELIFT_PROVIDER` handshake, and the
  64KiB identity cap. Log RPC payloads at debug only and never with secrets.
- Release matrix grows by one binary per platform; keep local packaging
  smoke single-target (`magelift-serial-builds`) and leave the matrix to CI.
- Live Dial proof (preview plus deploy plus outputs plus destroy through the
  session on Autopilot preview, then destroy plus `assert_clean`) runs in
  Phase 2 on GCP; this intent delivers everything up to the live run.

## Open questions carried forward

- Proof adapter is GCP Autopilot (default taken: host already serves its
  Ping/Describe/Plan/Program RPCs; reference origin; unlimited test budget).
- Dial scope is full stack lifecycle via `Execute`; day-2 stays in-process
  for v1 (default taken: deploy path is the hard part and the core value;
  day-2 moves are mechanical follow-up).
- Compatibility enforcement is the handshake plus RPC round-trip plus refusal
  tests plus the clean-room build (default taken: documentary freeze was
  judged insufficient for a plugin boundary).
