## ADDED Requirements

### Requirement: Management UIs are optional and temporary

When a provider offers a management UI (database console, search dashboard, queue UI), MageLift MAY print a time-limited URL or tunnel. Access MUST use the same capability and secret-safety rules as exec. A missing UI MUST be typed unavailable, not an error that blocks logs or tunnels.

#### Scenario: Provider has no database UI

- **WHEN** an operator requests a database management UI on a target that has none
- **THEN** the command returns unsupported and still allows dump or tunnel commands that the target does support

#### Scenario: Operator requests env ui

- **WHEN** an operator runs `magelift env ui <environment> --target db-ui`
- **THEN** MageLift prints a session-only tunnel shape when the adapter has that UI, or exits 3 typed unavailable without blocking `env dump` or `tunnel`
