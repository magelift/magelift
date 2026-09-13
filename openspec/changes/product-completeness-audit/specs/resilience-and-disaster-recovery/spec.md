## ADDED Requirements

### Requirement: Auto-healing is bounded and visible

Health checks, readiness, liveness, restarts, instance replacement, and autoscaling MAY recover instance-level failure. They MUST emit observable health or event records. After the documented retry budget, a systemic failure (bad digest, failed Magento setup, quota exhaustion) MUST surface as deploy or health failure. Auto-healing MUST NOT be reported as success while Magento remains unusable. Failover and rollback remain as specified in the existing HA and DR requirements.

#### Scenario: Quota exhaustion during replacement

- **WHEN** a failed instance cannot be replaced because the provider returns quota exhaustion
- **THEN** health is unhealthy, the error names quota, and MageLift does not report the environment as self-healed
