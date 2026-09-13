## ADDED Requirements

### Requirement: New Relic live cells follow thin-credit attach

New Relic Magento telemetry evidence MUST attach to a packed GCP Magento origin when possible. Thin paid credits apply. Collector lifecycle remains experimental until evidenced.

#### Scenario: New Relic does not spawn Magento

- **WHEN** New Relic queryable delivery is the open claim
- **THEN** the runner uses the existing Magento session or records the cell unproven
