# ADR 0013: Provider plugin contract (core versus provider)

- Status: Accepted
- Date: 2026-09-16

## Context

The shipped CLI compiles provider implementations into its registration path
(`internal/registry/registry.go` registers all six first-party modules), core
configuration owns provider enums and per-provider target types
(`internal/config`), the public SDK leaves the program contract outside the
type system (`Program(ModulePlan) (any, error)` with a `pulumi.RunFunc`
assertion in `internal/platform/public_module.go`), and the wire protocol is a
narrow JSON proof (`request_json` / `plan_json` / `result_json` /
`execute_json` in `internal/providerhost/hostproto/ping.proto`). ADRs 0008 and
0011 scoped that shape deliberately as a single-version proof: exact-version
equality between CLI and provider artifact, silent fallback to embedded
execution on plugin failure, and day-2 ports staying in-process.

`intent/audit.md` (F01–F03, F15) finds that boundary unfinished: calling it
done would mislead every consumer and contributor. This ADR promotes the proof
into the versioned contract that followers implement. It changes the target
rules; it does not rewrite the history that 0008/0011 record.

## Decision

### Ownership split

The shipped core owns configuration envelopes, command UX, plugin discovery,
trust (identity, integrity, compatibility), locking, and orchestration policy.
The provider owns its cloud implementation, its Pulumi program and stack
execution, and its operating capabilities. Each responsibility sits on exactly
one side; there are no jointly owned rows.

### Autonomous provider operations

A provider is autonomous when it provisions and independently operates the
seven day-2 operations over the versioned protocol: status, logs, execution,
backup, restore, teardown, and credential refresh. Cost-estimate inputs are
required; full cost adapters are out of scope for alpha. A plugin that
provisions but cannot operate is not the product vision.

### Versioned typed operations protocol

The protocol is versioned typed operations, not a hardened envelope over the
JSON host protocol: `ping.proto` stays frozen as the single-version proof and
is not extended. The operations set is stack lifecycle (plan, preview, apply,
destroy, refresh, outputs with a redacted variant) plus Describe
(negotiation) plus ValidateConfig plus the seven day-2 operations. Requests
and responses are typed; no operation carries an untyped JSON string or a Go
`any` holding a Pulumi program. Errors are typed codes with human-readable
detail and a retryability flag. Cancellation is per-operation context
cancellation across the transport; progress is a typed event stream
(started/progress/log/done). Provider state is opaque bytes to the core: the
core stores and returns it without inspection, and secret values never appear
in logs or evidence.

### Execution boundary

The provider executes its own Pulumi Automation API stacks inside the provider
process. A `RunFunc` closure cannot cross gRPC (established by ADR 0011), so
the `any`-carrying `Program` shape is retired: stack work runs provider-side
and only typed results cross. The core never imports provider SDKs or Pulumi
provider packages.

### Configuration split

The core owns a stable common YAML envelope (project, environments, target
selectors, application, presets) and validates it. Each provider owns its
`target.<provider>` block: it publishes the schema and validates the raw
block bytes passed to ValidateConfig and Plan. The cloud resource model is
not forced to a lowest common denominator. The core accepts any well-formed
provider ID at parse; unknown providers fail at resolution, not at parse:
no installed compatible plugin is a fail-closed error naming the provider ID,
the required protocol version, and the install action — never a silent
substitution.

### Negotiation and fail-closed compatibility

The provider Describe call reports protocol version, provider ID and version,
supported operations with versions, and target runtimes. The core requires a
protocol-major match plus the operations each command needs. Checksum
rejection, incompatibility, and plugin load/dial errors each fail closed. The
only sanctioned fallback is an explicit `--migration-mode` flag for moving
between embedded and plugin execution; migration runs log the selected
implementation at startup, and every run logs the selected plugin ID plus
version. Silent conversion of plugin errors into embedded execution is
forbidden.

### Independent releases

Provider implementations live in nested modules under `providers/<name>/`
(one `go.mod` each); the core never imports provider packages. Tags are
`vX.Y.Z` for the core, `sdk/vX.Y.Z` for the SDK, `providers/<name>/vX.Y.Z`
for providers. Alpha ships all three lockstep; the independent-cadence rules
(additive changes minor, removals major, protocol major enforced at
negotiation via the core-held compatibility matrix) are specified here and
activate post-alpha. Independent releases are a build and dependency
contract; separate Git repositories are explicitly not required and not
created.

### Proof gates for followers

`gcp-autonomous-provider` proves the contract with two MUST-pass gates: a
provider built outside the root module using only the public SDK (clean
module, `GOWORK=off`), and a core built without provider SDK dependencies
(`go list` over the shipped CLI closure rejecting provider SDK packages).

### Process boundary honesty

The provider subprocess runs with the user's cloud privileges. The process
boundary is a deployment and compatibility boundary, not a security sandbox.
No section of this contract calls it a sandbox or implies privilege
separation.

### Documentation updates

The boundary sections of `docs/adding-a-provider.md` and
`docs/architecture.md` describe this contract as specified, and ADRs 0008 and
0011 carry reciprocal scope notes. Human docs and decision record ship
together; neither lands without the other.

### Supersession of 0008 and 0011

This ADR supersedes as target rules: 0008's "Magento deploy still needs the
AWS and GCP adapters in the same process as the CLI for RC1" and "adapters
may stay in-process for one RC1 release if extract lags"; 0011's
"Single-version v1: the provider artifact version equals the CLI version",
"A failed subprocess attempt falls back to the in-process backend with a
stderr notice. It never fails the command.", and "Day-2 ports stay
in-process for every adapter". What stands: no unsigned downloads, HashiCorp
go-plugin transport with `plugin.Open` forbidden, lockfile digest plus Cosign
identity verification before spawn, in-process loading for tests and Floci,
compile-time community binaries, provider-side Pulumi execution, and
operations crossing as data. The current code still behaves the old way where
noted; Order 5 implements this contract, and behavior changes land there.

## Consequences

- Follower intents implement against this contract instead of freelancing the
  protocol; `gcp-autonomous-provider` is unblocked on a specified boundary.
- `internal/cloud/<provider>/` keeps topology until Order 5 performs the
  specified move; no directory-only reorganization happens here.
- The SDK gains protocol types under the compatibility rules above; nothing
  is exported merely to make the current package graph compile.
- Distribution trust mechanics (download, install, lockfile) are specified
  at policy level here and implemented in
  `verified-provider-distribution`.

## Alternatives considered

- Hardened envelope over the JSON host protocol: rejected (the JSON proof
  does not grow into the product; typed operations from the start).
- Provider returns a Pulumi program handle: impossible across gRPC (same as
  ADR 0011); execution stays provider-side.
- Lowest-common-denominator cloud resource model: rejected (destroys
  provider expressiveness; the envelope stays stable instead).
- Separate Git repositories per provider: rejected (multiplies
  cross-repository changes before boundaries settle; the monorepo stays).
- Silent migration mode: rejected (a rejected checksum must never change the
  selected implementation quietly).

## Provenance

`intent/audit.md` F01–F03/F15, `intent/provider-plugin-contract/spec.md`,
`sdk/modules.go`, `internal/platform/public_module.go`,
`internal/providerhost/hostproto/ping.proto`, `internal/config`,
`internal/registry/registry.go`, ADRs 0003/0004/0008/0011.
