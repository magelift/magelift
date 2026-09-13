## MODIFIED Requirements

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

## ADDED Requirements

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
