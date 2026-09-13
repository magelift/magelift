## ADDED Requirements

### Requirement: GCP evidence maps to certification-gcp cell IDs

Each GCP live Magento (or declared infra-only) evidence file MUST identify the `certification-gcp` cell: runtime Autopilot or Standard, Magento release, Cloud SQL availability, Memorystore version, OpenSearch mode, queue mode, and digest. KEEP rows MUST NOT close a certified matrix cell until destroy plus orphan assert succeed. 2.4.6-p15 Cloud SQL MySQL topology evidence MUST NOT certify an Adobe-unsupported database intersection.

#### Scenario: KEEP Autopilot pass is not yet certified

- **WHEN** a GCP KEEP cell reports Magento health PASS and the stack is retained
- **THEN** evidence MAY record the KEEP run and MUST NOT replace the certified matrix row until teardown and cleanup assertion complete
