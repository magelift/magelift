---
status: superseded
slug: mocked-shop-scenarios
intent: intent.md
---

> HISTORICAL 2026-09-16: superseded by `intent/audit.md` (F14). Retained for
> reference; not approval of the rewritten draft scope (synthetic scenario
> foundation). Do not implement from this spec.

# Spec: mocked flagship end tests after all roadmap work (HISTORICAL)

## Requirements

What the system must do. Testable. Not file paths. One block per requirement,
each with at least one scenario.

### Requirement: Original flagship fixtures

Two fixture shops SHALL exist under `tests/fixtures/flagship/`: `b2b-aws`
(Terraform-shaped Magento 2.4.9 + ElasticSuite, staging + production) and
`m2-upsun` (Magento 2.4.8 + ece-tools platform shape with Fastly, Algolia,
SendGrid-default markers), modeled on the inventoried shapes with zero bytes
copied and zero secrets.

#### Scenario: Example-only identities

- **WHEN** the fixture tree is scanned
- **THEN** every FQDN, email, and domain matches `*.example.test` or `*.example.invalid`, and no other dotted quad or cloud identifier appears
- **AND** `b2b-aws` carries a parity checklist mapping each inventoried Terraform concern to its Magelift owner

### Requirement: Scenario 1 passes (AWS parity, both envs)

Mocked Magelift deploys of staging and production SHALL cover the full
Terraform parity checklist with Magento-healthy mock outputs.

#### Scenario: Staging and production resolve fully

- **WHEN** the mocked deploy runs per environment
- **THEN** every parity row resolves to a managed Magelift resource or an explicit deliberate-BYO with a procedure — zero silent gaps
- **AND** production additionally holds its gates (approval flag plus digest pin present)
- **AND** no Terraform file remains referenced except as the checklist's "replaced" column

### Requirement: Scenario 2 passes (Upsun to GCP, zero manual)

The importer SHALL convert the Upsun fixture to valid GCP-targeted YAML, and
the mocked GCP deploy SHALL cover all mapped services with an empty manual
list.

#### Scenario: Import converts cleanly

- **WHEN** `magelift init --from-upsun` runs on the fixture
- **THEN** it exits 0, the YAML validates, and `.unmapped.md` lists only the standing exclusions (Algolia-as-SaaS, custom PubSub datalake connector)

#### Scenario: Mocked GCP deploy needs nothing manual

- **WHEN** the mocked GCP deploy runs
- **THEN** MariaDB to Cloud SQL, Valkey to Memorystore, search, RabbitMQ, crons, mounts, routes, Fastly intent, and email (managed SES cross-cloud default) all resolve, with zero manual follow-ups

### Requirement: Mocked execution, zero cloud

Both scenarios SHALL run with no cloud credentials, no spend, and no live
calls.

#### Scenario: Green with credentials stripped

- **WHEN** run with all cloud credential env vars unset
- **THEN** both scenarios pass using only Pulumi mocks, Floci/floci-gcp, and local planning, per a documented mock inventory per step

### Requirement: Gaps file back, never silent

Anything the scenarios cannot cover SHALL fail the run with a named gap
owning intent or procedure — never skip-and-green.

#### Scenario: Unmockable step names its owner

- **WHEN** a scenario step has no mocked path
- **THEN** the run fails naming the gap and its owning intent (or deliberate-BYO procedure)

## Design

How it fits the existing codebase: surfaces, data, APIs, ownership.

Fixtures plus a Go suite (proposed `flagship` build tag beside `floci`)
driving config load/validate, the importer, Pulumi mock programs per
provider, and local compose planning. No Docker required: Magento-health
steps assert on mock outputs. The b2b "deleted Terraform" is modeled as the
parity checklist, never executed. Algolia stays SaaS and the custom PubSub
datalake connector stays app code, both by standing decision recorded in
the fixture README; datalake-side access stays operator-owned.
Migrated-shop email defaults to managed SES cross-cloud.

Sequenced after full-deployment-coverage (managed email), the migrate path,
and skills acceptance: the scenarios consume those capabilities and prove
them against realistic shops.

## Gotchas / policy flags

Security, auth, PII, compatibility, contradictions the spec cannot satisfy.

- Example-only domains enforced by grep gate plus PR review; employer
  IDs/secrets never enter fixtures.
- Catalog must cover the fixture pins (2.4.9/2.4.8, ece-tools,
  ElasticSuite, RabbitMQ 3.13, MariaDB 11.4, Valkey 8.0, PHP 8.3) or the gap
  files back per the gaps requirement.
- Version mapping (platform service versions onto catalog versions) is
  parity work, not assumed.
- Fixing gaps found here belongs to the owning intent, not this one; this
  intent fails loudly and files back.
- The custom PubSub datalake connector is app code, not infrastructure: the
  scenarios assert it is untouched, and any GCP IAM it needs files back as
  a gap rather than silently passing.

## Open questions carried forward

Unresolved items from intent.md, plus new ones. Each has an owner or a default.

- Runner form: Go suite vs script? Default: Go suite, `flagship` tag.
  Owner: implementer.
- Docker for health steps vs mock outputs? Default: mock outputs suffice.
  Owner: implementer.
- Pass judged scripted-only? Default: yes, deterministic asserts (unlike
  the human-judged skills run). Owner: implementer.
- ROADMAP: decided orders 23-24, closing Phase 5. Owner: maintainer.
- ElasticSuite-vs-native for scenario 1? Default: fixture maps to
  provisioned OpenSearch + ElasticSuite as inventoried. Owner: implementer.
