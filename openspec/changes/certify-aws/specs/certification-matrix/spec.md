## ADDED Requirements

### Requirement: First-party providers own per-provider certification catalogs

Each first-party provider (AWS, GCP, OVHcloud, Scaleway) MUST have a `certification-<provider>` spec that enumerates implemented cells and names the certified subset. Shared evidence stages, tuple identity, and “no inferred smoke” rules remain in this capability. A packed multi-provider campaign MUST NOT be the only tracker for a provider’s cells.

#### Scenario: AWS KEEP stall does not hide GCP status

- **WHEN** AWS Aurora KEEP is in progress and GCP Autopilot KEEP has already passed
- **THEN** `certification-gcp` can record GCP status independently of `certification-aws` and the packed campaign MUST NOT treat both as a single incomplete row
