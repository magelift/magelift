## ADDED Requirements

### Requirement: OVH evidence maps to certification-ovh cell IDs

Each OVH live Magento (or declared infra-only) evidence file MUST identify the `certification-ovh` cell: region, MKS plan, MySQL plan/version, Valkey plan/version, and digest. Topology-only, unit, or Floci rows MUST NOT satisfy a Magento certification gate.

#### Scenario: Private-network unit split is not Magento evidence

- **WHEN** EU-WEST-PAR tests separate gateway, floating IP, and CNI failures
- **THEN** evidence MAY record networking honesty and MUST NOT mark an OVH Magento cell certified
