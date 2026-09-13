## Purpose

Defines a safe provider-neutral boundary for external edge and observability
services so MageLift can preserve migration intent without claiming lifecycle
support or certification that has not been proven.

## Requirements

### Requirement: External capabilities are explicit

Edge and observability choices MUST be represented as named capabilities with a
provider, lifecycle mode, credential references, and certification status.
Portable configuration MUST NOT require a Fastly, Datadog, New Relic, or other
vendor-specific object schema.

#### Scenario: A project selects Fastly with GCP below it

- **WHEN** the project requests Fastly edge and GKE as the origin runtime
- **THEN** the plan records Fastly and GCP as separate capabilities and keeps
  Fastly options under the edge extension boundary

### Requirement: Credentials remain references

External-service credentials MUST be accepted only through supported secret
references or provider-managed identity. They MUST NOT be written to generated
YAML, state output, evidence, logs, or extension diagnostics.

#### Scenario: A Fastly token is supplied through a secret store

- **WHEN** the edge adapter resolves the token for apply
- **THEN** the generated plan and evidence contain the reference and scheme but
  never the token value

### Requirement: Lifecycle evidence is required for certification

An external edge or observability capability MUST remain experimental,
unavailable, or blocked unless its adapter has live evidence for the declared
operations, credential path, routing or telemetry path, and exact cleanup. A
mock or import mapping alone MUST NOT close a live certification gate.

#### Scenario: Fastly routing and purge pass but adapter teardown is missing

- **WHEN** a disposable Fastly smoke test proves routing and purge but no
  registered MageLift adapter owns teardown
- **THEN** the capability remains experimental and the release gate stays open

#### Scenario: Native cloud logs are available

- **WHEN** a provider exposes native logs but no metrics or traces adapter exists
- **THEN** logs are reported as first-party capability and metrics/traces remain
  extension-owned or unavailable instead of being implied

### Requirement: Importers preserve external intent

ACC and Upsun importers MUST preserve recognizable edge and observability intent
as typed capability data or an explicit unmapped sidecar. They MUST NOT silently
drop Fastly configuration, vendor agents, purge policy, or telemetry settings.

#### Scenario: ACC includes bundled Fastly configuration

- **WHEN** an ACC project includes Fastly service or VCL intent
- **THEN** the import records supported fields under the edge capability and
  reports fields that require a provider extension

### Requirement: Cleanup ownership is narrow

External-service cleanup MUST use an exact run marker, service identity, domain,
subscription, or other ownership proof. It MUST leave pre-existing services,
domains, zones, and credentials untouched and report them separately.

#### Scenario: A pre-existing Fastly service is present

- **WHEN** a disposable run creates one marked service beside an unmarked service
- **THEN** cleanup removes only the marked service and records the unmarked one
  as preserved

### Requirement: Observability extensions use one vendor-neutral contract

An observability extension MUST receive the typed provider, signal selection,
endpoint, service identity, environment, labels, and credential references from
`sdk.ObservabilityIntent`. It MUST validate the requested signals before
mutation, report which signals it provisions or observes, return only redacted
outputs, and own teardown of resources created for its exact marker. Vendor
configuration MUST remain under the extension namespace rather than adding
vendor fields to core YAML.

#### Scenario: A project selects Datadog logs and metrics

- **WHEN** a Datadog extension receives the observability intent with logs and
  metrics enabled
- **THEN** it validates the credential references and endpoint, reports the
  exporter and signal capabilities in the plan, and keeps the token out of
  state, evidence, and diagnostics

#### Scenario: A requested signal is not implemented

- **WHEN** an extension receives traces but implements only logs and metrics
- **THEN** validation fails before mutation and names the unsupported signal

### Requirement: Registered edge lifecycle is explicit

The registered Fastly adapter MUST expose a side-effect-free plan and explicit
apply and destroy operations. Apply and destroy MUST persist or consume only
the exact service, domain, and ownership marker needed for recovery. The CLI
MUST NOT delete an existing service solely because it appears in configuration,
and an edge destroy failure MUST leave enough ownership state for a later retry.

#### Scenario: An operator applies an existing Fastly service

- **WHEN** `magelift edge apply` runs with an authenticated Fastly CLI profile
  and a configured service ID
- **THEN** it reconciles the configured domains through the current Domain
  Management API, optionally purges the service, and stores no token value

#### Scenario: A deployment uses Fastly in front of a cloud origin

- **WHEN** the origin deployment and health gates pass and the edge provider is
  Fastly
- **THEN** the deployment lifecycle applies the edge configuration and records
  its exact ownership marker; origin failure MUST NOT trigger an edge mutation

#### Scenario: Edge cleanup is retried after an origin destroy

- **WHEN** origin teardown succeeds but Fastly cleanup fails
- **THEN** the Fastly state remains available and a later explicit destroy can
  retry only the resources proven to belong to that run

### Requirement: External vendors use thin paid credits

Live Cloudflare, Fastly, New Relic, and SendGrid certification MUST assume thin paid credits, exact cleanup of DNS, CDN services, APM apps, and sender identities, and MUST NOT remain wired overnight unless KEEP is explicit and bounded.

#### Scenario: SendGrid sender identity is cleaned up

- **WHEN** a SendGrid live cell ends
- **THEN** evidence records cleanup of MageLift-owned sender or API resources and does not leave a billed sender identity unmarked
