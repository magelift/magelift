## ADDED Requirements

### Requirement: Local commands use the existing `dev` tree

Local Docker workflows MUST be exposed as `magelift dev init`, `dev up`, `dev down`, `dev status`, and `dev logs` (plus existing `dev seed` and `dev exec`). MageLift MUST NOT require a parallel `magelift local` command family. Local environments MUST approximate production Magento behavior for PHP, web, database, cache, search, and queue when those services are selected, without recreating managed cloud control planes. Local HTTPS MAY use a development CA on a documented loopback port.

#### Scenario: Operator starts local Magento

- **WHEN** a user runs `magelift dev init` then `magelift dev up` with a catalog-backed configuration
- **THEN** Compose starts the selected services, `dev status` reports them, and `dev logs` tails without requiring cloud credentials

#### Scenario: Local HTTPS on loopback

- **WHEN** the local web runtime publishes HTTPS
- **THEN** it listens on the documented loopback port with a development certificate and does not require a public DNS name

### Requirement: Mailpit is a verified local SMTP sink

`local.email.mode: mailpit` MUST write a digest-pinned Mailpit Compose service with a documented health check and Magento SMTP wiring to that sink. SMTP, SendGrid, SES, disabled, and Mailpit are the verified local modes. Mailpit MUST NOT be claimed as cloud delivery.

#### Scenario: Mailpit selected locally

- **WHEN** local email mode is `mailpit`
- **THEN** `dev init` writes the Mailpit service and Magento SMTP host/port, and `dev up --service app` enables the email Compose profile
