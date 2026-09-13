## ADDED Requirements

### Requirement: GCP coverage is independent of AWS packing

GCP certification coverage MUST enumerate Autopilot versus Standard cold boundaries and allowed warm service transitions on one GCP KEEP digest. It MUST NOT share a fingerprint, prefix, or teardown with AWS. Exhaustive support means every implemented `target.gcp` cell is listed; it MUST NOT mean one live shop per combination.

#### Scenario: Autopilot and Standard are separate cold baselines

- **WHEN** a session has an Autopilot Magento KEEP baseline and YAML switches to `gke-standard`
- **THEN** the runner starts a cold Standard baseline instead of a warm Autopilot cell-update
