## Purpose

States what MageLift is, who it is for, which providers it covers, and how remaining work is prioritized so later specs are read against one product boundary.

## Requirements

### Requirement: MageLift is a bring-your-own-account Magento CLI

MageLift MUST be a CLI for Magento Open Source and Adobe Commerce developers and small teams with limited Cloud or DevOps expertise. It MUST let those users develop Magento locally, provision cloud infrastructure, deploy Magento, operate fixed environments, operate ephemeral branch or pull-request previews, run from CI, monitor and troubleshoot, access databases and internal services, manage backups and recovery, inspect cloud costs, and switch supported cloud providers without relearning a different product. MageLift MUST provision resources in the user's cloud account. It MUST NOT operate as a hosting service, proxy cloud billing, or add a resource markup.

#### Scenario: A Magento developer starts from an existing repository

- **WHEN** a user adds `magelift.yaml` to a Magento project and uses the documented first-run commands
- **THEN** the CLI is sufficient to validate configuration, build, provision, deploy, and inspect the selected environment on a certified target without editing Pulumi or Go

#### Scenario: MageLift is not a host

- **WHEN** documentation or CLI output describes MageLift
- **THEN** it identifies the user's cloud account as the billing and resource owner and does not claim MageLift hosts the shop

### Requirement: Familiar operator surface without cloning Adobe source

The developer experience MUST stay familiar to users of Magento Cloud CLI, Adobe Commerce Cloud, `ece-tools`, and Upsun CLI: YAML configuration, environment names, build then deploy, and day-2 logs, SSH, and database access. MageLift MUST implement that purpose with Go, the Pulumi Automation API, and Docker. It MUST NOT vendor `ece-tools`, Magento Cloud CLI, `magento-cloud-patches` databases, or Upsun source. Public Adobe and Upsun documentation MAY inform field shapes.

#### Scenario: An ACC operator imports a project

- **WHEN** the user runs the documented ACC or Upsun import
- **THEN** mapped fields become `magelift.yaml`, unmapped fields are reported with a non-zero exit, and no Adobe or Upsun source tree is copied into the repository

### Requirement: Provider-oriented monorepo with a GCP reference path

MageLift MUST keep a provider-oriented architecture that can host first-party and later community-maintained providers. The RC1 infrastructure providers MUST be GCP, AWS, Scaleway, and OVHcloud. RC1 integrations MUST include Cloudflare, Fastly, New Relic, and SendGrid. GCP MUST be the reference implementation and the first live Magento origin. AWS MUST be the second certified Magento origin used to prove generic abstractions. OVHcloud and Scaleway MUST ship as complete first-party adapters in RC1 with Magento remaining experimental until a certified Magento cell exists. Provider-independent commands MUST use generic domain concepts. Provider-specific behavior MUST be explicit capabilities, not hidden conditionals in generic commands. MageLift MUST NOT force every provider into a lowest-common-denominator feature set.

#### Scenario: A generic command hits an unsupported provider capability

- **WHEN** the user runs a provider-independent command whose selected target cannot perform the requested action
- **THEN** the CLI returns a typed unsupported or unavailable result naming the capability and provider, and does not pretend a weaker substitute succeeded

#### Scenario: RC1 Magento certification bar

- **WHEN** release documentation describes `v1.0.0-rc.1` Magento certification
- **THEN** GCP GKE Autopilot and AWS ECS Fargate are the certified Magento origins, Cloudflare SendGrid New Relic and Fastly are claimed only against the GCP origin, and OVHcloud and Scaleway Magento remain experimental

### Requirement: Priorities follow dependency order

Remaining work MUST be classified as P0, P1, P2, or P3, where P0 is the Magento PHP operator path (install, init, `local`, configure, validate, create environment, provision, build, deploy, health, status/logs/access, update, destroy, plus the same flow non-interactively from CI and for preview environments), P1 is required before a stable production release including the RC1 adapter set, P2 is important expansion, and P3 is nice-to-have. Priorities MUST represent dependency order, not arbitrary importance. The web runtime MUST be nginx-fpm only. `openspec/BACKLOG.md` MUST name exactly one active objective. When no live Magento cell is open, that objective MAY be non-provisioning (local CLI, schema, runtime config). When a packed live session is open, the objective MUST be that cloud-provisioning session. Local CLI contracts that a packed live session will exercise MUST exist with unit or fake-client proof before that session sets KEEP. Floci, unit, schema, signed-provider-host, and other non-provisioning tracks MAY proceed in parallel when they do not create paid cloud resources.

#### Scenario: An agent asks for the next task

- **WHEN** an implementer reads `openspec/BACKLOG.md` and no Magento cell is running
- **THEN** exactly one active objective is named, it is the highest-priority incomplete work that is not blocked, and it MAY be non-provisioning

#### Scenario: A non-provisioning track is eligible

- **WHEN** Floci, fake-client, schema, or signed-provider-host work does not provision a paid cloud resource
- **THEN** that work MAY run while the named objective is in progress

#### Scenario: KEEP waits on local health layers

- **WHEN** runtime health checks lack a named layer field
- **THEN** a packed live session does not set KEEP until that contract exists, rather than inventing layers while a Magento stack is live

### Requirement: Magento PHP storefront is first-class

`application.mode: integrated` MUST be a certified production path for Magento's own PHP storefront (Luma, Blank, custom Magento themes). Headless mode MUST remain API, CORS, admin, and media without deploying a JavaScript storefront.

#### Scenario: Integrated storefront uses Magento URLs

- **WHEN** an integrated environment is deployed on a certified target
- **THEN** Magento base URLs, cookies, static and media, and optional Varnish FPC apply to the Magento shop hostname

### Requirement: Paid integrations follow thin-credit discipline

Live Cloudflare, Fastly, New Relic, and SendGrid exercises MUST use the same scarce-credit rules as AWS, OVH, and Scaleway. GCP KEEP MAY retain slow-to-provision Magento origins. Vendor live tests MUST attach to an existing packed GCP session when the vendor is the claim.

#### Scenario: SendGrid does not spawn a second Magento origin

- **WHEN** SendGrid delivery evidence is required
- **THEN** the runner attaches to the packed GCP Magento session or records the cell unproven, and does not provision a second Magento stack only to send mail
