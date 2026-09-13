## Purpose

Defines projects, fixed environments, preview environments, inheritance, naming, state, locking, drift, and lifecycle so operators can run Magento without reinventing environment identity per provider.

## Requirements

### Requirement: Project and environment are distinct objects

A project MUST have one configuration document that names the application, default target provider and runtime, and defaults. Each environment MUST have a name, a provider association (inherited or overridden), an environment class, and optional overlays. Environment classes MUST include at least `preview`, `development`, `staging` or `uat`, and `production`. User-defined environment names (for example `staging`) MUST map to a class; MageLift MUST NOT require those exact names.

#### Scenario: Staging inherits project defaults

- **WHEN** a project sets provider, region, and preset defaults and `environments.staging` overrides only account and domain
- **THEN** effective configuration for `staging` uses the overlays, retains provenance for every value, and does not require copying the full project document

### Requirement: Environment class selects capacity and safety defaults

Presets MUST follow environment purpose: preview is cost and speed optimized, development favors developer productivity, staging and UAT are production-like where useful, production favors reliability, security, HA, and DR. Preview MUST NOT receive production-grade HA or DR by default. Production MUST NOT silently use preview-only safety defaults for backups, WAF, or deletion protection.

#### Scenario: Preview disables expensive HA

- **WHEN** an environment uses the preview class and omits HA overrides
- **THEN** planning selects the documented preview catalog, does not provision multi-AZ HA or production backup retention, and records those omissions in effective configuration

#### Scenario: Production keeps deletion protection

- **WHEN** a production-class environment is planned
- **THEN** database deletion protection and the documented backup retention are enabled unless YAML explicitly overrides them, and destroy still requires `--yes`

### Requirement: Fixed and preview environments share one lifecycle contract

Environment create, provision, build, deploy, validate, update, and destroy MUST use the same generic commands for fixed and preview environments. Preview identity, idempotent repeat deploys, and ownership-scoped cleanup remain as specified in `preview-environment-identity`. Fixed environments MUST persist until explicitly destroyed. Preview environments MUST support automatic or explicit destroy.

#### Scenario: Repeat deploy on a fixed environment

- **WHEN** an operator runs the normal non-interactive deploy command a second time against an existing staging environment with the same artifact digest
- **THEN** the command is idempotent, health is re-checked, and the environment identity does not change

#### Scenario: Preview destroy is ownership-scoped

- **WHEN** a preview environment is destroyed
- **THEN** only resources matching the preview ownership marker and generation are deleted, and unmarked account resources remain

### Requirement: Naming, tags, state, locking, and drift are explicit

Resource names and tags or labels MUST include a MageLift ownership marker and environment identity. Mutating commands that need exclusive access MUST take the documented environment lock, fail if the lock is held, and support a guarded unlock for stale locks. Deploy and destroy MUST be idempotent for the same desired state. When the CLI can observe drift from the last applied configuration, it MUST report it; it MUST NOT silently rewrite live resources onto an undocumented shape.

#### Scenario: Concurrent deploys

- **WHEN** two mutating commands target the same environment at once
- **THEN** one holds the lock and proceeds, the other fails with a lock-held error naming the holder, and neither leaves a half-applied unmarked stack

#### Scenario: Drift is reported

- **WHEN** status or preview detects that live resources no longer match the last applied configuration
- **THEN** the command reports drift as a distinct state and does not claim the environment is in sync

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
