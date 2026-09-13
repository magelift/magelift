## MODIFIED Requirements

### Requirement: One command tree across providers

The CLI MUST expose a single `magelift` command tree for initialization, configuration, validation, environment create/list/status/destroy, deployment, logs, SSH or shell access, tunnels, database operations, service access, costs, budgets, local development, CI generation, diagnostics, and version or upgrade. Command names MUST stay consistent with the tree (`init`, `config`, `env`, `deploy`, `preview`, `health`, `doctor`, `logs`, `ssh`, `exec`, `tunnel`, `cost`, `local`, `ci`, `version`). Laptop Docker MUST be `magelift local`. MageLift MUST NOT expose `magelift dev`. Provider-specific flags MUST NOT appear on generic commands unless the selected target advertises that capability.

#### Scenario: Help lists the operator surface

- **WHEN** a user runs `magelift --help`
- **THEN** the listed commands include `local` and do not list `dev` as a command, and do not present a second provider-specific CLI as the primary interface

#### Scenario: A reserved command is not yet implemented

- **WHEN** a user invokes a reserved command that this release has not implemented
- **THEN** the CLI exits 3, names the command, and does not mutate configuration or cloud state

#### Scenario: magelift dev is not a command

- **WHEN** a user runs `magelift dev`
- **THEN** the CLI exits as an unknown or reserved command and does not start Compose

### Requirement: Actionable errors

Error output MUST name the failed action, the environment and provider when known, and a next step the operator can take (missing flag, missing capability, permission, retry, or unsupported). Compatibility and catalog failures MUST name which authority rejected the combination: Adobe, MageLift, or the selected provider. MageLift-experimental warnings MUST name MageLift and MUST NOT fail the command. Errors MUST NOT print secret values, credential files, or Composer authentication material.

#### Scenario: Missing cloud credential

- **WHEN** a mutating cloud command runs without usable credentials for the selected provider
- **THEN** the CLI fails before mutation, names the provider and the login or federation command to run, and does not print credential contents

#### Scenario: Catalog rejection names the authority

- **WHEN** validation rejects an Adobe-unsupported service or a provider-unavailable product
- **THEN** the error names Adobe or the provider respectively, and does not print secret values

#### Scenario: Experimental cell warns without failing

- **WHEN** validation sees only a MageLift-experimental cell
- **THEN** the command succeeds, prints a warning that names MageLift, and does not print secret values

## ADDED Requirements

### Requirement: Operators never type Pulumi on the happy path

Mutating Magento and infrastructure commands MUST drive the Pulumi Automation API internally. Help, doctor next-steps, and user skills MUST NOT document `pulumi up` as the operator path. `doctor --install-dependencies` MAY install a Pulumi executable only after confirmation and allowlisted argv.

#### Scenario: Deploy without a Pulumi CLI recipe

- **WHEN** a Magento PHP developer runs `magelift deploy --env staging --yes` on a certified target
- **THEN** the command completes or fails as Magelift, and the documented next step is not a Pulumi CLI invocation

### Requirement: Audit and evidence are distinct reports

`magelift evidence` MUST remain the production change journal (digest, actor, config provenance, backup policy, no secret values). Control-posture output MUST be a distinct `audit` report or an evidence subcommand that exports a control matrix plus evidence pointers. The two MUST NOT be the same blob.

#### Scenario: Security reviewer exports controls

- **WHEN** a production-class environment is audited
- **THEN** the report lists encryption, IAM, logging, backup, WAF, and residency controls with pointers and contains no secret values

### Requirement: Operators can purge edge cache

The CLI MUST expose a Magelift command that invalidates or purges the selected environment's Magento-facing CDN or native edge cache when the target advertises that capability. A provider that cannot purge MUST return typed unsupported.

#### Scenario: On-call purges CloudFront or native edge

- **WHEN** the operator runs the documented purge command for a certified environment
- **THEN** Magelift issues the provider purge and does not require a cloud console recipe
