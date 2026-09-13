## Purpose

Keeps the published CLI a thin host: YAML, Cobra, admission, registry, and plugin RPC. Magento HTTP, cloud graphs, edge, mail, search, queue, and telemetry evolve as independently versioned plugins behind generic `sdk/v1` ports.

## ADDED Requirements

### Requirement: Core has no provider special cases

`cmd/magelift` and `internal/cli` MUST NOT import Pulumi SDKs or construct provider clients (`internal/cloud/<provider>/`). Commands MUST resolve a registered extension and call `sdk/v1` ports (`Module`, optional `EdgeAdapterFactory`, `ObservabilityAdapterFactory`, `CollectorDeploymentAdapterFactory`, `PlanAdmissionFactory`, `ResilienceAdapterFactory`, and the new `WebRuntime` port). Topology MUST stay inside `internal/cloud/<provider>/`. Shared components MUST NOT branch on `if provider ==`.

#### Scenario: Cleanup uses a port

- **WHEN** a leftover-backup or cleanup command runs
- **THEN** it calls a registered module or cleanup port and does not construct a GCP or AWS client in the Cobra package

### Requirement: Plugins version independently of magelift version

Every first-party and community capability MUST expose `ExtensionDescriptor` identity, semantic version, and `ExtensionAPIVersion`. A plugin bugfix MUST ship without a core CLI tag when the API version matches. First-party adapters MAY compile into the default binary for RC1 DX but MUST still register on the same registry as subprocess plugins. Community adapters MUST load only as signed HashiCorp go-plugin subprocesses. Go `plugin.Open` MUST NOT be used.

#### Scenario: Apache plugin bumps without CLI bump

- **WHEN** `php-apache` descriptor version increments and `ExtensionAPIVersion` is unchanged
- **THEN** `magelift version` MAY stay the same and `magelift extensions` shows the new plugin version

### Requirement: Plan and lifecycle preserve caller context and identity

`Module.Plan` and adapter factories MUST take the caller's `context.Context`. They MUST NOT switch to `context.Background()` for planning. Returned `ModulePlan` MUST keep request `Provider`, `Runtime`, `Region`, environment class, protection, and image digest; a mismatch MUST fail before mutate. A subprocess provider MUST return an executable plan the host can run, not only a kind string.

#### Scenario: Planning is cancelled

- **WHEN** the operator context is cancelled during `Plan`
- **THEN** the plugin returns before provider mutation and does not continue on a detached background context
