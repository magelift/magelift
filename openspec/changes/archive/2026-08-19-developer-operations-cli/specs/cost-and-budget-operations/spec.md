## ADDED Requirements

### Requirement: Cost output identifies the evidence class

Cost output MUST distinguish planned estimates, live price lookups, actual
spend, forecasts, and unavailable values. Each value MUST include its currency,
scope, source, observation time, and freshness when those facts are available.

#### Scenario: Only a plan estimate is available

- **WHEN** a provider cannot supply actual or forecast spend
- **THEN** the command labels the result as an estimate, reports the missing
  evidence explicitly, and does not present it as a billing total

#### Scenario: A live price request is stale or unavailable

- **WHEN** the provider price API fails or returns data older than the declared
  freshness budget
- **THEN** the report identifies the stale or unavailable value and returns a
  bounded diagnostic rather than silently using a different price source

### Requirement: Budget state is provider-backed or explicitly unavailable

A budget status MUST reference an owned provider budget or an explicit
unsupported result. It MUST include period, currency, limit, current or
forecast value, threshold state, and observation time when the provider exposes
them. A configuration field alone MUST NOT be reported as an enforced budget.

#### Scenario: An owned budget is readable

- **WHEN** the selected provider returns a budget owned by the configured
  MageLift scope
- **THEN** the command reports the limit, current or forecast spend, threshold
  state, and provider identity without exposing credentials

#### Scenario: A budget object is unowned or ambiguous

- **WHEN** a provider query finds a budget that lacks the MageLift ownership
  marker or cannot be mapped to one environment
- **THEN** the command refuses to mutate or claim the budget and reports the
  ownership conflict

#### Scenario: GCP exposes a project-scoped provider budget without MageLift ownership

- **WHEN** the configured GCP project has a budget whose filter contains that
  project and no other project
- **THEN** the command reports the budget as scope-verified, marks provider
  ownership and deployment enforcement separately, omits account-wide and
  multi-project budgets, and leaves actual and forecast spend unavailable when
  the selected billing API does not expose those values

#### Scenario: AWS exposes account-scoped cost budgets without environment ownership

- **WHEN** the configured AWS account returns cost budgets and notification
  thresholds through the read-only Budgets API
- **THEN** the command reports the account scope, currency, limit, percentage
  thresholds, and per-budget actual or forecast values when present, marks
  MageLift ownership and deployment enforcement false, and does not present an
  account budget as an environment-owned budget

#### Scenario: OVHcloud or Scaleway has no verified budget adapter

- **WHEN** `cost --budget` targets OVHcloud or Scaleway before an authenticated
  provider budget contract exists
- **THEN** the command returns a typed unavailable budget report and does not
  convert `monthlyBudgetCents` into a provider budget or enforcement claim

#### Scenario: GCP budget permission is missing

- **WHEN** the configured project is billing-enabled and the Budget API is
  enabled, but the authenticated principal cannot list budgets on the attached
  billing account
- **THEN** the command fails with the provider permission diagnostic, does not
  claim that no budget exists, and does not create or change a budget

### Requirement: Budget mutations are planned and confirmed

Any command that creates, updates, or deletes a provider budget MUST produce a
plan, require explicit confirmation, validate scope and currency, and use
provider-native idempotency and ownership checks. Failed or partial mutations
MUST include rollback or reconciliation instructions.

#### Scenario: A budget mutation targets an unsupported provider

- **WHEN** the selected provider has no verified budget adapter
- **THEN** the command fails before mutation with a typed unsupported result and
  does not create a local claim that a budget is enforced
