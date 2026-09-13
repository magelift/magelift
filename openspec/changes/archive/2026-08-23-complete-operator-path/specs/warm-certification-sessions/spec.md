## ADDED Requirements

### Requirement: Vendor live tests reuse packed Magento sessions

Warm sessions MUST allow Cloudflare, Fastly, New Relic, and SendGrid attach onto an existing packed GCP Magento fingerprint when only the vendor path changes. A new Magento origin MUST NOT be created solely for those vendors.

#### Scenario: Fastly attaches to KEEP Magento

- **WHEN** a packed GCP Magento session is live and Fastly evidence is still open
- **THEN** the runner may add Fastly to that session and records the vendor as a warm or attach cell, not a new Magento baseline
