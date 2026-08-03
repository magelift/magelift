# ADR 0008: Ports and adapters for multi-provider stacks

- Status: Accepted
- Date: 2026-07-19

## Context

Certified v1 targets are AWS ECS Fargate and GCP GKE Autopilot. Adding further clouds (and later Azure or community
clouds) must not copy Magento orchestration logic into every `internal/cloud/<p>`
package, and must not introduce a shared Pulumi resource graph with provider
switches. ADR 0002 already rejected lowest-common-denominator cloud YAML and
shared topology schemas. ADR 0004 keeps provider packages behind stable seams.
ADR 0007 places GCP as the v1.1 experimental first-party candidate.

The CLI currently types infrastructure factories on `awsstack.Spec`, which blocks
a second provider without either a large CLI rewrite or a Magento-shaped port.

## Decision

MageLift uses **ports and adapters**:

1. **Ports** live in `internal/platform` (and existing `sdk/v1` Target /
   CapabilityProvider contracts). They own Magento-shaped concerns: stack module
   registration, stable stack output keys, Magento workloads, and Magento env
   binding names. Ports do not import cloud SDKs beyond the Pulumi `RunFunc`
   type needed to hand a program to Automation API.
2. **Adapters** live in `internal/cloud/<provider>/`. Each adapter owns product
   topology (VPC, Cloud SQL, GKE, Aurora, ECS, …), catalogs, and provider YAML
   under `target.<provider>`.
3. The CLI selects a `StackModule` by `target.provider` + `target.runtime`.
   Provider-specific deploy/exec/bootstrap remain behind typed checks and fail
   clearly for experimental targets that do not implement them yet.
4. Shared Pulumi components that `switch` on provider are forbidden.

GCP (`gcp` / `gke-autopilot`) is the first second adapter and ships as
**experimental** until the shared Magento acceptance suite passes on a real
account.

## Consequences

- Adding another cloud is primarily a new adapter plus config block and module
  registration, not a third copy of Magento env/workload/CLI dispatch logic.
- AWS and GCP stay certified; thin wrappers register each as a `StackModule` without
  relocating AWS resource packages.
- Output keys used by portable CLI commands stay provider-neutral where possible;
  cloud-specific handles may be exported in addition.
- Portable YAML still does not grow a universal infrastructure catalog.

## Alternatives considered

- Shared Pulumi `Network`/`Database` components with provider switches: rejected
  (ADR 0002); APIs and failure modes diverge too far.
- Leaving the CLI AWS-typed and forking GCP commands: rejected; duplicates
  lifecycle orchestration and blocks community modules.
- CapabilityProvider-only composition without a StackModule: deferred; capability
  registration remains available, but production stacks today compose resources
  inside provider `stack` packages.

## Provenance

Original project decision extending ADR 0002, 0004, and 0007.
