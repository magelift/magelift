## MODIFIED Requirements

### Requirement: Priorities follow dependency order

Remaining work MUST be classified as P0, P1, P2, or P3, where P0 is the Magento PHP operator path (install, init, `local`, configure, validate, create environment, provision, build, deploy, health, status/logs/access, update, destroy, plus the same flow non-interactively from CI and for preview environments), P1 is required before a stable production release including the RC1 adapter set, P2 is important expansion, and P3 is nice-to-have. Priorities MUST represent dependency order, not arbitrary importance. The default web runtime MUST be the `nginx-fpm` plugin. Additional HTTP servers MUST exist only as web-runtime plugins and MUST NOT be described as Adobe-supported unless Adobe's current table lists them for that Magento release. `openspec/BACKLOG.md` MUST name exactly one active objective. When no live Magento cell is open, that objective MAY be non-provisioning (local CLI, schema, runtime config). When a packed live session is open, the objective MUST be that cloud-provisioning session. Local CLI contracts that a packed live session will exercise MUST exist with unit or fake-client proof before that session sets KEEP. Floci, unit, schema, signed-provider-host, and other non-provisioning tracks MAY proceed in parallel when they do not create paid cloud resources.

#### Scenario: An agent asks for the next task

- **WHEN** an implementer reads `openspec/BACKLOG.md` and no Magento cell is running
- **THEN** exactly one active objective is named, it is the highest-priority incomplete work that is not blocked, and it MAY be non-provisioning

#### Scenario: Community Apache is in scope as a plugin

- **WHEN** release documentation describes HTTP frontends
- **THEN** it names nginx-fpm as the Adobe-aligned default and names Apache and FrankenPHP classic as independently versioned plugins that require `compatibility.allowUnsupported` on the current Magento line, not as silent aliases of nginx

#### Scenario: A non-provisioning track is eligible

- **WHEN** Floci, fake-client, schema, or signed-provider-host work does not provision a paid cloud resource
- **THEN** that work MAY run while the named objective is in progress

#### Scenario: KEEP waits on local health layers

- **WHEN** runtime health checks lack a named layer field
- **THEN** a packed live session does not set KEEP until that contract exists, rather than inventing layers while a Magento stack is live
