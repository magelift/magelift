## ADDED Requirements

### Requirement: Deployment locks fail closed and release by owner

Certified deploy adapters MUST require a working state backend. GCS or S3 client construction errors, including authentication and configuration failures, MUST return to the operator and MUST NOT yield a no-op lock. A missing bootstrap bucket MAY be a typed bootstrap error, not a silent skip. `Release` MUST use the caller context (with timeout), MUST verify owner and object generation, and MUST delete only that generation. Stale holders MUST NOT delete a successor's lock.

#### Scenario: GCS client cannot be created

- **WHEN** Autopilot deploy cannot construct the GCS state client
- **THEN** deploy fails before Magento mutate and does not proceed unlocked

#### Scenario: Successor acquired the lock

- **WHEN** a cancelled deploy releases after a newer owner holds the same key
- **THEN** the release does not delete the successor's lock object
