# ADR 0021: Initial Magento database bootstrap

- Status: Accepted
- Date: 2026-10-18

## Context

On Magento Open Source 2.4.9 with an empty database, setup:upgrade validates
search before creating the default website. The reference recipe therefore
fails before schema creation, even with a reachable search service.

## Decision

The existing PHP deployment plan checks the effective database with SHOW TABLES.
Only a database with zero tables receives Magento's setup:db-schema:upgrade and
setup:db-data:upgrade before the normal lifecycle. Populated databases skip this
step. No tables are dropped, no administrator is created, and the configured
crypt key is preserved. Subcommand failures stop the migration without retry.
A partial initialization is preserved for diagnosis and repair.

The helper is self-contained PHP embedded through the existing lifecycle golden.
The Go adapters render that plan with POSIX argument escaping for inline PHP.
Runtime containers do not need the Composer build package. No YAML field,
manifest, provider RPC or separate installer is introduced.

## Verification

PHP regressions cover empty and populated databases, effective configuration,
subprocess failure, stream handling and error disclosure. The actual Go-rendered
shell completed twice inside the signed reference application image against
local MySQL, Valkey and OpenSearch: database status remained healthy, the marker
ID and crypt key were preserved, and no administrator was created. Live GCP
application and product persistence remain separate acceptance checks.
