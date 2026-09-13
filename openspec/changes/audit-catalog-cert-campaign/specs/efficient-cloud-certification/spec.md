## ADDED Requirements

### Requirement: Packed cells prefer update over recreate

On AWS remaining credits, the runner MUST KEEP one origin per compute family and warm-update catalog cells. It MUST NOT open a new Magento shop per queue, search, or database SKU. OVH and Scaleway MUST still be one preview create, in-place updates, and a single destroy. Cartesian Magento version × SKU matrices MUST be refused.

#### Scenario: Amazon MQ reuses the Fargate origin

- **WHEN** a campaign profile requests `amazon-mq` after Fargate Magento KEEP is healthy
- **THEN** the runner updates the same stack and does not create a second Magento origin

### Requirement: Campaign spend stays inside named credit and cash caps

The runner and operator runbook MUST treat OVH first-month credits (~$200) as one MKS preview; remaining AWS credits plus Bedrock/Lambda promo cash as the packed AWS KEEP matrix; and Scaleway plus Cloudflare/SendGrid/Fastly/New Relic as a shared $50 own-money cap. Bedrock/Lambda promo tasks MUST NOT be billed as Magento cells.

#### Scenario: Scaleway share of the cash cap is consumed

- **WHEN** the Scaleway Kapsule preview is paid from the $50 cap
- **THEN** remaining vendor attach cells MUST fit in what is left or be recorded typed unsupported
