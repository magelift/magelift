## ADDED Requirements

### Requirement: Edge cert follows Magento origin and withheld Armor

CloudFront plus WAF Magento-safe claims MUST use Magento-origin traffic on the campaign AWS Fargate stack, not a docs host and not alias-HTTPS-only evidence. CloudFront alias HTTPS and GCP URL-map failover that used a non-Magento origin MUST stay bounded infra evidence. Cloud Armor Magento `requestBodiesToExclude` MUST stay withheld because GA import drops Magento exclusions; this campaign MUST NOT retry that as Magento-safe WAF. Cloudflare MUST be documented as DNS cutover only. EKS edge MUST stay deferred.

#### Scenario: Armor Magento exclusions stay withheld

- **WHEN** a campaign profile requests Cloud Armor Magento body exclusions
- **THEN** planning records withheld or typed unsupported and does not apply a GA import that drops `requestBodiesToExclude`
