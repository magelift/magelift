## Purpose

Specifies pluggable Magento transactional email: local SMTP testing, SendGrid, AWS SES, and equivalent provider adapters, with secrets kept out of YAML.

## ADDED Requirements

### Requirement: Email is a named mode with secret references

Configuration MUST select an email mode per environment or local overlay. Supported modes MUST include `disabled`, `smtp`, `sendgrid`, `ses`, and local `mailpit`. Provider-native equivalents (for example a GCP or Scaleway SMTP or API adapter) MAY be added as named modes when a source-dated adapter exists. Credentials MUST be secret references or environment-file entries, never plaintext YAML. MageLift MUST validate required fields for the selected mode before Magento is configured to send mail.

#### Scenario: SendGrid without an API key reference

- **WHEN** local or cloud configuration selects `sendgrid` and no credential reference is set
- **THEN** validation fails before Magento SMTP is written and names the required secret

#### Scenario: SES uses SMTP credentials as secrets

- **WHEN** an environment selects `ses` with host, port, username, and a credential reference
- **THEN** Magento's SMTP transport is configured from those values and the password is not written to YAML or logs

### Requirement: Local email does not claim cloud delivery

Local `smtp`, `sendgrid`, and `ses` modes MUST configure Magento's SMTP transport only. Delivery to an external provider MUST NOT be claimed as certified unless a live cell proves it. Local `mailpit` MUST pin a verified image, health check, and Magento SMTP wiring to a loopback sink; it MUST NOT be claimed as cloud delivery.

#### Scenario: Mailpit is selected locally

- **WHEN** `local.email.mode` is `mailpit`
- **THEN** Compose includes the pinned Mailpit service with a health check, Magento SMTP points at that sink, and output warns that this is not cloud delivery

### Requirement: Environment-specific email is allowed

Staging, production, and preview MAY use different email modes. Preview and development MUST default to non-production delivery (disabled, sink, or clearly sandboxed credentials). Production MUST NOT inherit a local Mailpit or open relay setting.

#### Scenario: Preview does not send real customer mail

- **WHEN** a preview environment omits email configuration
- **THEN** effective configuration uses a non-production default and does not reuse production SendGrid or SES credentials
