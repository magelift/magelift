## Purpose

Specifies technical controls that help teams operating Magento under SOC 2 or ISO 27001. MageLift does not certify the customer's organization.

## ADDED Requirements

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

#### Scenario: Operator exports change evidence

- **WHEN** an operator runs `magelift evidence`
- **THEN** output includes artifact digest, actor, config digest, config provenance sources, and backup policy, and does not include secret values

### Requirement: Retention and access logging follow environment class

Production MUST enable the documented access-logging and backup-retention defaults. Preview MUST NOT pay for production retention by default. Operators MUST be able to export or point auditors at provider audit logs when the provider adapter supports them; when it does not, the capability MUST be typed unavailable.

#### Scenario: Preview skips production audit retention

- **WHEN** a preview environment is planned with default presets
- **THEN** effective configuration does not enable production-length audit retention, and the omission is visible in provenance
