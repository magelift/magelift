## ADDED Requirements

### Requirement: Multi-website hostnames are first-class

A Magento project MUST be able to declare more than one shop hostname (websites or stores) with cookie scope and TLS names. Preview, staging, and production overlays MUST be able to replace those hostnames without copying the whole project document.

#### Scenario: Two websites on one Magento

- **WHEN** YAML lists two website hostnames for a production-class environment
- **THEN** planning records both names for certificates and Magento base URLs and does not require a second Magelift project

### Requirement: Agencies isolate clients by project file

Each client MUST use a distinct Magento project configuration and cloud account or project binding. MageLift MUST NOT share secret references, state backends, or ownership markers across those projects by default.

#### Scenario: Two clients in one agency

- **WHEN** two Magento repositories each have a `magelift.yaml` with different accounts
- **THEN** deploy of one MUST NOT read or write the other's secret references or stack state
