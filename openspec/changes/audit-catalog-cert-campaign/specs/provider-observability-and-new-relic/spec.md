## ADDED Requirements

### Requirement: CloudWatch, GCP ops, New Relic, and X-Ray are distinct claims

AWS CloudWatch and GCP logs/metrics graphs MUST NOT be called Magento three-signal certified without Magento-origin evidence. AWS X-Ray MUST be an `ObservabilityAdapter` plugin. Until that plugin is registered, X-Ray MUST be typed unavailable (IAM snippets on EKS MUST NOT count). After it exists, one Magento-origin X-Ray cell MAY run on AWS Fargate KEEP. New Relic Magento APM MUST require logs, metrics, and traces on the GCP Magento origin within the $50 cap; the 2026-08-13 NerdGraph operations cell and an ECS verifier that missed the trace window MUST NOT close APM certification.

#### Scenario: X-Ray has no plugin

- **WHEN** YAML or a campaign profile requests X-Ray traces and no X-Ray observability plugin is registered
- **THEN** validation or the runner records typed unavailable and does not claim native AWS traces

#### Scenario: X-Ray plugin on Fargate KEEP

- **WHEN** the X-Ray plugin is registered and Fargate Magento KEEP is healthy
- **THEN** evidence may record Magento-origin traces for that tuple only
