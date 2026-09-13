## Purpose

Ensures every disposable certification run leaves no MageLift-owned billable resource, state object, secret, or temporary credential behind.

## Requirements

### Requirement: Unique ownership markers

Every live run MUST use a unique project prefix and provider ownership markers on all taggable resources. The cleanup scope MUST be derived from those markers, never from a broad account-wide delete.

#### Scenario: Two providers run on the same day

- **WHEN** AWS and GCP acceptance runs overlap
- **THEN** each cleanup operation sees only its own prefix and project identity

### Requirement: Destroy on normal and abnormal exit

The runner MUST attempt teardown on normal completion and on an interrupt after a stack has been created. A retained-debug mode MUST be explicit and MUST be visible in the evidence.

#### Scenario: Runner is interrupted after provision

- **WHEN** the process receives an interrupt after resource creation
- **THEN** it records the interrupted cell, attempts provider teardown, and runs the orphan assertion

### Requirement: Provider orphan assertion

After destroy, every provider adapter MUST query the resource classes it owns and fail if a matching acceptance prefix or ownership marker remains.

#### Scenario: Destroy leaves an asynchronous dependency

- **WHEN** a provider reports a resource still deleting
- **THEN** cleanup waits within a bounded timeout, retries the assertion, and records the remaining resource if it still exists

### Requirement: Protected and external resources

Cleanup MUST refuse to delete a resource unless its ownership marker was created by the run. Existing networks, user secrets, DNS zones, state buckets, and explicitly retained resources MUST be reported separately.

#### Scenario: Stack references an existing network

- **WHEN** a cell uses an existing network reference
- **THEN** cleanup leaves the network intact and verifies only MageLift-created resources

### Requirement: Credential cleanup

Temporary acceptance credentials, generated profiles, and local credential files MUST be removed or invalidated after the final run. Logs MUST not contain secret values.

#### Scenario: Temporary AWS identity is used

- **WHEN** the final AWS cell and cleanup assertion pass
- **THEN** the temporary IAM access key, user, and local profile are removed
