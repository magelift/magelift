## ADDED Requirements

### Requirement: Fastly live cells follow thin-credit attach

Fastly Magento edge evidence MUST attach to a packed GCP Magento origin when possible. Magento-safe WAF on Fastly MUST follow `magento-aware-security`. Thin paid credits apply.

#### Scenario: Fastly is not a second Magento origin

- **WHEN** Fastly routing evidence is required
- **THEN** the origin is the existing Magento cell and Fastly is recorded as an external edge capability
