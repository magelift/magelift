## Purpose

Defines the CLI contract operators and CI rely on: predictable commands, non-interactive use, exit codes, output formats, and explicit destructive actions.

## ADDED Requirements

### Requirement: One command tree across providers

The CLI MUST expose a single `magelift` command tree for initialization, configuration, validation, environment create/list/status/destroy, deployment, logs, SSH or shell access, tunnels, database operations, service access, costs, budgets, local development, CI generation, diagnostics, and version or upgrade. Command names MUST stay consistent with the existing tree (`init`, `config`, `env`, `deploy`, `preview`, `health`, `doctor`, `logs`, `ssh`, `exec`, `tunnel`, `cost`, `dev`, `ci`, `version`). MageLift MUST NOT require a `local` alias when `dev` already covers local Docker workflows. Provider-specific flags MUST NOT appear on generic commands unless the selected target advertises that capability.

#### Scenario: Help lists the operator surface

- **WHEN** a user runs `magelift --help`
- **THEN** the listed commands include the operator surface above and do not present a second provider-specific CLI as the primary interface

#### Scenario: A reserved command is not yet implemented

- **WHEN** a user invokes a reserved command that this release has not implemented
- **THEN** the CLI exits 3, names the command, and does not mutate configuration or cloud state

### Requirement: Non-interactive and CI-safe execution

When `--no-interaction` is set, or when stdin is not a terminal and confirmation would be required, the CLI MUST NOT prompt. It MUST fail closed if a destructive or mutating action needs confirmation and `--yes` is absent. Global `--output` MUST support `table`, `json`, and `yaml`. JSON and YAML output MUST be parseable and MUST NOT interleave logs on stdout; diagnostics belong on stderr.

#### Scenario: CI deploy without a TTY

- **WHEN** CI runs `magelift deploy --env staging --no-interaction` with valid credentials and configuration
- **THEN** the command completes or fails without reading stdin, writes machine-readable result according to `--output`, and writes diagnostics to stderr

#### Scenario: Destroy without confirmation

- **WHEN** a user runs destroy or another documented destructive command on a production or protected environment without `--yes`
- **THEN** the CLI exits 2, explains that `--yes` is required, and performs no mutation

### Requirement: Documented exit codes

The CLI MUST use these exit codes unless a command documents a stricter mapping: 0 success, 1 general failure, 2 usage or validation error, 3 unavailable or not implemented, 4 unhealthy or degraded runtime health. A command that reports both a payload and a non-zero status MUST still write the payload when the documented contract says the report is the result.

#### Scenario: Health is unhealthy

- **WHEN** `magelift health` gathers runtime evidence and a check is unhealthy or degraded
- **THEN** the command writes the health report and exits 4

#### Scenario: Invalid configuration

- **WHEN** `magelift config validate` finds a schema or compatibility error
- **THEN** the command exits 2, names the field or combination, and does not plan or mutate infrastructure

### Requirement: Actionable errors

Error output MUST name the failed action, the environment and provider when known, and a next step the operator can take (missing flag, missing capability, permission, retry, or unsupported). Errors MUST NOT print secret values, credential files, or Composer authentication material.

#### Scenario: Missing cloud credential

- **WHEN** a mutating cloud command runs without usable credentials for the selected provider
- **THEN** the CLI fails before mutation, names the provider and the login or federation command to run, and does not print credential contents
