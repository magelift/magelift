## Purpose

Specifies technical controls that help teams operating Magento under SOC 2 or ISO 27001. MageLift does not certify the customer's organization.

## Requirements

### Requirement: MageLift does not claim customer certification

Documentation, CLI output, and certification rows MUST NOT state that using MageLift makes a company SOC 2 or ISO 27001 certified. They MAY state that MageLift provides technical controls useful in those programs.

#### Scenario: Help text stays honest

- **WHEN** a user reads compliance-related docs or `magelift` output
- **THEN** the text describes encryption, audit, IAM, backup, and retention controls and does not claim SOC 2 or ISO 27001 certification of the customer

### Requirement: Control evidence is reconstructable

For production-class environments, MageLift MUST make the following reconstructable from configuration, state, and logs: who applied a change (CI identity or operator principal), what configuration digest was applied, which artifact digest was deployed, secret references used (not secret values), backup policy, retention, encryption settings, and destroy or rollback events. Access to databases and exec sessions MUST be attributable as specified in remote-access requirements. Infrastructure MUST remain reproducible from configuration plus the Pulumi state backend.

#### Scenario: A production deploy is traceable

- **WHEN** a production deploy completes
- **THEN** the release journal records time, environment, artifact digest, configuration provenance, and actor identity without storing secret values

### Requirement: Retention and access logging follow environment class

Production MUST enable the documented access-logging and backup-retention defaults. Preview MUST NOT pay for production retention by default. Operators MUST be able to export or point auditors at provider audit logs when the provider adapter supports them; when it does not, the capability MUST be typed unavailable.

#### Scenario: Preview skips production audit retention

- **WHEN** a preview environment is planned with default presets
- **THEN** effective configuration does not enable production-length audit retention, and the omission is visible in provenance

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
