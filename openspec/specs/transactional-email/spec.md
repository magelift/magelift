## Purpose

Specifies pluggable Magento transactional email: local SMTP testing, SendGrid, AWS SES, and equivalent provider adapters, with secrets kept out of YAML.

## Requirements

### Requirement: Email is a named mode with secret references

Configuration MUST select an email mode per environment or local overlay. Supported modes MUST include `disabled`, `smtp`, `sendgrid`, and `ses`. Provider-native equivalents (for example a GCP or Scaleway SMTP or API adapter) MAY be added as named modes when a source-dated adapter exists. Credentials MUST be secret references or environment-file entries, never plaintext YAML. MageLift MUST validate required fields for the selected mode before Magento is configured to send mail.

#### Scenario: SendGrid without an API key reference

- **WHEN** local or cloud configuration selects `sendgrid` and no credential reference is set
- **THEN** validation fails before Magento SMTP is written and names the required secret

#### Scenario: Cloud environment SendGrid is validated before KEEP Magento

- **WHEN** a fixed environment selects `sendgrid` with a secret reference
- **THEN** configuration validation succeeds without claiming certified delivery; certified delivery remains the session-1 GCP origin cell or typed unsupported

#### Scenario: SES uses SMTP credentials as secrets

- **WHEN** an environment selects `ses` with host, port, username, and a credential reference
- **THEN** Magento's SMTP transport is configured from those values and the password is not written to YAML or logs

### Requirement: SendGrid cloud delivery is an RC1 GCP-origin cell

RC1 MUST include a SendGrid integration that configures Magento SMTP from secret references on the packed GCP Magento origin. Cloud environment configuration MUST validate SendGrid and SES secret references before Magento SMTP is written. Certified SendGrid delivery MUST be proven on that origin or recorded as typed unsupported. AWS SES remains a named mode and MUST NOT be required to re-prove SendGrid.

#### Scenario: SendGrid on the GCP warm stack

- **WHEN** session-1 GCP Magento is KEEP-true and SendGrid credentials are secret references
- **THEN** Magento SMTP is configured from those references, a bounded delivery or typed-unsupported result is recorded, and no second Magento stack is created for email

### Requirement: Local email does not claim cloud delivery

Local `smtp`, `sendgrid`, and `ses` modes MUST configure Magento's SMTP transport only. Delivery to an external provider MUST NOT be claimed as certified unless a live cell proves it. RC1 SendGrid cloud delivery, when claimed, MUST use the packed GCP Magento origin cell. `mailpit` MUST remain unavailable until a pinned image and health contract exist; selecting it MUST fail before Compose is written.

#### Scenario: Mailpit is requested before it is verified

- **WHEN** `local.email.mode` is `mailpit` and the catalog has no verified Mailpit image
- **THEN** `dev init` or `dev up` fails before volume or network mutation and names `smtp` or `disabled` as the supported alternative

#### Scenario: Local SendGrid is not cloud certification

- **WHEN** local configuration selects `sendgrid` and only SMTP wiring is tested
- **THEN** docs and evidence MUST NOT claim certified cloud delivery

### Requirement: Environment-specific email is allowed

Staging, production, and preview MAY use different email modes. Preview and development MUST default to non-production delivery (disabled, sink, or clearly sandboxed credentials). Production MUST NOT inherit a local Mailpit or open relay setting.

#### Scenario: Preview does not send real customer mail

- **WHEN** a preview environment omits email configuration
- **THEN** effective configuration uses a non-production default and does not reuse production SendGrid or SES credentials

### Requirement: SendGrid live evidence does not multiply Magento stacks

Cloud SendGrid delivery evidence MUST reuse the packed Magento origin session. Preview MUST NOT inherit production SendGrid credentials. Secret references remain required for cloud SendGrid.

#### Scenario: One email does not justify a new shop

- **WHEN** SendGrid Magento delivery is the open claim
- **THEN** Magelift uses the existing Magento session or records the cell unproven
