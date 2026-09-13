## ADDED Requirements

### Requirement: Scaleway stays one serialized preview inside the money cap

Scaleway certification coverage MUST be one Kapsule Magento preview, serialized against other Scaleway mutations, with no KEEP and no overlap with AWS or GCP KEEP fingerprints. Exhaustive support means every implemented `target.scaleway` cell is listed; it MUST NOT mean one live shop per combination. Live Scaleway plus vendor cells MUST share the $50 own-money cap.

#### Scenario: Scaleway does not wait on AWS Aurora

- **WHEN** AWS KEEP is mutating Aurora and the $50 cap still allows one Kapsule preview
- **THEN** the Scaleway preview MAY run in its own prefix and state, and AWS stall MUST NOT hide Scaleway catalog status
