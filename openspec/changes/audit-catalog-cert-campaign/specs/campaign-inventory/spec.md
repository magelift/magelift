## Purpose

Pins the do-not-forget list for `audit-catalog-cert-campaign`: plugin-first core, Adobe hatches, money, packed AWS matrix on remaining credits, architecture-review release blockers, addon honesty, and named product gaps. Exceeding a cash cap or shipping a fail-open lock MUST stop the release. It MUST NOT mint a certified badge from smoke.

## ADDED Requirements

### Requirement: Named budget envelope is fail-closed

This campaign MUST stay inside the named envelope. ~$8 of ~$160 AWS credits is already spent; remaining AWS credits plus the Bedrock/Lambda promo ~$40 MUST fund a packed Magento matrix, not a second Cartesian shop. GCP Autopilot KEEP remains the thorough GCP Magento path. OVH first-month credits (~$200) remain one MKS preview. Own money MUST NOT exceed $50 total across Scaleway, Cloudflare, SendGrid, Fastly, and New Relic. Bedrock playground and Lambda web-app promo tasks MUST run first to unlock the extra AWS credits and MUST NOT produce Magento evidence or matrix rows.

| Source | Amount (spec time) | Allowed use | Forbidden use |
| --- | --- | --- | --- |
| OVH first-month credits | ~$200 | One MKS preview; Magento if the SKU can run it, else infra-honest experimental | Production-size HA; OVH native CDN |
| AWS credits | ~$152 remaining of ~$160, plus ~$40 after Bedrock/Lambda promo | Packed KEEP Magento matrix: ECS Fargate, ECS Managed Instances, EKS, `rds-mysql`→`aurora-mysql`, `ecs-rabbitmq`, `amazon-mq`, EKS RabbitMQ, OpenSearch (serverless then provisioned only as a warm/cold fingerprint allows), HA, CloudWatch, X-Ray plugin once it exists, Magento-origin CloudFront on an existing origin | Cartesian N shops; Magento Commerce without a licensed artifact; claiming cert without Magento-origin evidence |
| AWS Bedrock / Lambda promo | the extra ~$40 | Account credit quest **first**, outside Magento evidence | Magento cells, matrix rows, edge/APM/mail claims |
| Scaleway | no free credits | One smallest Kapsule preview from the own-money cap | Magento certified badge; Valkey claims (managed cache is Redis) |
| Cloudflare, SendGrid, Fastly, New Relic | no free credits | Attach to the GCP Autopilot Magento origin only, from the own-money cap | Second Magento origin; overnight KEEP; DNS-only or NerdGraph-only as vendor Magento cert |
| Own money | max $50 total | Scaleway + the four vendors above | A second paid Magento stack |

#### Scenario: Bedrock promo is not Magento evidence

- **WHEN** an operator completes the Bedrock playground or Lambda web-app credit quest
- **THEN** no Magento evidence file is written and the capability matrix does not gain a row

#### Scenario: Own-money cap is exhausted

- **WHEN** Scaleway plus vendor attach spend would exceed $50
- **THEN** remaining vendor cells are recorded typed unsupported or experimental and are not provisioned

### Requirement: AWS packed matrix is thorough and non-Cartesian

After the promo credits land, AWS MUST set `MAGELIFT_AWS_ACCEPTANCE_KEEP=true` for this campaign only. The runner MUST reuse one Magento artifact digest. Cold boundaries MUST be compute families: ECS Fargate, ECS Managed Instances (`computeMode: managed-instances`), EKS. Warm transitions on a live family MUST cover catalog cells: `catalog.queueMode` `db` / `ecs-rabbitmq` / `amazon-mq`; EKS `queueMode` `database` / `rabbitmq`; `searchMode` `disabled` then serverless then provisioned OpenSearch only when the fingerprint allows; `databaseEngine` `rds-mysql` then `aurora-mysql`; HA / multi-AZ as an explicit fingerprint; CloudWatch Magento-origin signals; X-Ray only after an `ObservabilityAdapter` plugin exists. Default omitted standard/HA `queueMode` in product YAML MUST remain `ecs-rabbitmq` (**BREAKING**); the campaign MUST still select `amazon-mq` explicitly. Destroy plus orphan check MUST run when the AWS KEEP campaign closes. OVH and Scaleway MUST NOT set KEEP.

#### Scenario: Amazon MQ is a warm cell on Fargate KEEP

- **WHEN** the Fargate Magento origin is healthy with KEEP set and the profile sets `queueMode: amazon-mq`
- **THEN** the runner updates the same stack instead of creating a second Magento origin, and evidence names Amazon MQ as that cell only

#### Scenario: EKS is a cold compute boundary

- **WHEN** the campaign needs EKS Magento plus EKS RabbitMQ
- **THEN** it uses a separate EKS KEEP prefix from Fargate and does not treat Fargate health as EKS certification

### Requirement: Magento editions and HTTP plugins stay honest

