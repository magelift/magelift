## Purpose

Defines native cloud edge and Fastly edge delivery as composable capabilities
with secure origins, routing, TLS, cache, WAF, failover, rollback, and cleanup
evidence for every supported runtime architecture.

## ADDED Requirements

### Requirement: Edge capabilities are composable and explicit

An architecture MUST identify whether traffic uses provider-native edge,
Fastly, both in a defined chain, or no edge. The catalog MUST represent AWS
CloudFront and WAF, Google Cloud Load Balancing, Cloud CDN, and Cloud Armor,
and the current Scaleway or OVHcloud equivalents only when their official
capabilities and MageLift adapters support the requested behavior.

#### Scenario: Fastly fronts an AWS origin

- **WHEN** a profile selects Fastly in front of ECS and CloudFront is disabled
- **THEN** the plan records Fastly as the external edge, ECS as the origin,
  native AWS edge as disabled, and the exact routing, TLS, purge, WAF, and
  failover ownership boundaries

### Requirement: Edge failover and rollback have explicit lifecycle actions

The public edge contract MUST expose `apply`, `verify`, `failover`, `rollback`,
and `destroy` as independently declared adapter capabilities. A provider MAY
omit `failover` or `rollback` only when its descriptor and capability catalog
return a typed unavailable, unsupported, experimental, or blocked result;
intent or proof fields alone MUST NOT be treated as an executable lifecycle
implementation.

#### Scenario: A provider has no native failover translator

- **WHEN** an architecture requests edge failover on an adapter that declares
  only apply, verify, and destroy
- **THEN** planning rejects the requested capability before mutation and the
  certification row records the provider-specific reason

#### Scenario: GCP switches an owned two-origin URL map

- **WHEN** a GCP profile supplies an ownership-scoped primary and secondary
  backend service plus health references for both origins
- **THEN** apply verifies both backends, failover and rollback mutate only the
  owned URL map after the health gate, poll the Google control plane to
  convergence, and record control-plane evidence without claiming measured
  traffic convergence

#### Scenario: Scaleway multi-backend routing is not health failover

- **WHEN** a Scaleway profile requests the portable failover action
- **THEN** planning returns a typed unsupported capability because the current
  Edge Services multi-backend path is routing-oriented, and it performs no
  provider mutation

#### Scenario: Fastly mutates an existing service safely

- **WHEN** a Fastly apply or cleanup needs to change domains or versioned
  configuration on an existing service
- **THEN** the adapter clones an editable service version through its
  provider-specific SDK port, applies only ownership-scoped changes there,
  validates the version, activates it after the mutation, and restores the
  previous active version when post-activation verification fails; an adapter
  without that version port rejects the mutation before changing state

### Requirement: Origin health and routing are verified

Every edge profile MUST define origin discovery, health checks, headers,
timeouts, retry behavior, health-based routing, maintenance behavior, and
rollback. Edge apply MUST NOT route traffic to an origin that has not passed
the declared application and resilience health gates.

#### Scenario: A new origin is unhealthy

- **WHEN** an edge apply sees a failed origin health check
- **THEN** it leaves the current healthy route unchanged, reports the failed
  origin, and performs no destructive edge mutation

### Requirement: TLS, DNS, and certificate ownership are safe

Edge configuration MUST identify DNS ownership, certificate issuer, renewal
method, private-key reference, minimum TLS policy, hostname coverage, and
rollback behavior. Certificates and private keys MUST remain secret references;
DNS cleanup MUST never delete a pre-existing zone or record without ownership.

For current Fastly Unified Domain Management, a versionless domain MUST treat
managed-TLS verification and activation as a prerequisite to serving traffic.
The Fastly adapter MUST expose the provider-returned ACME DNS challenge as an
opaque provider result, while DNS record creation and deletion MUST remain in a
separately selected DNS adapter. The portable core MUST NOT import a Cloudflare
schema merely because the disposable acceptance harness uses the authenticated
Cloudflare CLI for the private test zone.

#### Scenario: Fastly managed TLS uses an external DNS adapter

- **WHEN** a Fastly versionless-domain profile requests managed TLS
- **THEN** the Fastly adapter returns the exact ACME challenge record and route
  CNAME, the selected DNS adapter creates only those ownership-scoped records,
  route verification waits for certificate and edge convergence, and teardown
  removes the TLS subscription before the versionless domain and DNS records

#### Scenario: Fastly account exposes only current Domain Management

- **WHEN** the classic service-domain API is unavailable but versionless Domain
  Management is available
- **THEN** the adapter uses the current versionless API, refuses to claim
  non-TLS versionless routing, and reports classic non-TLS support as a separate
  capability rather than silently switching APIs

#### Scenario: A certificate is near expiry

- **WHEN** an edge profile detects a certificate inside its renewal window
- **THEN** it reports the expiry and renewal owner, verifies the replacement
  certificate before cutover, and retains the previous certificate for the
  declared rollback period

### Requirement: Cache and purge behavior is testable

Each edge profile MUST define cache keys, bypass rules for authenticated or
dynamic traffic, stale behavior, invalidation scope, purge credentials, and
post-purge verification. A successful purge API call without an origin response
check MUST NOT close edge certification.

#### Scenario: A deployment changes product content

- **WHEN** the deployment completes and the profile requires cache invalidation
- **THEN** the edge adapter purges only the declared ownership scope and proves
  that a request receives the new artifact without bypassing the intended cache
  policy

### Requirement: Edge security is part of the profile

Native WAF, Fastly security policy, DDoS controls, origin authentication,
private-network restrictions, rate limits, and security headers MUST be
declared with their enforcement mode and evidence status. A profile MUST NOT
claim protected production edge when the policy is only planned or mocked.

#### Scenario: The origin endpoint is publicly reachable

- **WHEN** an edge profile requires origin restriction but the origin can be
  reached without the edge's authentication or network policy
- **THEN** the profile fails its security gate and remains non-production until
  the declared origin restriction is enforced

### Requirement: Edge failover and rollback are exercised

An HA or DR edge profile MUST test origin failure, edge control-plane failure
where practical, DNS or route rollback, and recovery to the authoritative
origin. The exercise MUST record traffic impact, detection time, recovery time,
and any stale or lost requests.

#### Scenario: The primary origin fails during a DR drill

- **WHEN** the declared health policy marks the primary origin unavailable
- **THEN** traffic follows the declared secondary or maintenance route, the
  application and data recovery checks pass, and the profile records measured
  RTO and rollback state

### Requirement: Edge lifecycle cleanup is ownership-scoped

Fastly and native edge resources created by a run MUST use exact ownership
markers and have plan, apply, destroy, retry, and direct-inventory behavior.
Pre-existing services, domains, zones, certificates, and WAF policies MUST be
preserved and reported separately.

#### Scenario: A Fastly account has an unmarked existing service

- **WHEN** a disposable edge test creates a marked service beside an unmarked
  service
- **THEN** cleanup removes only the marked service, leaves the existing service
  intact, and records both the cleanup result and preserved object

### Requirement: Edge support remains honest when no native equivalent exists

If a provider does not expose a required native edge capability or MageLift has
not implemented it, the profile MUST label that boundary unavailable,
experimental, or blocked. Fastly MAY be used as a separate external edge only
when its origin, credential, routing, and cleanup requirements are satisfied.

#### Scenario: Scaleway native CDN is not available for the profile

- **WHEN** a user requests native Scaleway CDN behavior that the provider
  capability catalog cannot verify
- **THEN** the plan reports native edge as unavailable and does not imply that
  a load balancer or DNS record provides CDN or WAF semantics
