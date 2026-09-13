## Purpose

Adds an explicit Fastly edge target and preserves Fastly-related migration intent without leaking provider credentials or raw VCL into portable MageLift configuration.

## ADDED Requirements

### Requirement: Fastly edge intent

The edge model MUST represent Fastly service identity, domains, TLS intent, purge behavior, and VCL or policy references as typed fields or secret references.

#### Scenario: User selects Fastly for an imported project

- **WHEN** a migrated configuration contains Fastly edge intent
- **THEN** validation preserves the intent and identifies the required Fastly configuration

### Requirement: Secret references only

Fastly API tokens, service credentials, and TLS private keys MUST be accepted only as secret references and MUST NOT be written to generated configuration or evidence.

#### Scenario: Import contains a plaintext token

- **WHEN** an importer sees a plaintext Fastly token
- **THEN** it rejects or redacts the value and reports that a secret reference is required

### Requirement: Preview and ownership

Fastly preview MUST be non-mutating. Apply and destroy MUST operate only on resources owned by the selected MageLift project and MUST expose a cleanup result.

#### Scenario: Preview is requested

- **WHEN** the user runs preview with Fastly selected
- **THEN** MageLift validates the edge plan without changing the Fastly account

### Requirement: Migration field reporting

ACC and Upsun importers MUST preserve supported Fastly intent and report unmapped or operator-owned edge settings with stable diagnostics.

#### Scenario: Import contains custom VCL

- **WHEN** the source contains VCL that MageLift cannot translate
- **THEN** the importer writes a redacted reference and a clear manual follow-up diagnostic

### Requirement: Certification honesty

Fastly MUST remain experimental until a real-account acceptance cell proves configuration, TLS, purge, request routing, and teardown.

#### Scenario: Only mock coverage exists

- **WHEN** Fastly passes provider mocks but no real account cell exists
- **THEN** the capability matrix labels it experimental rather than certified
