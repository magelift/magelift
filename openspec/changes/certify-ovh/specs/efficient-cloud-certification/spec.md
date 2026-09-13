## ADDED Requirements

### Requirement: OVH stays one serialized preview

OVH certification coverage MUST be one MKS Magento preview, serialized against other OVH mutations, with no KEEP and no overlap with AWS or GCP KEEP fingerprints. Exhaustive support means every implemented `target.ovh` cell is listed; it MUST NOT mean one live shop per combination.

#### Scenario: OVH does not wait on AWS Aurora

- **WHEN** AWS KEEP is mutating Aurora and OVH credits allow one MKS preview
- **THEN** the OVH preview MAY run in its own prefix and state, and AWS stall MUST NOT hide OVH catalog status
