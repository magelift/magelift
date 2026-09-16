---
status: specified
slug: gcp-autonomous-provider
intent: intent.md
---

# Spec: autonomous GCP provider (the first real plugin)

## Requirements

What the system must do. Testable. Not file paths. One block per requirement,
each with at least one scenario.

### Requirement: Out-of-module provider build with enforced import rules

`providers/gcp` SHALL be a nested Go module (own `go.mod`) that builds
in the workspace. It SHALL import only the public SDK, the Go standard
library, its own cloud dependencies, and versioned root internal shared
libraries from the allowlist: `platform`, `cloud/kube` (including
`kube/queue` and `kube/search`), `deploy`, `automation`, `provider`,
`topology`, `shared/*`, `secretref`, `secretsafe`, `edge/waf`,
`external/edge`, `external/observability`. Forbidden direct imports are
sibling provider implementations (`internal/cloud/aws`,
`internal/cloud/ovh`, `internal/cloud/scaleway`), core-owned singletons
(`internal/config`, `internal/cli`, `cmd/*`), and `internal/external/newrelic`
(whose transitive closure is disproportionate; the needed NRDOT subset is
vendored instead). Forbidden transitively are sibling providers, `cli`,
and `cmd/*`. Transitive `config`/`build`/`certification` presence via
`platform`/`kube` is tolerated dead weight (documented; a future platform
slimming may remove it) — the provider never calls it. The shipped core
SHALL build without provider SDK dependencies. All rules SHALL be enforced
by executable gates.

Refinement of the Order-2 proof wording (flagged for maintainer sign-off):
"using only the public SDK" holds for the wire contract; the implementation
additionally requires versioned root internal libraries. Strict SDK-only
would fork the core into the provider (measured: platform and kube drag
build, certification, config, and deploy transitively); SDK promotion of
those packages would bloat the public surface. Go modules version the
library dependency like any other.

#### Scenario: Separated provider build

- **WHEN** `go build ./...` runs from `providers/gcp` in the workspace
- **THEN** it exits 0; direct imports show no sibling providers, no
  `internal/config`, no `internal/cli`, no `cmd/*`, and no
  `internal/external/newrelic`; and the transitive closure shows no sibling
  providers, cli, or cmd packages. Full `GOWORK=off` consumer resolution is
  impossible before the first tags exist (the unpublished root has no
  versions to require); placeholder requires ride the workspace now, and
  the `GOWORK=off` proof belongs to `verified-provider-distribution`
  (its roadmap exit), not here.

#### Scenario: SDK-free core build

- **WHEN** `go list` runs over the shipped `cmd/magelift` dependency closure
- **THEN** no AWS, GCP, OVH, Scaleway, Pulumi provider, or Kubernetes client
  package appears

### Requirement: Versioned typed protocol over plugin transport

Core and provider SHALL speak versioned typed operations (Go structs over
HashiCorp go-plugin net/rpc with gob): every message carries an explicit
protocol version; operations are named with typed requests, responses, and
error codes. The frozen JSON gRPC proof SHALL NOT be extended; the GCP JSON
dial path and its silent fallback SHALL be removed so exactly one execution
path exists. Cancellation means client abort (plugin process killed) plus
idempotent reconcile; progress means typed `Status` polling with a log tail
plus cursor. A future streaming transport is a protocol-major upgrade, not
alpha scope.

#### Scenario: Typed operations, no untyped blobs

- **WHEN** the protocol surface is reviewed
- **THEN** every operation has a named Go request/response struct with a
  version field; rich pre-existing payloads (stack outputs, platform
  reports, adapter plans) cross as JSON bytes inside typed envelopes with
  the JSON schema named on each field (RawExtension pattern, never an
  untyped blob); no gob `any`/interface field exists on the wire; and
  `ping.proto` plus its generated code are byte-identical or deleted
  (never extended)

#### Scenario: Version mismatch fails closed

- **WHEN** a plugin reporting an incompatible protocol major is selected
- **THEN** the command fails before any cloud call naming the expected and
  reported versions, with no fallback execution

### Requirement: Provision plus the full day-2 surface in-provider

