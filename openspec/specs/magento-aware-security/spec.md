## Purpose

Specifies MageLift security defaults and Magento-aware WAF behavior so production edge does not break admin, checkout, APIs, or payment callbacks.

## Requirements

### Requirement: Secure defaults match actual risks

MageLift MUST apply least-privilege identities, secret references instead of plaintext, encryption in transit for operator and application paths, encryption at rest where the provider offers it for databases, object storage, and backups, private database endpoints, and SSH or exec access that is capability-advertised and attributable. Images MUST be digest-pinned. Supply-chain controls already required for signed builds (SBOM, provenance, Cosign) MUST remain in force for pushed artifacts. WAF and rate limiting MUST be part of production edge profiles. Backup artifacts MUST use the same ownership and encryption rules as other durable data. MageLift MUST NOT invent IAM or encryption claims the provider adapter cannot implement.

#### Scenario: Database is not public

- **WHEN** a production or staging environment is provisioned
- **THEN** the database endpoint is not reachable on the public internet, and credentials remain secret references

#### Scenario: Exec is attributable

- **WHEN** an operator runs `magelift exec` or `ssh` against a supported runtime
- **THEN** the session is capability-checked, arguments stay bounded, and output is redacted for known secret shapes

### Requirement: Production WAF is Magento-safe

A production edge profile that claims WAF protection MUST apply a Magento-safe policy, not an unmodified vendor CRS. The portable policy MUST account for Magento admin HTML and uploads, checkout, REST and GraphQL APIs, static and media paths, and payment or webhook callbacks. Body inspection MUST be large enough for Magento admin and catalog import traffic (64 KB on AWS CloudFront-class paths, or the provider equivalent). Unmodified `AWSManagedRulesCommonRuleSet`, Cloud Armor sensitivity 4, or generic CRS paranoia 2+ MUST NOT be reported as Magento-protected. A profile whose WAF is only attached in count mode MUST NOT be certified as protecting production traffic.

#### Scenario: GraphQL POST is legitimate Magento traffic

- **WHEN** Magento-safe WAF is applied and a typical Magento GraphQL POST is sent through the edge
- **THEN** the request is not blocked by default SQLi or XSS body rules, and evidence records that the Magento exceptions are active

#### Scenario: Vendor default CRS is rejected as Magento-ready

- **WHEN** a plan would attach unmodified Common Rule Set in block mode as the only WAF
- **THEN** validation or certification fails to call the cell Magento-protected and names `waf/magento-safe` or the provider equivalent

### Requirement: Webhooks and admin are not collateral damage

Rate limits and WAF exclusions MUST allow Magento payment callbacks, inbound webhooks, and admin paths that Magento requires. Admin MUST still be protected (restricted source, extra rule, or both) rather than left fully open. Changes to Magento-safe rules MUST have a data-plane test or an explicit unsupported mark; control-plane attach alone is not production evidence.

#### Scenario: Payment callback reaches Magento

- **WHEN** a documented payment or webhook path is exercised against a Magento-safe production edge
- **THEN** the callback returns Magento's expected status rather than a WAF 403, or the cell is marked failed rather than certified

### Requirement: Admin frontName drives WAF exclusions

Magento-safe WAF MUST use the configured admin `frontName` from Magento runtime YAML. Soak-then-block MAY be an evidence sequence. Count-mode-only attach MUST NOT be certified as Magento-protected.

#### Scenario: Custom admin is not /admin

- **WHEN** Magento-safe WAF is applied and YAML sets a custom frontName
- **THEN** admin HTML POST to that path is not blocked by default XSS body rules, and `/admin` is not treated as the admin panel
