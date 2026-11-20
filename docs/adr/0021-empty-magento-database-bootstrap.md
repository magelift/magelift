# ADR 0021. Bootstrap an empty Magento database before the normal upgrade

Accepted on 2026-10-18.

On an empty Magento Open Source 2.4.9 database, `setup:upgrade` can validate
search before the default website exists. The deployment must initialize the
schema before running the usual upgrade.

The PHP lifecycle helper checks the selected database with `SHOW TABLES`.
When it finds zero tables, it runs Magento's `setup:db-schema:upgrade` and
`setup:db-data:upgrade`. The normal lifecycle follows. An existing database
starts at the normal upgrade. The helper preserves the configured encryption
key. The operator sets up Admin access separately.

A failed subcommand stops the migration. Diagnose a partial initialization
before retrying. Go renders the helper from the PHP lifecycle plan with
POSIX argument escaping so the runtime needs only the Magento image.

PHP tests cover both database states, subprocess failures and diagnostics.
A local run of the Go-rendered commands against MySQL, Valkey and OpenSearch
completed twice in the reference image and preserved the marker data and
crypt key. [Operate a shop](../operations.md)
