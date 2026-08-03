# ADR 0007: Multi-provider roadmap and community targets

- Status: Accepted
- Date: 2026-07-19

## Context

V1 certifies AWS ECS Fargate only. Maintainers can validate AWS sparsely with free
credits and intend to validate GCP with an existing project for a v1.1 track.
Azure, OVHcloud, Hetzner, and others expose Pulumi providers, but Pulumi plugin
availability alone does not make a Magento platform target. Emulators (Floci,
floci-gcp, floci-az) help offline SDK coverage; real accounts and community reports
cover production-shaped pain.

MageLift must grow providers without leaking cloud product names into portable YAML,
without pretending every Pulumi backend is certified, and without forcing the core
team to fund continuous multi-cloud CI.

## Decision

### Certification tiers

| Tier | Meaning | Who maintains |
| --- | --- | --- |
| Certified | Passes the shared Magento acceptance suite on a real account; documented ops | Core (first-party) |
| Experimental | In-tree or published module; incomplete suite; may break | Core or named owner |
| Community | Out-of-tree module implementing `sdk/v1` contracts; not MageLift-certified | External maintainers |

MageLift does not claim multi-cloud support until at least two **certified**
first-party targets exist (ADR 0002). Experimental and community targets must be
labeled as such in docs and CLI output.

### First-party layout

Provider code stays under `internal/cloud/<provider>/` with the same package
boundaries as AWS (ADR 0004). Planned order:

1. **AWS**: v1 certified target (ECS Fargate).
2. **GCP**: v1.1 candidate (`internal/cloud/gcp`), validated on a maintainer GCP
   project; offline work may use floci-gcp where useful.
3. **Azure / OVHcloud / Scaleway / Hetzner**: later first-party or community, depending on
   demand and ownership. Hetzner/OVH/Scaleway often map to Kubernetes/K3s-shaped targets
   rather than copying the AWS managed-service graph. OVH (`ovh` / `mks`) and Scaleway
   (`scaleway` / `kapsule`) ship as experimental first-party adapters validated with Pulumi
   mocks (no paid multi-cloud CI by default).

Portable contracts (`Target`, `CapabilityProvider`, artifact requirements, deploy
orchestrator) remain provider-neutral. Topology, cost, and recovery stay
provider-specific and out of `magelift.yaml`.

### Community providers

Community targets are **compiled Go modules** that implement versioned `sdk/v1`
interfaces and register through the existing extension registry. They are not:

- arbitrary YAML/Pulumi fragments pasted into project config;
- auto-downloaded unsigned plugins;
- required dependencies of the default `magelift` binary.

Distribution model:

- Publish as a separate Go module (for example `github.com/org/magelift-provider-foo`).
- Consumers opt in by building a custom CLI/binary that imports the module, or via an
  explicit, version-pinned extension load path once that advanced mechanism is
  documented and security-reviewed.
- Community providers must declare capability IDs they implement and document which
  Magento acceptance checks they pass or skip.

Core may later host a curated list of community providers with no certification
promise. Issues against community providers are triaged to their maintainers.

### Verification strategy

| Layer | Mechanism |
| --- | --- |
| Daily offline | Floci (AWS), later floci-gcp / floci-az where applicable; Pulumi mocks |
| Sparse real cloud | Local maintainer runs (`docs/aws-acceptance.md`); destroy-on-exit |
| Breadth | Community issue reports from disposable customer accounts |
| CI | No paid multi-cloud GitHub matrix by default |

### Shared acceptance suite

Every certified target must pass the same application-level suite: immutable digest
deploy, capability injection, Magento migrate candidate, runtime health, destroy or
equivalent teardown. Provider-specific evidence (OIDC shape, managed search auth,
edge) is additive, not a substitute for that suite.

## Consequences

- Certified first-party paths today: AWS ECS Fargate and GCP GKE Autopilot.
  Further providers stay experimental until the shared acceptance suite is green.
- Adding a cloud is an `internal/cloud/<provider>` (or community module) effort, not
  a YAML schema expansion.
- Free-credit and community validation are first-class; continuous paid cloud CI is
  not required for pre-alpha or for uncertified providers.
- Users who need an unsupported cloud either wait for a certified target, fund/build
  a community provider, or operate outside MageLift.

## Alternatives considered

- Lowest-common-denominator multi-cloud YAML was rejected (ADR 0002).
- Shipping many Pulumi wrappers without an acceptance suite was rejected as shallow.
- Dynamic remote plugin install as the default was rejected for supply-chain risk;
  explicit compile-time or pinned load is required.
- Replacing Floci with a second AWS-only emulator was rejected; Floci stays for AWS
  offline coverage and future multi-cloud emulator siblings.

## Provenance

Original project decision extending ADR 0001, 0002, and 0004. Informed by public
Pulumi multi-language provider patterns and MageLift's existing extension registry;
no third-party source code was copied.
