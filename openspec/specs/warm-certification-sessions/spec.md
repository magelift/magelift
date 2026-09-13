## Purpose

Lets the certification runner reuse compatible infrastructure cells to reduce
paid-provider startup and teardown time while keeping state changes, evidence,
and final cleanup trustworthy.

## Requirements

### Requirement: Session reuse is fingerprinted

Every warm session MUST fingerprint provider, region, runtime topology, edition,
Magento release, immutable artifact digest, PHP and Composer contract, database
engine and major version, search, cache, queue, web-cache, edge, and cell
catalog inputs. A changed fingerprint MUST invalidate or archive the old
checkpoint before provisioning.

#### Scenario: The image digest changes

- **WHEN** a new immutable image digest is supplied to an existing session
- **THEN** the runner refuses checkpoint reuse and starts a new baseline

### Requirement: Warm reuse has explicit boundaries

The runner MUST permit warm reuse only for service-only changes that preserve
the provider, runtime topology, database engine and major version, application
artifact, and schema assumptions. A new provider, Kubernetes topology, database
engine or major version, migration-affecting artifact, or managed-to-self-hosted
replacement MUST require a cold baseline.

#### Scenario: Queue mode changes on a compatible stack

- **WHEN** only the queue service changes from database messaging to a supported
  self-hosted broker
- **THEN** the runner may update infrastructure in place and run the queue and
  runtime health cells without rerunning the baseline migration

#### Scenario: Database engine changes

- **WHEN** a session changes from MySQL to MariaDB
- **THEN** the runner starts a cold session and performs a fresh migration

### Requirement: Migration ownership is unambiguous

Each warm session MUST run the full Magento migration once during its baseline
cell. Later service-only cells MUST use an infrastructure-only update plus
bounded service and runtime health checks. Infrastructure components MUST NOT
start a second migration job for the same session.

#### Scenario: A later search cell is executed

- **WHEN** the baseline cell already completed migration and the search mode is
  changed
- **THEN** the search cell does not launch a second schema migration

### Requirement: Checkpoints are safe to resume

The runner MUST record the session fingerprint, completed cells, stack identity,
ownership markers, and cleanup mode atomically enough to resume after an
interruption. A stale or incompatible checkpoint MUST be archived rather than
silently trusted.

#### Scenario: The process stops after provisioning

- **WHEN** the operator resumes with the same fingerprint
- **THEN** the runner continues from the recorded cells and retains ownership of
  the existing stack

### Requirement: Final teardown is mandatory for cost closure

The final session invocation MUST perform dependency-aware destroy, bounded
provider-specific orphan cleanup, direct service inventories, and a cleanup
assertion. Retained-debug mode MUST be explicit and MUST appear in evidence.

#### Scenario: The last warm cell passes

- **WHEN** the cell catalog is complete and retention is disabled
- **THEN** the runner destroys the shared stack once and records a direct empty
  inventory for every resource family it owns

### Requirement: Warm evidence distinguishes reuse from a new baseline

Generated evidence MUST record whether each cell created the baseline or reused
it, the shared stack identity, the fingerprint, the per-cell duration, and the
final cleanup result. Warm reuse MUST NOT be presented as an independent cold
provisioning certification.

#### Scenario: A reviewer compares two queue cells

- **WHEN** the first queue cell creates the stack and the second reuses it
- **THEN** evidence shows the shared baseline and labels the second result as a
  warm service transition

### Requirement: Vendor live tests reuse packed Magento sessions

Warm sessions MUST allow Cloudflare, Fastly, New Relic, and SendGrid attach onto an existing packed GCP Magento fingerprint when only the vendor path changes. A new Magento origin MUST NOT be created solely for those vendors.

#### Scenario: Fastly attaches to KEEP Magento

- **WHEN** a packed GCP Magento session is live and Fastly evidence is still open
- **THEN** the runner may add Fastly to that session and records the vendor as a warm or attach cell, not a new Magento baseline
