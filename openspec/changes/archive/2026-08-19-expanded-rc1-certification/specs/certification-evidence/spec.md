## Purpose

Defines reproducible multi-provider acceptance evidence so the expanded RC1 claim can be checked from machine-generated results rather than from hand-edited documentation.

## ADDED Requirements

### Requirement: Stable cell identity

Each acceptance cell MUST identify provider, runtime, region, edition, Adobe release line, database, search, cache, queue, web-cache, edge, artifact digest, and evidence tier.

#### Scenario: Two cells differ by one service

- **WHEN** the runner changes only the queue engine
- **THEN** it records two distinct cell IDs and retains the shared stack identity

### Requirement: Immutable artifact evidence

Every live Magento cell MUST use an immutable artifact digest and record the digest, image provenance identity, and validation result.

#### Scenario: Mutable image tag is supplied

- **WHEN** the acceptance command receives a tag without a digest
- **THEN** it refuses the live Magento cell before creating the stack

### Requirement: Shared acceptance stages

Every required live cell MUST run the shared stages applicable to its target: validate, provision, artifact deploy, Magento migration, runtime health, service exercise, evidence append, destroy, and cleanup assertion.

#### Scenario: Target does not implement a stage

- **WHEN** a target lacks a required stage
- **THEN** the cell is recorded as unsupported or incomplete and cannot pass certification

### Requirement: Generated evidence only

The runner MUST append evidence rows itself, including account or project identity, timestamp, duration, cell ID, result, and cleanup result. Hand-edited PASS rows MUST NOT satisfy the release gate.

#### Scenario: Operator edits a matrix row

- **WHEN** an evidence file contains a PASS row without a runner provenance record
- **THEN** verification rejects the row as non-certifying evidence

### Requirement: Provider-specific evidence

The evidence format MUST support AWS ECS or EKS, GCP GKE, OVHcloud MKS, and Scaleway Kapsule without changing the portable cell schema.

#### Scenario: OVH and Scaleway use managed Kubernetes

- **WHEN** the target runtime is OVH MKS or Scaleway Kapsule
- **THEN** the result uses the same application stages and records provider-specific service evidence separately

### Requirement: Release gate classification

The release gate MUST distinguish `pass`, `fail`, `blocked`, `unsupported`, and `not-run`, and MUST fail the expanded RC1 gate for any catalog cell marked `required` that is not `pass`.

#### Scenario: Required cell is blocked by missing provider service

- **WHEN** a required cell cannot run because the target lacks a service implementation
- **THEN** the expanded RC1 gate fails instead of silently reducing the claim
