## ADDED Requirements

### Requirement: Admin frontName drives WAF exclusions

Magento-safe WAF MUST use the configured admin `frontName` from Magento runtime YAML. Soak-then-block MAY be an evidence sequence. Count-mode-only attach MUST NOT be certified as Magento-protected.

#### Scenario: Custom admin is not /admin

- **WHEN** Magento-safe WAF is applied and YAML sets a custom frontName
- **THEN** admin HTML POST to that path is not blocked by default XSS body rules, and `/admin` is not treated as the admin panel
