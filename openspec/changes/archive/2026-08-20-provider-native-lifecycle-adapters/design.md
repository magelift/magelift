## Context

The public SDK already owns provider-neutral intent, descriptor validation,
portable resilience-graph compilation, edge/observability lifecycle gates,
credential-reference validation, and normalized evidence. First-party Pulumi
modules own deployment graphs, while the current resilience packages expose
descriptors and accept an injected `ResilienceOperationClient`. The missing
piece is the provider-owned client construction and native API translation that
turns those seams into production lifecycle behavior. See `proposal.md` for
the motivation and `specs/provider-native-lifecycle-adapters/spec.md` for the
observable contract.

The design must preserve two identities that are easy to conflate:

```text
origin target: AWS / GCP / Scaleway / OVHcloud + runtime
external destination: Fastly or New Relic
```

Native destination adapters are target-owned. Fastly and New Relic are
independent lifecycle owners composed with an origin through the public typed
request. Their provider identities must not be rewritten to the origin.

## Goals / Non-Goals

**Goals:**

- Keep semantic ordering, safety gates, evidence, and cleanup in one core implementation.
- Keep provider SDK dependencies and native object models inside provider or external adapter packages.
- Make provider client construction explicit, injectable, cancellable, testable, and safe for community implementations.
- Give every architecture family an independent capability and evidence boundary.
- Reuse expensive resources only when a complete fingerprint proves it is safe.
- Make no live-support claim without a concrete client and matching evidence.

**Non-Goals:**

- No generic “lowest common denominator” cloud API that hides provider capability differences.
- No dynamic Go plugin loader or runtime loading of unsigned provider binaries.
- No migration compatibility layer for the removed singular edge or observability provider fields in the first release.
- No automatic alternate-provider recovery when identity, data semantics, network, edge, or ownership contracts are not proven.

## Decisions

### 1. Use narrow provider-side translators over the existing SDK ports

Each provider package will have constructors that accept narrow interfaces for
the operations it owns and return the existing SDK adapter. The provider
translator converts a semantic request into the official provider client call,
normalizes operation status, and returns only opaque identities plus normalized
proof facts. The core runner remains unaware of the native request/response
types.

The same pattern applies to resilience, edge, and observability. A target
module's planned factory receives the opaque validated plan and returns the
target-owned adapter. External Fastly and New Relic adapters are constructed
independently and are invoked with `TargetProvider` and `TargetRuntime` in the
typed plan request.

For S3-compatible Object Storage, the shared recovery package owns the
pagination, archive-copy, sealed-manifest, idempotency, restore, integrity,
ownership, encryption, and Object Lock policy algorithm behind a neutral
`S3ObjectAPI` port. AWS, Scaleway, and OVHcloud packages translate their
official SDK models to that port; provider-specific endpoints, credentials,
encryption headers, and retention fields remain at the provider edge. A new
S3-compatible community provider can therefore reuse the recovery algorithm
without importing an existing cloud package or creating a second core flow.

Alternative considered: put concrete provider clients in `internal/platform`.
Rejected because it couples the core to every SDK, makes community providers
depend on first-party imports, and encourages provider-specific branches in
the lifecycle engine.

### 2. Treat operation polling and cleanup as first-class provider ports

Native SDK calls often return asynchronous operation identities or eventually
consistent inventories. Provider translators will implement the existing
bounded polling and owning-service inventory ports. The core will own timeout,
cancellation, retry budget, checkpoint ordering, and the rule that delayed or
protected resources are not clean.

Alternative considered: let each provider implement its own wait loop.
Rejected because retry and cancellation behavior would diverge, and a provider
could accidentally report a plan or secondary index as cleanup truth.

### 3. Resolve credentials at the provider edge

Constructors accept validated credential references and a provider-owned
credential resolver/client factory. They never accept raw secret strings in
the public SDK. Provider API error text is redacted before normalization, and
all adapter outputs pass the shared secret-safety validator before checkpoint
or evidence writes.

