## ADDED Requirements

### Requirement: Health is layered and doctor is local

`magelift health` MUST distinguish infrastructure, service, Magento application, dependency, and deployment checks when the adapter can observe them. An overall healthy result MUST NOT be returned if Magento is down while infrastructure is up. `magelift doctor` MUST check local project readiness (dependencies, configuration, catalog, Docker when required) and MUST NOT claim deployed environment health.

#### Scenario: Database is up and Magento is down

- **WHEN** runtime health sees a healthy database and an unhealthy Magento probe
- **THEN** the report marks Magento and overall status unhealthy or degraded, keeps the database check visible, and exits 4

#### Scenario: Doctor on a laptop

- **WHEN** an operator runs `magelift doctor` in a project with missing Docker
- **THEN** doctor reports the Docker capability as missing for local commands, still reports unrelated config checks, and does not query cloud runtime health

### Requirement: Logs cover application and infrastructure streams

Log commands MUST be able to select application, web, PHP, Magento, service, and infrastructure streams where the provider exposes them. Streaming or tailing MUST work for supported adapters. Filters remain fail-closed as already specified.

#### Scenario: Tail Magento logs

- **WHEN** an operator requests Magento or application logs with follow on a supported target
- **THEN** the CLI streams matching events until interrupted and redacts known secret shapes