The provider SHALL serve 28 operations mirroring the module interfaces
1:1, all executed inside the provider process against GCP APIs. Lifecycle:
Describe, ValidateConfig, Plan, Apply, Outputs, Destroy. Bootstrap:
BootstrapVerify, BootstrapEnsure. State (Pulumi state, locks, snapshots):
StateStatus, StateLock, StateUnlock, StateBackup, StateRestore. Secrets:
SecretList, SecretSet, SecretRemove, SecretRead (secretref value
resolution; JSON-field extraction stays core-side). Observe: TailLogs,
CheckRuntime, PrepareExec. Tunnel: PrepareTunnel (target computed
provider-side; the launcher runs client-side). Cost: CostInputs (report
from resolved inputs plus raw target block; no stored plan required).
Cleanup ledger replay: Inventory, Delete (ledger-carried identity; no
envelope). Adapter proxies: EdgePlan, EdgeExecute, ResiliencePlan,
ResilienceExecute. No Preview/Refresh ops (no code referent), no redacted
Outputs variant (redaction stays core-side), no CredentialRefresh op (the
contract's cred-refresh is the internal token-source mechanism with its
own expiry tests). No other operations may be added without a spec
amendment.

#### Scenario: Each operation independently exercised

- **WHEN** the provider suite runs
- **THEN** every operation above has at least one passing test against fakes
  (no cloud), and destroy destroys exactly what the provider created

### Requirement: Explicit negotiation with fail-closed integrity

Selection SHALL verify lockfile digest plus Cosign identity before spawn,
negotiate protocol major plus required operations via Describe, and fail
closed on any integrity or compatibility failure. Every run SHALL log the
selected plugin ID plus version. No migration flag ships in this intent:
with no embedded GCP implementation left, there is nothing to migrate from,
so a single execution path satisfies the contract vacuously.

#### Scenario: Tampered plugin refuses before spawn

- **WHEN** the installed provider binary or bundle fails digest or identity
  verification
- **THEN** the command fails before spawning, naming the failed check

#### Scenario: Selected implementation always visible

- **WHEN** any provider operation runs
- **THEN** the startup log names the plugin ID plus version

### Requirement: Fresh credentials with tested expiry behavior

Operations SHALL obtain fresh GCP credentials per operation through ADC
(no saved-token dependence; the static-token kubeconfig builder is not
carried over). Expiry SHALL self-heal via token refresh; refresh failure
SHALL surface as a typed credential error naming re-authentication (no
cached-token fallback); restart SHALL re-read ambient credentials
statelessly; CI identity SHALL flow through the same ADC path (WIF).
Refreshed secrets SHALL never land in YAML, logs, or evidence.

#### Scenario: Expiry, failure, restart, CI covered

- **WHEN** the provider auth suite runs
- **THEN** passing tests exist for token expiry with refresh, refresh
  failure surfacing the typed error, restart statelessness, and CI-shaped
  ADC selection — all with fakes, asserting no secret values in captured
  logs

### Requirement: GCP config split with the core kept honest

Provider target validation SHALL move to the provider's ValidateConfig over
its own schema (moved `GCPTarget` struct plus validation, YAML-parsed from
raw block bytes). Core config SHALL slim its GCP case to structural presence
plus a generic at-most-one-target-block rule (no per-provider semantics, no
new provider knowledge). The four-provider enum stays (removal belongs to
`verified-provider-distribution`, which owns resolution); the Order-3
synthetic suite SHALL be updated in this change per its follower rule
(missing-required now passes core validation; the provider suite asserts
rejection on the same fixture).

#### Scenario: Split enforcement points

- **WHEN** the GCP suites run
- **THEN** core config accepts a structurally present but semantically empty
  `target.gcp`, the provider's ValidateConfig rejects it naming the field,
  and the synthetic suite greens with updated expectations

### Requirement: Minimal deliberate SDK additions

SDK additions SHALL be limited to wire-necessary data: the protocol message
types plus the minimal shared wire values both sides need (output keys,
cost-estimate data shapes). No interfaces, no behavior, no third-party
imports. The SDK SHALL remain dependency-free.

#### Scenario: SDK stays lean

- **WHEN** the SDK module is listed and vetted standalone
- **THEN** `go.mod` shows zero requires, the new files contain data types
  plus validation only, and every addition is referenced by the protocol

### Requirement: Human docs match the built provider

The capability-matrix GCP rows SHALL name the `providers/gcp` home, the
adding-a-provider plugin path SHALL describe authoring against the built
contract, and the provider skill SHALL match. No page SHALL claim AWS, OVH,
or Scaleway extraction.

