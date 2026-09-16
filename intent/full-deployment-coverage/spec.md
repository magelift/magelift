---
status: superseded
slug: full-deployment-coverage
intent: intent.md
---

> HISTORICAL 2026-09-16: superseded by `intent/audit.md` (F13). Retained for
> reference; not approval of the rewritten draft scope (reference onboarding).
> Do not implement from this spec.

# Spec: every resource Magento needs, managed by Magelift (HISTORICAL)

## Requirements

What the system must do. Testable. Not file paths. One block per requirement,
each with at least one scenario.

### Requirement: Audit report with triage

The intent SHALL produce an audit enumerating every manual item on the AWS
deployment path, each triaged as deliberate-BYO or gap-to-close.

#### Scenario: Full triage, zero orphans

- **WHEN** the audit report is reviewed
- **THEN** every researcher-found manual item is dispositioned with file:line evidence
- **AND** each deliberate-BYO item cites its deciding doc or ADR plus the operator procedure
- **AND** each gap-to-close is either covered by this spec or filed as a follow-up stub (slug + one-line problem), leaving zero untriaged items

### Requirement: Managed SES sending on AWS

Deploy SHALL provision the SES identity, DKIM, and SMTP credentials, and
wire Magento SMTP from managed secrets, with no console steps.

#### Scenario: Verified identity with wired Magento SMTP

- **WHEN** a deployment requests managed SES for a verifiable domain
- **THEN** the SES identity verifies, DKIM records are created in the operator-supplied hosted zone, SMTP credentials land in a MageLift-managed secret, and Magento env receives host, port, username plus the secret reference
- **AND** sandbox-exit remains an explicit manual step (AWS support request), surfaced pre-deploy, not discovered at first send

### Requirement: Managed TEM sending on Scaleway

Deploy SHALL provision the TEM domain, validation, and SMTP credentials for
fr-par shops.

#### Scenario: Verified TEM domain with wired Magento SMTP

- **WHEN** a Scaleway deployment requests managed TEM (Essential tier default)
- **THEN** `scaleway.tem.Domain` + `DomainValidation` verify, SMTP credentials (Project ID + API secret) land in a managed secret, and Magento env is wired
- **AND** non-fr-par regions fail closed with the region limit named

### Requirement: Managed OVH mailbox sending

Deploy SHALL provision a sending mailbox on the operator's existing MX Plan
domain and wire it, with quota limits recorded.

#### Scenario: Mailbox created and wired with honest limits

- **WHEN** an OVH deployment requests managed email
- **THEN** `ovh.EmailDomainAccount` is created on the operator-supplied domain and Magento SMTP points at `smtp.mail.ovh.net:465`
- **AND** the ~200 mails/hour quota and no-bulk limit are recorded in the capability matrix, and the MX Plan service itself stays operator-supplied

### Requirement: Cloudflare and SendGrid stay manual

Cloudflare Email Sending and SendGrid SHALL stay documented manual steps
plus their `smtp` recipes: the Pulumi Cloudflare provider (v6.20.0) exposes
no sending-domain resource, and no stable SendGrid package exists (a
single-maintainer alpha does not qualify).

#### Scenario: No hand-rolled sending path

- **WHEN** Pulumi coverage is re-checked
- **THEN** both adapters stay out until a stable package exists; neither the Cloudflare API-only path (`emailSending.subdomains.create`) nor direct SendGrid API calls are hand-rolled behind the backends

## Design

How it fits the existing codebase: surfaces, data, APIs, ownership.

Adapters land in taxonomy-decided homes after order 16, one provider per
package, no shared email component. Config surface: the existing `ses` mode
plus new `tem` and `ovh` modes gain provisioning when requested (exact shape,
a `managed` block vs provision-by-default, is pinned at plan time against
the config tests); the Magento SMTP wiring they produce is unchanged. DNS
records go into operator-supplied zones only: zones themselves stay BYO.
Preview environments keep email `disabled` unless the audit shows a cheap
managed-preview path.

Sequenced after the lean-core trio (ROADMAP orders 16-18): taxonomy first
decides adapter homes, sever keeps core imports clean, and the module split
is unaffected (adapters stay in the root module).

## Gotchas / policy flags

Security, auth, PII, compatibility, contradictions the spec cannot satisfy.

- SES sandbox exit, OVH domain-service purchase, and TEM tier upgrades are
  human/API steps no adapter can close; each gets a procedure pointer, not
  silence.
- Secrets by reference only; Pulumi-created credentials flow through the
  same secret-ref validation as operator-supplied ones.
- Nothing is called managed until matrix + evidence say so; each adapter
  needs a live cell.
- Brownfield stays untouched: adopted resources are never mutated into
  managed ones.
- TEM is fr-par only; OVH sending is quota-bound mailbox SMTP, not a
  transactional API; both limits are user-visible in validation errors and
  the matrix, not buried in docs.

## Open questions carried forward

Unresolved items from intent.md, plus new ones. Each has an owner or a default.

- Managed email on preview/staging, or production-class only? Default:
  production-class only; previews stay `disabled`. Owner: implementer.
- TEM tier default (Essential pay-as-you-go vs Scale EUR 80)? Default:
  Essential. Owner: implementer.
- OVH: can the provider also create the email domain service, or only
  accounts on it? Default: accounts only until implementation proves
  otherwise. Owner: implementer.
- Non-email gaps (ACM provisioning, hosted zones, KMS, secret ARNs, log
  bucket, DNS cutover): follow-up intent(s) after this one ships. Default:
  one intent per gap cluster, sequenced on the ROADMAP. Owner: maintainer.
- ROADMAP placement: decided order 22, opening Phase 5. Owner: maintainer.
