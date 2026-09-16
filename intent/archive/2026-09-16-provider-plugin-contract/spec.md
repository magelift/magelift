---
status: done
slug: provider-plugin-contract
intent: intent.md
---

# Spec: provider plugin contract (the boundary v1 never had)

## Requirements

What the approved specification plus ADR plus human-docs update must define.
Testable by reading the approved documents. One block per requirement, each
with at least one scenario.

### Requirement: Core versus provider ownership split

The approved spec SHALL assign every responsibility to exactly one side. The
shipped core SHALL own configuration envelopes, command UX, plugin discovery,
trust (identity, integrity, compatibility), locking, and orchestration policy.
The provider SHALL own its cloud implementation, its Pulumi program and stack
execution, and its operating capabilities: provision plus status, logs,
execution, backup, restore, teardown, and credential refresh.

#### Scenario: No shared ownership

- **WHEN** a reader opens the ownership table in the approved spec
- **THEN** each of configuration envelopes, command UX, plugin discovery,
  trust, locking, orchestration policy, cloud implementation, stack
  execution, and the seven operating capabilities appears on exactly one
  side with no "jointly owned" rows

#### Scenario: Seven operations mandatory

- **WHEN** a reader opens the autonomous-provider definition
- **THEN** status, logs, execution, backup, restore, teardown, and credential
  refresh are all listed as mandatory for "autonomous", with cost-estimate
  inputs required and full cost adapters explicitly out of scope for alpha

### Requirement: Versioned typed operations protocol

The approved spec SHALL define a small versioned protocol of typed operations
covering observable operations, typed errors, cancellation, progress,
capabilities, configuration validation, and opaque provider state. It SHALL
replace the `Program(ModulePlan) (any, error)` shape (whose `any` hides a
`pulumi.RunFunc` assertion in `internal/platform/public_module.go`) and the
JSON-payload host protocol (`request_json` / `plan_json` / `result_json` /
`execute_json` in `internal/providerhost/hostproto/ping.proto`, frozen as the
legacy single-version proof and not extended).

#### Scenario: No opaque program crossing

- **WHEN** a reader opens the protocol operations list
- **THEN** they find typed stack-lifecycle operations (plan, preview, apply,
  destroy, refresh, outputs) and typed day-2 operations, and no operation
  whose request or response is an untyped JSON string or a Go `any` carrying
  a Pulumi program

#### Scenario: Pulumi stays provider-side

- **WHEN** a reader opens the execution-boundary section
- **THEN** it states the provider executes its own Pulumi Automation API
  stacks inside the provider process (the direction ADR 0011 already
  established: a `RunFunc` closure cannot cross gRPC), and the core never
  imports provider SDKs or Pulumi provider packages

#### Scenario: Errors, cancellation, progress specified

- **WHEN** a reader opens the protocol error model
- **THEN** they find typed error codes (including compatibility and integrity
  failures), per-operation cancellation semantics, and a progress-reporting
  shape — each with MUST-level conformance language for providers

### Requirement: Configuration envelope versus provider schema split

The approved spec SHALL define a stable common YAML envelope owned by the
core and provider-owned target-block schemas validated by the provider. It
SHALL NOT force a lowest-common-denominator cloud resource model. Core
rejection of unknown providers (the `aws|gcp|ovh|scaleway` enum and the
per-provider switch in `internal/config`) SHALL be replaced by syntactic
acceptance plus plugin resolution: the core accepts any well-formed provider
ID, resolves a compatible plugin, and fails closed with a clear
no-compatible-plugin error when none is found.

#### Scenario: Envelope stable, schemas provider-owned

- **WHEN** a reader opens the configuration section
- **THEN** the envelope fields (project, environments, target selectors,
  application, presets) are listed as core-owned and versioned with the
  core, and each provider's `target.<provider>` block is listed as owned and
  validated by that provider against a provider-published schema

#### Scenario: Unknown provider fails at resolution, not at parse

