## MODIFIED Requirements

### Requirement: Provider-oriented monorepo with a GCP reference path

MageLift MUST keep a provider-oriented architecture that can host first-party and later community-maintained providers. The RC1 infrastructure providers MUST be GCP, AWS, Scaleway, and OVHcloud. RC1 integrations MUST include Cloudflare, Fastly, New Relic, and SendGrid. GCP MUST be the reference implementation and the first live Magento origin. AWS MUST be the second certified Magento origin used to prove generic abstractions. OVHcloud and Scaleway MUST ship as complete first-party adapters in RC1 with Magento remaining experimental until a certified Magento cell exists. Provider-independent commands MUST use generic domain concepts. Provider-specific behavior MUST be explicit capabilities, not hidden conditionals in generic commands. MageLift MUST NOT force every provider into a lowest-common-denominator feature set.

#### Scenario: A generic command hits an unsupported provider capability

- **WHEN** the user runs a provider-independent command whose selected target cannot perform the requested action
- **THEN** the CLI returns a typed unsupported or unavailable result naming the capability and provider, and does not pretend a weaker substitute succeeded

#### Scenario: RC1 Magento certification bar

- **WHEN** release documentation describes `v1.0.0-rc.1` Magento certification
- **THEN** GCP GKE Autopilot and AWS ECS Fargate are the certified Magento origins, Cloudflare SendGrid New Relic and Fastly are claimed only against the GCP origin, and OVHcloud and Scaleway Magento remain experimental

### Requirement: Priorities follow dependency order

Remaining work MUST be classified as P0, P1, P2, or P3, where P0 is the GCP reference vertical slice (install, init, configure, validate, create environment, provision, build, deploy, health, status/logs/access, update, destroy, plus the same flow non-interactively from CI and for preview environments), P1 is required before a stable production release including the RC1 adapter set, P2 is important expansion, and P3 is nice-to-have. Priorities MUST represent dependency order, not arbitrary importance. FrankenPHP worker mode MUST be P3 and MUST NOT block the nginx-fpm primary path. `openspec/BACKLOG.md` MUST name exactly one active **cloud-provisioning** objective. Local CLI contracts that a packed live session will exercise (health layers, dump-from-live, rollback-vs-schema, SendGrid secret-reference validation) MUST exist with unit or fake-client proof before that session sets KEEP. Floci leftover API smokes and signed-provider-host work MUST NOT be sequenced after live sessions. Floci, unit, schema, signed-provider-host, and other non-provisioning tracks MAY proceed in parallel when they do not create paid cloud resources.

#### Scenario: An agent asks for the next task

- **WHEN** an implementer reads `openspec/BACKLOG.md`
- **THEN** exactly one active cloud-provisioning objective is named, it is the highest-priority incomplete live session that is not blocked, local contracts that session will exercise are listed as KEEP blockers when incomplete, and lower-priority cloud rows stay listed without being started in parallel

#### Scenario: A non-provisioning track is eligible

- **WHEN** Floci, fake-client, schema, or signed-provider-host work does not provision a paid cloud resource
- **THEN** that work MAY run while the named cloud objective is in progress and MUST NOT be treated as a step after session 3

#### Scenario: KEEP waits on local health layers

- **WHEN** runtime health checks lack a named layer field
- **THEN** packed session 1 does not set KEEP until that contract exists, rather than inventing layers while a Magento stack is live
