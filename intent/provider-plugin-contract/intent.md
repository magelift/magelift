---
status: draft
slug: provider-plugin-contract
---
# Intent: provider plugin contract (the boundary v1 never had)

## Problem

The shipped CLI is not a lean core with autonomous provider plugins. Provider
implementations are compiled into the production registration path, core
configuration owns provider enums and provider-specific types, the public SDK
leaves the key runtime contract outside the type system, and the wire protocol
is a narrow JSON proof — not an independent operations API. Calling this
boundary finished would mislead every consumer and contributor.

## Evidence

`intent/audit.md` F01: `cmd/magelift/main.go` wires through
`internal/cli/cli.go` plus `internal/registry/registry.go`, which still
includes provider implementations (`awsops`, `awseksops`, `gcpops`, `ovhstack`,
`scwstack`). Production dependency enumeration finds 1,855 packages including
AWS, GCP, OVH, Scaleway, and Pulumi packages — not a lean binary.

F03: `sdk/modules.go` exposes `Program(ModulePlan) (any, error)` while
`internal/platform/public_module.go` expects a `pulumi.RunFunc`; the runtime
contract lives outside the public types. Public config carries raw maps while
`internal/config` retains provider enums and rejects unknown providers.
`internal/providerhost/hostproto/ping.proto` plans and executes over JSON
payloads without a complete operations API. Public CLI hooks expose internal
hook types.

F15: the monorepo is the right home, but ownership needs the contract to
decide final provider module locations — no directory-only reorganization
before dependencies are understood.

## Proposed outcome

An approved specification plus ADR plus human-docs update defines the real
boundary: the shipped core owns configuration envelopes, command UX, plugin
discovery, trust, locking, and orchestration policy; the provider owns its
cloud implementation and operating capabilities (status, logs, execution,
backup, restore, teardown, credential refresh). A small versioned protocol
covers observable operations, errors, cancellation, progress, capabilities,
configuration validation, and opaque provider state. The common YAML envelope
stays stable; the cloud resource model is not forced to a
lowest-common-denominator. Independent release mechanics are specified (not
assumed from the old single-version proof). A provider built outside the root
module using only the public SDK, and a core built without provider SDK
dependencies, are the proof shapes the followers must satisfy.

## Affected users and systems

Extension authors, provider maintainers, core CLI contributors. `sdk/`
(public surface), `internal/providerhost/` (protocol), `internal/config/`
(envelope vs provider schema split), `internal/platform/` (ports), ADRs 0003 /
0004 plus the new boundary ADR, `docs/adding-a-provider.md`, the
`magelift-extend` skill, repo navigation map and path ownership.

## Constraints

- Specification and ADR approval before any implementation intent starts; no
  code in this intent beyond contract tests or fixtures the spec needs.
- Do not export every existing internal interface to make the package graph
  compile. Deliberate policy split: which policy belongs to core, which schema
  belongs to the provider.
- Keep topology in `internal/cloud/<provider>/` until the contract decides
  final module homes; shared Pulumi components behind `if provider ==` stay
  forbidden.
- Independent releases are a build and dependency contract; separate Git repos
  are explicitly not required.
- No silent migration mode: compatibility negotiation is explicit, and
  integrity or compatibility failure fails closed (mechanics land in
  `gcp-autonomous-provider` plus `verified-provider-distribution`).
- A process boundary is not a security sandbox: the provider runs with the
  user's cloud privileges; the spec says so.
- Human docs and website copy go through humanizer, then remove-ai-marks.

## Out of scope

- Implementing any provider against the contract (first: `gcp-autonomous-provider`).
- Distribution mechanics (signing, download, install trust): specified here
  at the policy level, implemented in `verified-provider-distribution`.
- Directory reorganization beyond what the spec requires to prove module
  homes.
- Community catalog hosting.

## Open questions

- Protocol shape: versioned RPC with typed operations, or a hardened envelope
  over the current host protocol? Default: versioned typed operations; the
  JSON proof does not grow into the product. Owner: spec author.
- Which day-2 operations are mandatory for "autonomous" (status, logs, exec,
  backup, restore, teardown, credential refresh, cost inputs)? Default: all
  seven; the spec justifies any deferral. Owner: spec author with maintainer.
- Tag and versioning scheme for core, SDK, and providers once independent
  releases exist. Default: specified here, proved in distribution. Owner:
  spec author.