- **WHEN** a reader opens the unknown-provider rule
- **THEN** it states that parse accepts any well-formed provider ID and that
  resolution failure (no installed compatible plugin) is a fail-closed
  error naming the provider ID, the required protocol version, and the
  install action — never a silent substitution

### Requirement: Explicit negotiation and fail-closed compatibility

The approved spec SHALL define compatibility negotiation: the provider
describes its protocol version, provider ID and version, supported operations
with versions, and target runtimes; the core requires a protocol-major match
and the operations each command needs. Integrity or compatibility failure
SHALL fail closed. The ADR 0011 behaviors of exact-version equality and
silent fallback to embedded execution on plugin failure SHALL be explicitly
superseded; an intentional migration mode MAY exist but SHALL require an
explicit flag and SHALL log the selected implementation at startup.

#### Scenario: Negotiation inputs named

- **WHEN** a reader opens the negotiation section
- **THEN** they find the Describe fields (protocol version, provider
  ID/version, operation versions, runtimes) and the core's acceptance rule
  (major match plus required-operation presence) stated normatively

#### Scenario: No silent fallback

- **WHEN** a reader opens the failure-mode section
- **THEN** checksum rejection, incompatibility, and plugin load/dial errors
  each map to a fail-closed error, and the only sanctioned fallback is an
  explicit `--migration-mode` flag whose runs log the selected implementation

### Requirement: Independent release mechanics specified

The approved spec SHALL specify independent release mechanics: final provider
module homes, the tag and versioning scheme for core, SDK, and providers, the
compatibility matrix the core enforces, and the lockstep rule for alpha.
Separate Git repositories SHALL be explicitly not required.

#### Scenario: Module homes decided

- **WHEN** a reader opens the layout section
- **THEN** provider implementations live in nested modules under
  `providers/<name>/` (one `go.mod` each, no core imports of provider
  packages), with the move itself owned by the follower intents — no
  directory-only reorganization happens in this intent

#### Scenario: Tag scheme and alpha lockstep

- **WHEN** a reader opens the versioning section
- **THEN** they find core tags `vX.Y.Z`, SDK tags `sdk/vX.Y.Z`, provider tags
  `providers/<name>/vX.Y.Z`, the rule that alpha ships all three lockstep,
  and the specified (not yet activated) independent-cadence rules for
  post-alpha, including the core-enforced compatibility matrix

### Requirement: Proof shapes for followers

The approved spec SHALL state the two proof shapes the followers must satisfy:
a provider built outside the root module using only the public SDK, and a
core built without provider SDK dependencies — each with its exact gate
command.

#### Scenario: Gates are executable

- **WHEN** a reader opens the conformance section
- **THEN** they find the out-of-module provider build command (clean module,
  `GOWORK=off`) and the provider-SDK-free core dependency gate (`go list`
  over the shipped CLI closure rejecting provider SDK packages) stated as
  MUST-pass gates for `gcp-autonomous-provider`

### Requirement: Process boundary honesty

The approved spec SHALL state that the provider subprocess runs with the
user's cloud privileges and that the process boundary is a deployment and
compatibility boundary, not a security sandbox.

#### Scenario: No sandbox claim

- **WHEN** a reader opens the security-considerations section
- **THEN** it states the privilege inheritance and the non-sandbox status in
  normative language, and no section of the spec, ADR, or human docs calls
  the boundary a sandbox or implies privilege separation

### Requirement: ADR plus human docs updated together

The approved change SHALL include an accepted ADR 0013 plus human-docs
updates: the boundary sections of `docs/adding-a-provider.md` and
`docs/architecture.md` SHALL describe the contract as specified, and ADRs
0008/0011 SHALL carry scope notes recording which bullets 0013 supersedes
(exact-version equality, silent fallback) and which stand (in-process tests,
go-plugin transport, provider-side Pulumi execution).

#### Scenario: Decision record complete

- **WHEN** a reader opens `docs/adr/0013-*`
- **THEN** its status is Accepted with a date, it names the superseded 0008 /
  0011 bullets verbatim, and 0008/0011 each carry a reciprocal scope note

