## ADDED Requirements

### Requirement: Scaleway evidence maps to certification-scaleway cell IDs

Each Scaleway live Magento (or declared infra-only) evidence file MUST identify the `certification-scaleway` cell: region/zone, Kapsule version, RDB HA, Redis cluster size, and digest. Account-free cost rows and unit/Floci rows MUST NOT satisfy a Magento certification gate. Cockpit source configuration MUST NOT certify Magento telemetry delivery.

#### Scenario: Cockpit sources are not Magento certification

- **WHEN** the Scaleway adapter declares Cockpit log/metric/trace sources
- **THEN** evidence MAY record adapter capability and MUST NOT mark a Scaleway Magento cell certified
