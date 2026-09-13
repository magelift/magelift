## Purpose

States what MageLift is, who it is for, which providers it covers, and how remaining work is prioritized so later specs are read against one product boundary.

## ADDED Requirements

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

MageLift MUST keep a provider-oriented architecture that can host first-party and later community-maintained providers. The initial infrastructure providers MUST be GCP, AWS, Scaleway, and OVHcloud. Additional integrations MUST include Cloudflare, Fastly, and New Relic. GCP MUST be the reference implementation. AWS MUST be the second provider used to prove and refine generic abstractions. Provider-independent commands MUST use generic domain concepts. Provider-specific behavior MUST be explicit capabilities, not hidden conditionals in generic commands. MageLift MUST NOT force every provider into a lowest-common-denominator feature set.

#### Scenario: A generic command hits an unsupported provider capability

- **WHEN** the user runs a provider-independent command whose selected target cannot perform the requested action
- **THEN** the CLI returns a typed unsupported or unavailable result naming the capability and provider, and does not pretend a weaker substitute succeeded

### Requirement: Priorities follow dependency order

Remaining work MUST be classified as P0, P1, P2, or P3, where P0 is the GCP reference vertical slice (install, init, configure, validate, create environment, provision, build, deploy, health, status/logs/access, update, destroy, plus the same flow non-interactively from CI and for preview environments), P1 is required before a stable production release, P2 is important expansion, and P3 is nice-to-have. Priorities MUST represent dependency order, not arbitrary importance. FrankenPHP worker mode MUST be P3 and MUST NOT block the nginx-fpm primary path.

#### Scenario: An agent asks for the next task

- **WHEN** an implementer reads `openspec/BACKLOG.md`
- **THEN** exactly one active implementation objective is named, it is the highest-priority incomplete work that is not blocked, and lower-priority rows stay listed without being started in parallel
