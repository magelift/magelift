## ADDED Requirements

### Requirement: Production offers SOC 2 and ISO 27001-shaped controls

Production-class environments MUST default to reconstructable change evidence, secret references, encryption in transit and at rest where the provider offers it, private data stores, attributable exec, digest-pinned images, Magento-safe WAF, and documented backup retention. Documentation MUST NOT claim the customer is SOC 2 or ISO 27001 certified. `magelift audit` MUST export a control matrix with evidence pointers and no secret values.

#### Scenario: Audit export has no secrets

- **WHEN** an operator runs the audit report for production
- **THEN** the output lists the control set and pointers and does not contain credential values

### Requirement: Chosen-region residency is fail-closed when requested

When YAML declares a data region for Magento database, media, and backups, MageLift MUST refuse to place those data classes outside that region. Dump files remain labeled unsanitized Magento data unless sanitization is explicitly opted in. MageLift MUST NOT claim GDPR certification.

#### Scenario: Backup would leave the declared region

- **WHEN** a production overlay pins EU residency and a backup target would be another continent
- **THEN** planning fails before mutate and names the residency field
