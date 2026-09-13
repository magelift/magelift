## ADDED Requirements

### Requirement: SendGrid live evidence does not multiply Magento stacks

Cloud SendGrid delivery evidence MUST reuse the packed Magento origin session. Preview MUST NOT inherit production SendGrid credentials. Secret references remain required for cloud SendGrid.

#### Scenario: One email does not justify a new shop

- **WHEN** SendGrid Magento delivery is the open claim
- **THEN** Magelift uses the existing Magento session or records the cell unproven
