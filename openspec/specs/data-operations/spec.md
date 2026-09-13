## Purpose

Specifies operator database and media workflows: dump, retrieve, restore, tunnels, and credential handling, with sanitization left as later work.

## Requirements

### Requirement: Operators can dump and retrieve a database

MageLift MUST support creating a Magento database dump from a selected environment, retrieving that dump to the operator workstation, and restoring a dump into an appropriate target. Dump and restore MUST use secret references for database credentials and MUST NOT print passwords. Restore into production or protected environments MUST require `--yes`. Preview environments MAY receive sanitized or subset dumps; they MUST NOT be the default destination for an unsanitized production dump without confirmation.

#### Scenario: Retrieve a staging dump locally

- **WHEN** an operator requests a dump of `staging` and a local destination path
- **THEN** MageLift creates or fetches the dump over the documented access path, writes it only to the requested path, and does not log connection passwords

#### Scenario: Restore to production needs confirmation

- **WHEN** an operator restores a dump into a production or protected environment without `--yes`
- **THEN** the CLI exits 2 and does not start restore

### Requirement: Tunnels stay loopback-bound

Database, search, queue, and cache tunnels MUST bind to loopback unless a documented capability explicitly allows otherwise. Credentials MUST come from the environment's secret store. A provider that cannot offer a tunnel MUST return typed unsupported rather than opening a public listener.

#### Scenario: Database tunnel is local-only

- **WHEN** an operator starts a database tunnel for a supported target
- **THEN** the listener binds to loopback, credentials are not printed, and stopping the command closes the tunnel

### Requirement: Media sync is separate from database dump

Media copy or sync MUST be a distinct command from database dump. It MUST copy only the Magento media ownership scope. Sanitization or anonymization of personally identifiable Magento data MUST be treated as a future, explicitly specified capability; until it exists, MageLift MUST NOT claim dumps are anonymous.

#### Scenario: Sanitization is not implied

- **WHEN** a dump is created and no sanitization capability is certified
- **THEN** output and docs label the dump as unsanitized Magento data

### Requirement: Brownfield Magento cutover is specified as P2

When an existing network or database is attached, MageLift MUST still import Magento media and a dump through documented Magelift commands, keep crypt key as a secret reference, and run Magento `app:config:import` on deploy. Destroy MUST NOT own attached VPC or database resources.

#### Scenario: Attached RDS is not destroyed

- **WHEN** the operator destroys a Magento environment that attached an existing database
- **THEN** Magelift-owned children are removed and the attached database remains
