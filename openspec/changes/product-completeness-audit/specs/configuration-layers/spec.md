## ADDED Requirements

### Requirement: Configuration schema is versioned and migrated explicitly

`schemaVersion` MUST be required. Unknown fields in core-owned sections MUST fail loading. Deprecated fields MUST fail or emit a migration instruction; they MUST NOT be silently ignored. `magelift config migrate` MUST rewrite a supported older schema to the current one without dropping secret references. Backwards-compatible additive fields MAY appear in a minor release; removing or renaming a field MUST bump the schema version and ship a migrator.

#### Scenario: Unknown core field

- **WHEN** YAML contains an unknown field in a core-owned section
- **THEN** `config validate` exits 2 and names the field

#### Scenario: Migrate keeps secret references

- **WHEN** an operator runs `magelift config migrate` on a supported older document that uses secret references
- **THEN** the new document validates at the current schema version and still contains those references, not secret values
