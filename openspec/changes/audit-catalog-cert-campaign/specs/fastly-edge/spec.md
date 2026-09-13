## ADDED Requirements

### Requirement: Magento Fastly evidence attaches to GCP origin under the cash cap

Fastly Magento edge evidence in this campaign MUST use the GCP Autopilot Magento origin when the $50 own-money cap allows. Adapter TLS/purge against a docs host MUST remain experimental Magento edge. Magento-safe WAF on Fastly MUST follow `magento-aware-security` and MUST NOT be inferred from native CloudFront WAF evidence.

#### Scenario: Cap exhausted leaves Fastly experimental

- **WHEN** the $50 cap is already spent on Scaleway or other vendors
- **THEN** Fastly is not provisioned and stays experimental