Rotation and revocation use the existing credential lifecycle contract and
record only an operation or credential-version identity. A live acceptance
run is admitted only after the provider-specific credential reference is
resolved and the minimum permissions are verified.

Alternative considered: resolve all credentials in the core and pass a generic
credential map to adapters. Rejected because it leaks provider semantics into
the core and makes accidental secret logging more likely.

### 4. Keep capability declarations source-dated and evidence-gated

The catalog remains the provider-fact layer. It records official source URLs,
retrieval dates, service major, lifecycle, provider status, MageLift status,
and certification status. Profile planning may reject a missing or stale source
without contacting a provider. Live evidence can promote a row, but a
descriptor alone cannot.

The Valkey mapping remains semantic in the SDK (`valkey/9`); provider packages
map it to the documented API value. The GCP profile treats Memorystore Valkey
9.0 as the default GA target and 9.1 as Preview, and the acceptance matrix
keeps those profiles separate.

### 5. Use three certification lanes

Tests run in this order:

1. local semantic, schema, secret-safety, descriptor, factory, and mock-client tests;
2. provider contract tests against deterministic fake clients and Pulumi graph previews;
3. disposable live cells admitted by quota/credential/network/state/fixture/ownership checks.

The live scheduler builds one immutable artifact per compatible runtime
contract, creates only required cold baselines, and selects bounded warm
transitions from complete fingerprints. Provider groups can run concurrently
when mutation keys do not overlap. A shared state, fixture, backup, telemetry,
or edge identity serializes its mutation group. Every cell still receives its
own evidence record.

### 6. Keep native and external integrations separate

Native CloudWatch, Google Cloud Observability, Cockpit, OVH Logs Data Platform,
CloudFront/WAF, Google edge, Scaleway edge, and OVH edge clients are selected
by the origin target and its native provider intent. Fastly and New Relic are
selected by external provider intent and can be reused across multiple origins
only when their own ownership and fingerprint boundaries are unchanged.

This allows a future community edge or telemetry provider to implement the
same public factory without changing target modules or adding a provider field
to core configuration.

### 7. Generate documentation from records, never from inference

The documentation generator loads sealed JSONL evidence through the shared
reader, groups records by run, validates run cleanup, and reports the exact
inputs consumed. If no bundle exists, it renders policy/catalog coverage plus
an explicit evidence-gated state. Markdown narratives remain human context;
they cannot silently promote a live capability row.

## Risks / Trade-offs

- [Provider APIs change faster than the core contract] → isolate API mappings in provider packages, attach source dates to catalog records, and fail stale-source validation before mutation.
- [SDK clients return sensitive diagnostics] → redact at the provider boundary, validate normalized output, and reject unsafe evidence/checkpoints.
- [Eventual consistency makes cleanup look complete] → query the owning service, poll delayed tombstones with bounded budgets, and preserve protected/ambiguous identities as non-clean.
- [Warm reuse masks an untested architecture change] → include provider/runtime/service majors, resilience, observability, edge, artifact, fixture, migration, state, and ownership in the fingerprint.
- [Provider feature gaps are accidentally marketed as support] → require typed unavailable/unsupported results and evidence-derived documents; descriptors without concrete clients remain experimental.
- [Cloud-test cost and duration become unbounded] → use admission checks, one immutable artifact, fixture/session reuse, bounded concurrency, per-provider budgets, and durable checkpoints.
- [Fastly credentials or previously exposed session material are reused unsafely] → do not run live Fastly operations until credentials are rotated/revoked by the owner; use only replacement secret references and ownership-scoped disposable services.

## Migration Plan

1. Land the provider-side client contracts and factory wiring without changing the public semantic schema.
2. Implement and mock-test one provider/data-class boundary at a time, leaving catalog rows experimental until the concrete client is present.
3. Run local and provider-contract gates; only then admit disposable live cells.
4. Promote catalog rows and generated documentation only from sealed evidence with independent cleanup proof.
5. Roll back a provider implementation by disabling its factory/capability row; the deployment module remains available for preview or explicitly unsupported lifecycle paths.
