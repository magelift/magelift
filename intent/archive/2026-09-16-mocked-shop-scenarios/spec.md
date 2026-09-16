---
status: done
slug: mocked-shop-scenarios
intent: intent.md
---

# Spec: synthetic scenario foundation (mocks prove contracts, not stores)

## Requirements

What the system must do. Testable. Not file paths. One block per requirement,
each with at least one scenario.

### Requirement: Original synthetic fixtures

Original synthetic fixtures SHALL exist under `tests/fixtures/synthetic/`:
a GCP Autopilot preview shape (alpha-recipe-shaped), an AWS Fargate preview
shape (routing coverage only), an `invalid/` set (each fixture expecting one
classified failure), synthetic ACC and Upsun importer inputs, and a README
stating scope and originality. All identities SHALL be example-only, with
zero secrets and zero bytes copied from private shops.

#### Scenario: Example-only identities

- **WHEN** the fixture tree is scanned for FQDNs, emails, domains, IPs, and
  cloud identifiers
- **THEN** every identity matches `*.example.test`, `*.example.invalid`, or
  an explicitly allowlisted documentation placeholder, and no secret-like
  assignment appears

#### Scenario: Invalid fixtures fail classified

- **WHEN** each `invalid/` fixture is loaded and validated
- **THEN** it fails with exactly its documented error class (unknown
  provider, unsupported version, missing required field) and never passes
  by silent substitution or defaulting

### Requirement: Offline layer proves routing, contracts, failures, validation

Layer 1 SHALL run with no cloud credentials, no spend, and no live calls. It
SHALL prove command routing (synthetic targets dispatch to the expected
in-process module/admission path), contracts (fake modules exercise
descriptor validation plus valid/invalid plan handling), deterministic
failures (expired preview refuses deploy, unmapped importer keys are
reported, unsupported versions are reported), and configuration validation
over the fixtures. The suite SHALL refuse to run when cloud credential
environment variables are present.

#### Scenario: Green with credentials stripped

- **WHEN** `go test -tags synthetic ./tests/synthetic/ -count=1` runs with
  all cloud credential env vars unset
- **THEN** it exits 0, covering routing, contract, failure, and validation
  cases with no network or cloud calls

#### Scenario: Refuses credentials

- **WHEN** the suite starts with any listed cloud credential env var set
  (AWS, GCP, OVH, Scaleway, plus the acceptance-harness TTL/KEEP markers)
- **THEN** it fails fast naming the offending variable instead of running

#### Scenario: Importer reports, never substitutes

- **WHEN** `magelift init --from-acc` and `--from-upsun` run on the synthetic
  importer fixtures
- **THEN** each exits 0, the emitted YAML validates, unmapped keys are listed
  explicitly, and unsupported versions are reported as errors — nothing is
  silently dropped or mapped onto a different version

### Requirement: Local layer adds planning signal without cloud

Layer 2 SHALL exercise local Magento application planning (localdev plan plus
compose template rendering) over the synthetic fixtures and assert offline
outputs (service images, environment wiring, queue/session/email shaping)
without starting containers and without Docker.

#### Scenario: Planning asserts hold offline

- **WHEN** the local layer runs on the synthetic GCP preview fixture
- **THEN** it asserts the planned application image, database/cache/search
  images, Magento environment wiring, and mailpit shaping from the rendered
  plan and template, with zero container or network operations

### Requirement: Gaps fail loudly and file back

Anything the scenarios cannot cover offline SHALL fail the run naming the
gap and its owning intent or procedure — never skip-and-green. Live Magento
data-plane proof SHALL be owned by `reference-store-acceptance`, never
claimed here.

#### Scenario: Unmockable step names its owner

- **WHEN** a scenario step declares a capability with no offline path
- **THEN** the run fails with `gap: <capability> owned by <intent>` drawn
  from the explicit gaps table, and the suite documents the per-step mock
  inventory (real code vs fake vs refused)

#### Scenario: No store-level claims

- **WHEN** the suite output and fixture README are reviewed
- **THEN** no line claims Magento installed, served assets, searched
  products, or recovered a database; every verdict is scoped to routing,
  contracts, failures, validation, or offline planning

## Design

How it fits the existing codebase: surfaces, data, APIs, ownership.

Fixtures plus a Go suite with the `synthetic` build tag under
`tests/synthetic/` (mirroring the `tests/floci/` tag pattern): config
load/validate over fixtures (real `internal/config` code), importer runs
(real `init --from-acc`/`--from-upsun` paths), registry dispatch with fake
`sdk.Module` implementations (contract fakes, in-process), localdev
Plan/ComposeTemplateFor asserts (local planning), and a hygiene gate over
the fixture tree. No Pulumi engine execution is reached, so no Pulumi mocks
are required; if a future scenario needs engine semantics it files back as
a gap per the requirement above. The suite refuses credentials via an
explicit env-var denylist checked before any case runs. The gaps table maps
known live-only capabilities to `reference-store-acceptance`; unknown
capabilities fail the run rather than passing silently.

Sequenced before the Magento safety and provider intents: later behavior
changes update this suite in their own changes (their specs own the updates);
this intent proves today's offline behavior honestly.

## Gotchas / policy flags

Security, auth, PII, compatibility, contradictions the spec cannot satisfy.

- Example-only domains enforced by the hygiene gate plus review; employer
  IDs/secrets never enter fixtures (`make check-clean-room` stays green).
- Fixtures model public default-stack shapes (current Adobe-listed Magento
  and service generations as documented in the matrix); pins are illustrative
  for routing, never store proof.
- Unsupported imported versions and services are reported, never silently
  substituted — asserted, not assumed.
- Fixing gaps found here belongs to the owning intent, not this one; this
  intent fails loudly and files back.
- Layer 1 and 2 verdicts never claim store-level proof; store proof belongs
  to bounded live acceptance on the shipped path.
- Human docs touched here go through humanizer, then remove-ai-marks.
- CI/Makefile wiring is out of scope; the suite runs via the documented
  `go test -tags synthetic` command (a CI-wiring suggestion rides in the
  report, not the implementation).

## Open questions carried forward

Unresolved items from intent.md, plus new ones. Each has an owner or a default.

- Fixture home adopted: `tests/fixtures/synthetic/` with recipe-shaped dirs.
  Owner: implementer (adopted).
- Runner form adopted: Go suite with the `synthetic` build tag for
  deterministic asserts. Owner: implementer (adopted).
- Importer coverage adopted: alpha-recipe GCP shape plus the ACC/Upsun
  importer inputs onboarding supports. Owner: spec author with
  `full-deployment-coverage` (adopted for fixtures; onboarding may extend
  the input set later).
- New: later intents changing offline behavior own this suite's updates in
  their own changes. Owner: each follower intent.