Live Magento cells MUST use Magento Open Source 2.4.8-p5 or 2.4.9. Magento Commerce live cells MUST stay out unless a licensed artifact is supplied later. Default `application.webRuntime` MUST be `nginx-fpm`. `frankenphp-classic` and `php-apache` on 2.4.8-p3+ / 2.4.9 MUST fail closed without `compatibility.allowUnsupported` and MUST hatch-warn when that flag is set. Docs MUST say Adobe lists nginx only on that train; FrankenPHP has no Adobe row. `frankenphp-worker` MUST fail closed. This campaign MUST NOT certify FrankenPHP or Apache.

#### Scenario: Commerce artifact is absent

- **WHEN** a campaign profile requests Adobe Commerce and no licensed artifact is configured
- **THEN** the runner refuses before mutate

#### Scenario: FrankenPHP without hatch is refused

- **WHEN** Magento 2.4.9 YAML sets `application.webRuntime: frankenphp-classic` without `compatibility.allowUnsupported`
- **THEN** validation fails closed and names the Adobe hatch

### Requirement: Certified Magento is evidence-dimensioned

`CertificationTier` MUST follow the evidence tuple (provider, runtime, compute mode, Magento release, catalog services, preset), not a static `Module.CertificationTier()` of `certified` for all ECS or all Autopilot. Only existing evidenced Magento claims MAY remain certified until new files ship: AWS ECS Fargate Magento 2.4.9 preview (not custom-domain TLS, not HA/DR, not Managed Instances) and GCP GKE Autopilot evidenced runtime cells (not GKE Standard, not HA, not release-wide Adobe cache intersection). EKS, Managed Instances, Aurora apply, Amazon MQ, provisioned OpenSearch Magento search, HA, X-Ray, FrankenPHP, Apache, OVH MKS, Scaleway Kapsule MUST stay experimental unless a new Magento evidence file for that exact tuple ships.

#### Scenario: Autopilot HA is not auto-certified

- **WHEN** YAML selects GKE Autopilot HA without a matching evidence file
- **THEN** the CLI warns experimental (or fails closed if the cell is unavailable) and does not inherit the preview Autopilot certified badge

### Requirement: Architecture-review defects are release blockers

First public Magento release MUST NOT ship while: (1) GCP `NewGCS` / client errors become a no-op deploy lock; (2) lock `Release` uses `context.Background()` or deletes without owner/generation match; (3) lock-release errors are dropped when a primary error exists; (4) `Module.Plan` uses `context.Background()` or returns a plan whose region, class, protection, or digest disagrees with the request; (5) a provider subprocess returns only a kind string the host cannot execute; (6) Cobra constructs provider clients; (7) `make lint` fails (gofmt / staticcheck). X-Ray MUST be an observability plugin before any X-Ray Magento cell. Homebrew/Scoop stay after the first public core tag.

#### Scenario: Unlocked Autopilot deploy is refused

- **WHEN** GCS state client construction fails during certified Autopilot deploy
- **THEN** deploy exits before Magento mutate

### Requirement: Addon claims match Magento-origin evidence

CloudWatch and GCP ops graphs MUST NOT be called Magento three-signal certified without Magento-origin evidence. X-Ray MUST stay typed unavailable until the plugin exists, then Magento-origin traces on the AWS KEEP origin. New Relic MUST NOT be promoted from NerdGraph ops-only. CloudFront Magento-safe WAF MUST NOT be newly certified from non-Magento traffic. Cloud Armor Magento `requestBodiesToExclude` MUST stay withheld. Fastly docs-host origin MUST NOT certify Magento edge. Cloudflare MUST remain DNS cutover, not CDN/WAF. SES MUST remain config validation unless Magento-origin delivery exists. SendGrid MUST remain delivery or typed unsupported.

#### Scenario: X-Ray requested before the plugin

- **WHEN** a campaign profile enables AWS X-Ray and no `ObservabilityAdapter` registers X-Ray
- **THEN** planning records typed unavailable and does not claim traces

### Requirement: MageLift is not the customer's SOC 2 issuer

Documentation, CLI help, and `magelift audit` MUST describe reconstructable controls and evidence pointers with no secret values. They MUST NOT state that the customer is SOC 2, ISO 27001, or GDPR certified.

#### Scenario: Audit export is not a certificate

- **WHEN** an operator runs `magelift audit` after this change
- **THEN** the output lists controls and pointers and does not claim the organization is SOC 2 or ISO 27001 certified

### Requirement: Named product gaps stay named until closed

The catalog MUST keep stating until a later change closes them: Magento websites/stores/views are Magento-owned; importer emits `application.magento` overlays (`frontName`, cookies, CORS, consumers) in this change; no Cloud SQL / OVH / Scaleway brownfield DB attach; SQS and Pub/Sub are Magento-module transports plus `composer.lock`; OVH native CDN is fail-closed; Scaleway managed cache is Redis not Valkey; split Valkey cache/session is AWS non-preview; RabbitMQ classic↔quorum refuse stays on EKS/GKE; unsigned `plugin.Open`; freelance `magelift deploy`/`destroy` on non-acceptance accounts.

#### Scenario: Operator looks for websites YAML

- **WHEN** a user searches schema or docs for a MageLift `websites` array
- **THEN** they are pointed at Magento scope configuration rather than a MageLift website list
