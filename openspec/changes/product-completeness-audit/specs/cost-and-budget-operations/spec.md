## ADDED Requirements

### Requirement: Unexpectedly expensive resources are visible before apply

Cost planning MUST flag catalog choices that are known to be expensive relative to the environment class (for example managed multi-AZ brokers on preview, provisioned search on a disposable cell). The flag MUST use provider price evidence when available, otherwise a documented qualitative warning. MageLift MUST NOT invent a precise bill.

#### Scenario: Preview would create Amazon MQ

- **WHEN** a preview plan selects a managed HA broker whose price class is documented as production-scale
- **THEN** `preview` or `cost` reports that choice as unexpectedly expensive for preview and does not present it as a cheap default

### Requirement: Preview budgets stay preview-scoped

Budget configuration for preview environments MUST not copy production limits as if preview were already production. Threshold warnings MUST use the provider budget API when it exists and MUST be labeled unavailable when it does not. Creating or changing a budget remains planned and confirmed as already specified.

#### Scenario: Preview environment has no production budget

- **WHEN** `cost --budget` runs against a preview environment with no preview-owned budget
- **THEN** the command reports no environment-owned budget or an explicit unavailable adapter, and does not display the production budget as that preview's limit
