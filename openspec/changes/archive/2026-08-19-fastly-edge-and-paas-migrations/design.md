## Context

See `proposal.md` for the migration motivation. MageLift already has provider-neutral edge capability IDs, CloudFront and Cloud Armor adapters, and ACC or Upsun import paths. Fastly must fit the same boundary and must not turn portable YAML into a Fastly schema.

## Goals / Non-Goals

**Goals:**

- Preserve migration intent for Fastly-backed sources.
- Add an explicit Fastly adapter with secret references and ownership markers.
- Keep preview non-mutating and cleanup auditable.

**Non-Goals:**

- Translating arbitrary VCL into a portable policy language.
- Replacing CloudFront or Cloud Armor.
- Claiming Adobe Commerce Cloud parity.

## Decisions

### 1. Keep Fastly behind the edge adapter boundary

Fastly fields will live under a provider-owned configuration namespace and map to portable edge outputs. Raw provider objects and VCL remain extension data or referenced files, not core schema fields.

### 2. Use references for secrets and VCL

Tokens, TLS material, and large VCL bodies will use secret or artifact references. Importers will retain source references and diagnostics rather than copying sensitive content into YAML.

### 3. Separate migration intent from live certification

Import mapping can ship before a live Fastly account is available. The capability remains experimental until the acceptance cell proves the real API behavior and teardown.

Apply requires an injected origin and route health probe before any Fastly
mutation. Destroy requires an owning-service inventory probe and bounded
polling after delete requests; a successful CLI delete response alone is not
cleanup evidence. The default CLI wires the inventory probe but deliberately
has no implicit origin URL or health semantics, so a live apply remains
fail-closed until the runtime supplies an application health implementation.

The current Fastly account uses Unified Domain Management. The adapter therefore
uses the current versionless `domain` API and treats managed TLS as a required
ownership/activation dependency for that path. A managed TLS subscription
returns an ACME DNS challenge; the adapter exposes that challenge as opaque
provider output, while a separately selected DNS adapter owns the DNS record.
The acceptance harness uses the authenticated Cloudflare CLI only for its exact
`acourtiol.com` disposable record and deletes the challenge before releasing the
run. The older classic service-domain API remains an explicit capability for
accounts that still expose it; this account rejects that API and the acceptance
profile does not claim non-TLS versionless routing.

Fastly route verification has a bounded convergence wait because service
activation, certificate deployment, and edge routing are asynchronous. Managed
TLS is deleted before the versionless domain, and account-level domain inventory
is checked after service deletion because a versionless domain can outlive its
owning service. A failed live test invokes an exact-marker fallback cleanup so a
process crash cannot silently retain the TLS subscription or domain.

## Risks / Trade-offs

- [Fastly configuration is richer than the portable edge contract] -> Preserve provider-owned extension data and emit unmapped diagnostics.
- [Purge or TLS operations are asynchronous] -> Poll provider status and make cleanup bounded and observable.
- [Migration input contains secrets] -> Redact before evidence and fail closed for plaintext credentials.

## Migration Plan

1. Add typed edge intent and validation.
2. Extend ACC and Upsun import mapping with unmapped-field diagnostics.
3. Add mock Fastly plans and ownership tests.
4. Add a disposable real-account acceptance profile.
5. Update migration documentation after the first live pass.
