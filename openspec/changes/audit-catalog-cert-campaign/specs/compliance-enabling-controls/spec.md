## ADDED Requirements

### Requirement: Campaign docs do not imply a customer certificate

Human pages touched by this change, generated CLI help, and `magelift audit` MUST keep stating that MageLift provides reconstructable controls useful to SOC 2 / ISO 27001 programs and does not certify the customer. They MUST NOT claim GDPR certification. `magelift audit` MUST remain a control-posture export with pointers and no secret values.

#### Scenario: FAQ answers SOC 2

- **WHEN** an operator asks whether MageLift is SOC 2 certified or certifies their shop
- **THEN** docs say MageLift does not certify the customer and point at `magelift audit` plus provider audit logs
