## ADDED Requirements

### Requirement: Vendor Magento cert cannot ride on DNS or NerdGraph-only cells

Live Cloudflare, Fastly, New Relic, and SendGrid cells in this campaign MUST attach to the packed GCP Autopilot Magento origin, MUST respect the $50 own-money cap shared with Scaleway, and MUST NOT remain wired overnight unless GCP KEEP is already set for Magento. A Cloudflare DNS cutover MUST NOT certify CDN or WAF. A Fastly docs-host origin MUST NOT certify Magento edge. A New Relic NerdGraph operations cell MUST NOT certify Magento APM. SendGrid MUST record Magento delivery or typed unsupported; config-only MUST NOT promote the prior Autopilot typed-unsupported row.

#### Scenario: Fastly docs origin is not Magento edge

- **WHEN** Fastly live evidence used a documentation host rather than Magento
- **THEN** Fastly stays experimental for Magento edge and this campaign does not promote it
