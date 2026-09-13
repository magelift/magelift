## ADDED Requirements

### Requirement: Health reports distinguish configuration from runtime evidence

The `health` command MUST return one normalized report shape for configuration,
stack-output, and runtime modes. The report MUST identify the environment,
provider/runtime target, observation time, source mode, overall status, and each
check's status and message.

#### Scenario: Configuration is valid but runtime evidence is unavailable

- **WHEN** configuration validation succeeds and the selected runtime adapter
  cannot read deployed health
- **THEN** the command reports configuration as healthy and runtime evidence as
  unavailable, exits with the documented unavailable status, and does not claim
  the environment is healthy overall

#### Scenario: A runtime check fails

- **WHEN** a provider adapter returns an unhealthy application, infrastructure,
  recovery, or edge check
- **THEN** the report preserves the failing check identity and source,
  summarizes the environment as unhealthy or degraded according to the shared
  policy, and returns a non-zero exit code

### Requirement: Log filters are effective or fail closed

Every runtime adapter that accepts `LogQuery.Filter` MUST apply an equivalent
provider-safe filter before returning events, or MUST return a typed unsupported
error without querying logs. The CLI MUST NOT silently ignore a non-empty
filter.

#### Scenario: Kubernetes logs use a filter

- **WHEN** an operator requests logs with a non-empty filter
- **THEN** the Kubernetes adapter returns only matching events, preserves the
  requested time window and limit, and has a contract test proving that a
  non-matching line is excluded

#### Scenario: A provider cannot translate a filter

- **WHEN** a provider has no safe equivalent for the requested filter syntax
- **THEN** the command returns an explicit unsupported result before provider
  log retrieval and explains the supported alternative

### Requirement: Log results are bounded and redacted

Log retrieval MUST enforce the requested limit and time window, define event
ordering and truncation, bound provider pagination and context time, and redact
known credential and token shapes from messages and provider metadata.

#### Scenario: One pod or stream fails

- **WHEN** a multi-pod or multi-stream query cannot read one source
- **THEN** the result records the partial-read status and source error without
  fabricating events, and the exit status distinguishes a partial result from a
  complete healthy read

#### Scenario: A workload pod has multiple containers

- **WHEN** an operator requests logs for a supported Kubernetes workload whose
  pod contains more than one container
- **THEN** the adapter selects the container whose name matches the workload
  before requesting logs, and does not issue an ambiguous provider request
