## Why

The existing RC1 plans cover the main compatibility catalog, evidence format,
skills, extensions, and Fastly intent, but the remaining work still has three
gaps that can produce misleading certification claims: the customer runtime
contract is not a single published behavior contract, warm-session reuse is not
specified independently from provider scripts, and third-party edge and
observability capabilities do not yet share one certification boundary.

## What Changes

- Define the customer-facing runtime contract for PHP, required extensions,
  Composer, credentials, release line, and edition.
- Define safe warm certification sessions with an input fingerprint, explicit
  cold-session boundaries, baseline migration ownership, and final teardown.
- Define provider-neutral edge and observability capability classification,
  including Fastly and third-party vendors without putting provider credentials
  or schemas in the portable core.
- Require the release gate to distinguish certified, experimental, unavailable,
  unsupported, blocked, and not-run cells using generated evidence.
- Add an implementation and certification checklist for the remaining
  Magento 2.4.6 through 2.4.9 service/provider matrix.

## Capabilities

### New Capabilities

- `runtime-requirements`: Customer-declared PHP, extension, Composer, release,
  edition, and secret-reference requirements.
- `warm-certification-sessions`: Fingerprinted reuse of compatible acceptance
  stacks and one final dependency-aware teardown.
- `external-service-certification`: Provider-neutral lifecycle and certification
  rules for edge and observability services.
- `certification-matrix`: Explicit cell dimensions, provider intersections,
  evidence thresholds, warm-session boundaries, cost reporting, and cleanup
  status for the release gate.

### Modified Capabilities

None. Existing compatibility, evidence, Fastly, skills, and extension changes
remain the source of their current contracts; this change fills the remaining
cross-cutting gaps before implementation resumes.

## Impact

The work affects configuration validation, the build request and evidence
protocols, acceptance checkpoint helpers, AWS/GCP/Kubernetes harnesses, provider
extension capability descriptors, importers for Adobe Commerce Cloud and Upsun,
release-readiness documentation, and the live certification matrix. It does not
authorize remote executable plugins or require a new cloud SDK.