#### Scenario: Docs describe one provider

- **WHEN** the touched pages are reviewed
- **THEN** GCP rows point at `providers/gcp` with Autopilot certified and
  Standard experimental, and no other provider is described as extracted

## Design

How it fits the existing codebase: surfaces, data, APIs, ownership.

`providers/gcp/` (nested module): moved `internal/cloud/gcp/*` (Autopilot
plus Standard runtimes; Standard stays experimental), vendored minimal
copies (kube deploy subset as `kubeops`, Magento shell/env helpers as
`magento/`), moved `GCPTarget` schema plus validation, the net/rpc protocol
server, the token-source auth package, and the plugin main. Internal
`internal/cloud/gcp/` is deleted after the move. New code: protocol server
plus client, auth package, discovery plus negotiation plus verification on
the core side (providerhost v2 surface; legacy JSON dial path removed),
core config slimming plus schema regeneration, cost/keys SDK additions.

Protocol operations (unary request/reply; versions explicit): Describe
(protocol version, provider ID/version, operation versions, runtimes);
ValidateConfig; Plan; Apply; Outputs (JSON-object values); Destroy;
BootstrapVerify/BootstrapEnsure; StateStatus/StateLock/StateUnlock/
StateBackup/StateRestore; SecretList/SecretSet/SecretRemove/SecretRead;
TailLogs/CheckRuntime/PrepareExec; PrepareTunnel; CostInputs;
Inventory/Delete; EdgePlan/EdgeExecute; ResiliencePlan/ResilienceExecute
(see sdk/protocol.go; the list is method-mapped 1:1 to the module
interfaces it replaces). Rich payloads cross as named-schema JSON bytes
inside typed envelopes. Errors are typed codes (integrity, compatibility,
credential, not-found, conflict, upstream) with detail plus retryability.
Opaque provider state (bytes) is stored by the core storage path and
returned uninspected. The lockfile moves to schema version 2 with
a required `protocol` marker per entry (`magelift-v2`); version 1 files
are refused with a re-lock error (nothing versioned shipped pre-alpha, so
no migration path is owed).

go-plugin handshake (magic cookie plus protocol version string) gates
transport compat before Describe negotiates semantics. Lockfile digest plus
Cosign identity verify before spawn (existing verify logic reused).
Timeouts are per-operation and explicit; long operations are polled, never
streamed.

## Gotchas / policy flags

Security, auth, PII, compatibility, contradictions the spec cannot satisfy.

- Implements ADR 0013; the two refinements above (library imports, no
  migration flag) plus the spec-scenario search refinement from Order 4
  carry-forward are flagged for maintainer sign-off, not freelanced
  silently.
- Fail closed everywhere: integrity, compatibility, credentials, digests.
  No silent fallback; exactly one GCP execution path when this lands.
- No secret values in YAML, logs, or evidence — references only; refreshed
  credentials included. Tests assert on captured logs.
- Topology ownership holds; no shared components behind `if provider ==`.
- The JSON proof path is removed, not extended; dead code goes with it
  (verified by build plus reference grep).
- Human docs and website copy go through humanizer, then remove-ai-marks.
- Tests ship with the implementation: negotiation, mismatch, checksum,
  expiry/refresh/restart/CI-identity, every operation. Live proof of the
  full loop belongs to `reference-store-acceptance`; this intent proves
  fakes plus gates. Live GCP spend in this intent: none without maintainer
  approval at plan review (unit plus fake-client proof here; the packed
  live session, if authorized, is recorded per-session with spend).

## Open questions carried forward

Unresolved items from intent.md, plus new ones. Each has an owner or a default.

- Module home adopted: `providers/gcp` nested module per ADR 0013. Owner:
  contract author (adopted).
- GCP-first confirmed: existing functional search and operator evidence
  minimize new proof; no pilot signal to flip. Owner: maintainer at
  acceptance (confirmed by proceeding).
- Live-vs-fake split adopted: unit plus fake-client proof here for all
  28 ops; live proof of the full loop in acceptance. Owner: spec author
  (adopted).
- New: enum removal deferred to `verified-provider-distribution`
  (resolution owner). Owner: Order 7.
- New: protobuf/streaming transport is a future protocol-major upgrade, not
  alpha scope. Owner: maintainer.
