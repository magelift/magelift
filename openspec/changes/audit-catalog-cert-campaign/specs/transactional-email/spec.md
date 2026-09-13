## ADDED Requirements

### Requirement: SES and SendGrid config is not certified Magento delivery

`ses` and `sendgrid` modes MUST keep validating secret references and writing Magento SMTP from those references. This campaign MUST NOT mark SES delivery certified. SendGrid certified delivery MUST require Magento-origin delivery on the GCP KEEP cell within the $50 cap, or remain typed unsupported. The existing Autopilot typed-unsupported SendGrid result MUST NOT be rewritten as certified without a new run.

#### Scenario: AWS preview validates SES and does not certify mail

- **WHEN** the campaign AWS Fargate preview selects `ses` with secret references
- **THEN** Magento SMTP is configured and evidence does not mark SES Magento delivery certified
