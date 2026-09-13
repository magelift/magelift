## ADDED Requirements

### Requirement: SendGrid cloud delivery is an RC1 GCP-origin cell

RC1 MUST include a SendGrid integration that configures Magento SMTP from secret references on the packed GCP Magento origin. Cloud environment configuration MUST validate SendGrid and SES secret references before Magento SMTP is written. Certified SendGrid delivery MUST be proven on that origin or recorded as typed unsupported. AWS SES remains a named mode and MUST NOT be required to re-prove SendGrid.

#### Scenario: SendGrid on the GCP warm stack

- **WHEN** session-1 GCP Magento is KEEP-true and SendGrid credentials are secret references
- **THEN** Magento SMTP is configured from those references, a bounded delivery or typed-unsupported result is recorded, and no second Magento stack is created for email

#### Scenario: Cloud environment SendGrid is validated before KEEP Magento

- **WHEN** a fixed environment selects `sendgrid` with a secret reference
- **THEN** configuration validation succeeds without claiming certified delivery; certified delivery remains the session-1 GCP origin cell or typed unsupported

## MODIFIED Requirements

### Requirement: Local email does not claim cloud delivery

Local `smtp`, `sendgrid`, and `ses` modes MUST configure Magento's SMTP transport only. Delivery to an external provider MUST NOT be claimed as certified unless a live cell proves it. RC1 SendGrid cloud delivery, when claimed, MUST use the packed GCP Magento origin cell. `mailpit` MUST remain unavailable until a pinned image and health contract exist; selecting it MUST fail before Compose is written.

#### Scenario: Mailpit is requested before it is verified

- **WHEN** `local.email.mode` is `mailpit` and the catalog has no verified Mailpit image
- **THEN** `dev init` or `dev up` fails before volume or network mutation and names `smtp` or `disabled` as the supported alternative

#### Scenario: Local SendGrid is not cloud certification

- **WHEN** local configuration selects `sendgrid` and only SMTP wiring is tested
- **THEN** docs and evidence MUST NOT claim certified cloud delivery
