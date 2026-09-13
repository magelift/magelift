## Purpose

Specifies operator database and media workflows: dump, retrieve, restore, tunnels, credential handling, and opt-in dump sanitization.

## ADDED Requirements

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

Media copy or sync MUST be a distinct command from database dump. It MUST copy only the Magento media ownership scope.

#### Scenario: Media sync is not dump

- **WHEN** an operator copies Magento media
- **THEN** MageLift uses a media-sync command distinct from database dump and copies only the Magento media ownership scope

### Requirement: Dump sanitization is opt-in and not certified anonymous

Default dumps MUST be labeled unsanitized Magento data. An explicit `--sanitize` flag MAY hash mailbox addresses in the dump. The sanitized output MUST still be labeled as not certified anonymous. MageLift MUST NOT claim dumps are anonymous PII-free copies.

#### Scenario: Default dump stays unsanitized

- **WHEN** a dump is created without `--sanitize`
- **THEN** output and docs label the dump as unsanitized Magento data

#### Scenario: Opt-in sanitization hashes mailboxes

- **WHEN** an operator passes `--sanitize` on `env dump`
- **THEN** mailbox addresses in the SQL are hashed, DEFINER hosts without a public domain are left unchanged, and the result is labeled as an email-hashed dump that is not certified anonymous
