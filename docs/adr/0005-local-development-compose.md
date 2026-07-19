# ADR 0005: Keep local development in Docker Compose

## Status

Accepted

## Context

Local development must work without AWS credentials, Pulumi state, or a cloud
account. It still needs the same Magento capability names used by deployed
environments so configuration errors are found early.

## Decision

`magelift dev init` generates `.magelift/compose.local.yml` and an empty
mode-0600 `.magelift/local.env`. The default project
contains MySQL and Valkey. The `app` profile runs a pinned FrankenPHP classic image,
OpenSearch, and RabbitMQ, and bind-mounts the repository for source changes. The
`search` and `queue` profiles can be started independently when a contributor does
not need the app container. Host ports bind to loopback. `dev reset --yes` is the
only command that removes named volumes.

The FrankenPHP app exposes HTTP on port 8080 and an opt-in HTTPS listener on port
8443. The HTTPS listener uses Caddy's internal development CA and stores its
ephemeral state below `/tmp`; it exists for local secure-cookie and integration
testing only, not as production TLS evidence.

`magelift dev seed` is the supported local first-install path when the repository
already has Composer dependencies and `bin/magento`. It starts the app profile and
runs Magento's installer with local service defaults. The administrator password is
kept in `.magelift/local.env`, which is ignored by Git and restricted to the user.
The command is non-interactive and refuses to overwrite an existing
`app/etc/env.php`.

The Compose file is an execution context, not a second cloud provider. Its image
references can be replaced with project-approved digests through environment
variables, while deployed resources continue to come from the provider target.

## Consequences

This gives contributors a native CLI workflow and keeps service state local. It
does not reproduce AWS-managed failover, IAM, or network behavior; those checks stay
in Floci tests, Pulumi mocks, and sparse local AWS acceptance runs.
