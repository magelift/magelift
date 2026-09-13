## ADDED Requirements

### Requirement: Magento-safe WAF is required to claim Magento protection

Native and Fastly production edge profiles that claim Magento WAF protection MUST implement the Magento-safe policy (admin, checkout, GraphQL, REST, static and media, payment callbacks, 64 KB-class body inspection or provider equivalent). Unmodified vendor CRS attach, count-mode-only attach, or Cloud Armor sensitivity 4 MUST NOT close a Magento-protected certification cell. Cloudflare MAY be used as a DNS adapter for tests on `acourtiol.com`; the portable core MUST NOT import a Cloudflare WAF schema just because that zone is used for acceptance.

#### Scenario: Magento-safe policy is the production default

- **WHEN** a production environment enables native WAF and omits a custom policy
- **THEN** the plan materializes Magento-safe rules, not unmodified Common Rule Set block mode

#### Scenario: Count-mode attach is not Magento-protected

- **WHEN** live evidence only shows WAF attached in count mode
- **THEN** the certification row may record attach proof and MUST NOT mark Magento traffic as WAF-protected
