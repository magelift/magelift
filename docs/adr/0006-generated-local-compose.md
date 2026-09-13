# ADR 0006: Local development is generated Compose

- Status: Accepted
- Date: 2026-08-22

## Context

Contributors need Magento locally without AWS credentials or Pulumi state. The local stack must still use the same capability names as cloud so config errors show up early.

## Decision

`magelift local init` generates `.magelift/compose.local.yml` and a mode-0600 `.magelift/local.env`. Compose is an execution context, not a second cloud provider.

Default project: MySQL and Valkey. The `app` profile uses nginx + PHP-FPM by default, plus OpenSearch and RabbitMQ, and bind-mounts the repository. `frankenphp-classic` and `php-apache` require `compatibility.allowUnsupported`; they have no Adobe row and are not certified. `frankenphp-worker` is unregistered. `search` and `queue` can start alone. Host ports bind to loopback. `local reset --yes` is the only command that removes named volumes.

`magelift local seed` is the local first-install path when Composer dependencies and `bin/magento` exist. It is non-interactive and refuses to overwrite `app/etc/env.php`.

HTTPS on 8443 is a local listener for cookies and integration tests, not production TLS evidence.

## Consequences

Cloud failover, IAM, and network behavior stay in Floci, Pulumi mocks, and sparse live acceptance. Image references can be replaced with project-approved digests through environment variables.

## Alternatives considered

- Check in a hand-maintained Compose file: rejected (drifts from CLI capabilities).
- Local = mini AWS: rejected (needs credentials).

## Provenance

Original. Docker Compose public behavior.
