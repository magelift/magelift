## ADDED Requirements

### Requirement: Brownfield Magento cutover is specified as P2

When an existing network or database is attached, MageLift MUST still import Magento media and a dump through documented Magelift commands, keep crypt key as a secret reference, and run Magento `app:config:import` on deploy. Destroy MUST NOT own attached VPC or database resources.

#### Scenario: Attached RDS is not destroyed

- **WHEN** the operator destroys a Magento environment that attached an existing database
- **THEN** Magelift-owned children are removed and the attached database remains
