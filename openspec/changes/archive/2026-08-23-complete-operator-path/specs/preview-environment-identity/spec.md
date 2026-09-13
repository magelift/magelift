## ADDED Requirements

### Requirement: Previews stay Magelift-only for QA

QA MUST create, deploy, seed, and sweep preview environments with Magelift commands and the same YAML as local. Production credentials MUST NOT be the default preview secret set.

#### Scenario: PR preview from CI

- **WHEN** CI deploys a pull-request preview
- **THEN** Magelift uses preview identity flags, a preview-class catalog, and sweep or destroy on close