#### Scenario: Human docs match

- **WHEN** a reader opens the boundary sections of `docs/adding-a-provider.md`
  and `docs/architecture.md`
- **THEN** core-vs-provider ownership, the typed-operations direction, and
  the `providers/<name>/` homes read identically to the spec (the
  `magelift-provider` skill is updated in the same change if its Boundary
  section contradicts the new text)

## Design

How the specified boundary fits the existing codebase: surfaces, data, APIs,
ownership. Normative for followers; no code lands in this intent.

The protocol is versioned typed operations (the intent's recommended default),
not a hardened envelope over the JSON host protocol: `ping.proto` stays frozen
as the single-version proof and the new operations are specified as normative
field tables (names, types, required/optional, error codes) that Order 5
implements. The operations set is stack lifecycle (plan, preview, apply,
destroy, refresh, outputs with a redacted variant) plus the seven day-2
operations plus Describe (negotiation) plus ValidateConfig. Cancellation is
per-operation context cancellation across the transport; progress is a typed
event stream (started/progress/log/done); errors are typed codes with
human-readable detail and a retryability flag. Provider state is opaque bytes
to the core: the core stores and returns it without inspection, and secret
values never appear in logs or evidence.

Configuration flows as: core parses and validates the envelope, extracts the
raw `target.<provider>` block bytes, resolves the plugin, and calls
ValidateConfig then Plan with those bytes. The provider validates against its
published schema and returns typed errors. The core never imports provider
target structs; the existing `internal/config` provider switch is removed by
followers, not here.

Trust flows as: discovery finds installed provider plugins with their
lockfile entries; the core verifies identity plus integrity before execution
(the verification mechanics are specified at policy level here and implemented
in `verified-provider-distribution`); Describe negotiates versions;
incompatible or unverifiable plugins fail closed. The startup log names the
selected implementation (plugin ID plus version, or explicit migration mode).

Topology stays in `internal/cloud/<provider>/` until Order 5 performs the
specified move to `providers/<name>/`; shared Pulumi components behind
`if provider ==` stay forbidden throughout. The SDK gains no new code here;
follower SDK work adds the protocol types under compatibility rules this spec
sets (additive changes minor, removals major, protocol major enforced at
negotiation).

## Gotchas / policy flags

Security, auth, PII, compatibility, contradictions the spec cannot satisfy.

- Specification and ADR approval before any implementation intent starts; no
  code in this intent beyond contract tests or fixtures the spec needs (none
  are needed: the protocol is specified as normative field tables).
- Do not export every existing internal interface to make the package graph
  compile. The spec deliberately splits policy (core) from schema (provider).
- Keep topology in `internal/cloud/<provider>/` until Order 5 moves it; no
  directory-only reorganization here.
- Independent releases are a build and dependency contract; separate Git repos
  are explicitly not required (and not created).
- A process boundary is not a security sandbox; the spec says so normatively.
- Human docs and website copy go through humanizer, then remove-ai-marks.
- This turn's turn-level context is long; keep the spec document tight and
  push rationale into the ADR, not into extra spec sections.

## Open questions carried forward

Unresolved items from intent.md, plus new ones. Each has an owner or a default.

- Protocol shape adopted: versioned typed operations; the JSON proof does not
  grow into the product. Owner: spec author (adopted).
- Mandatory autonomous set adopted: all seven day-2 operations plus
  cost-estimate inputs; full cost adapters out of scope for alpha. Owner:
  spec author with maintainer (adopted).
- Tag and versioning scheme adopted: `vX.Y.Z` core, `sdk/vX.Y.Z` SDK,
  `providers/<name>/vX.Y.Z` providers; lockstep for alpha, independent rules
  specified for post-alpha. Owner: spec author (adopted).
- New: proof-gate exactness — the conformance commands must run green on the
  Order-5 tree. If Order 5 finds a gate unexecutable as specified, it files
  back here rather than freelancing. Owner: Order 5 implementer.
