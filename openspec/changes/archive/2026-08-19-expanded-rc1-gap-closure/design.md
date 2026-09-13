## Context

See `proposal.md` for the motivation. MageLift already has typed build fields,
release compatibility checks, generated certification evidence, checkpointed AWS
and GCP scripts, and an explicit public extension boundary. The live runs show
that the expensive part is managed-service startup and deletion, while the
largest certification risk is confusing topology evidence with a complete
release or provider claim.

## Goals / Non-Goals

**Goals:**

- Make the effective Magento runtime contract visible from configuration through
  build and certification evidence.
- Turn the existing checkpoint behavior into a provider-independent warm-session
  contract with clear cold-session boundaries.
- Keep Fastly and third-party observability behind the existing extension
  boundary and give the release gate one honest classification model.
- Make the remaining matrix work measurable and safe to run on paid credits.

**Non-Goals:**

- Implement a remote executable plugin loader.
- Claim Fastly production certification from the existing disposable smoke.
- Provision Datadog, New Relic, or another vendor without a provider adapter and
  cleanup test.
- Reclassify OVH or Scaleway as certified without live Magento evidence.

## Decisions

### 1. Keep runtime requirements in the existing typed build contract

The project already uses `build.php`, `build.extensions`, and
`build.composer.version`, and the build protocol carries those values into the
isolated builder. The implementation should extend validation, importer
metadata, and evidence sealing around that contract rather than introduce a
second runtime configuration tree.

### 2. Reuse the existing checkpoint fingerprint as the session identity

The acceptance scripts already archive incompatible checkpoints and support
stable work directories. The shared fingerprint helper should become the single
source for warm-session identity. Provider scripts may add provider-specific
fields, but they must not bypass the common release, artifact, schema, and
cleanup inputs.

### 3. Separate baseline migration from service transitions

A warm session has one migration owner. Later cells use infrastructure-only
updates and health checks. This preserves the useful cost reduction without
allowing two migration jobs to race or allowing a service-only result to look
like a fresh application certification.

### 4. Use capability status instead of boolean support

The compatibility catalog and evidence model already distinguish supported,
compatible, unsupported, and unavailable choices. External services add
experimental, blocked, and not-run gate states. This avoids silently treating a
provider import or mock as a live certification.

### 5. Use direct provider inventories for final ownership proof

Tags and run markers discover candidates, but cleanup verification must query the
owning service APIs, including global regions and inactive metadata where the
provider exposes it. Existing networks, user secrets, DNS zones, and unmarked
edge services stay outside the deletion scope.

## Risks / Trade-offs

- [A warm stack can hide a cold-start or migration regression] -> Record the
  baseline explicitly and run cold sessions for every schema, provider, runtime,
  or database-engine boundary.
- [Provider APIs expose stale tags or asynchronous deletion] -> Use bounded
  polling, exact ownership markers, and a second direct inventory after teardown.
- [Adobe requirements change faster than the code] -> Date the catalog source,
  resolve the selected release before planning, and make the catalog update a
  release-gate task.
- [Vendor credentials can leak through diagnostics] -> Carry only secret
  references and scrub values at protocol, log, evidence, and extension seams.
- [Fastly and observability adapters grow the core] -> Keep vendor fields and
  lifecycle code in extensions; the core only consumes the versioned capability
  contract.

## Migration Plan

1. Add contract and fingerprint tests before changing live harness behavior.
2. Update the shared evidence and release-gate validators, then migrate AWS and
   GCP scripts to the common session result.
3. Add provider-specific inventory assertions for OVH and Scaleway before their
   next paid run.
4. Implement Fastly and observability adapters independently, each with a
   disposable lifecycle test and exact cleanup proof.
5. Run the matrix in warm groups, create evidence, and close only the cells that
   have the required live stages.

## Open Questions

None. The remaining vendor-specific choices are intentionally deferred to the
corresponding extension implementation and do not change these contracts.
