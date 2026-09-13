## Why

Related scope is defined by [`multi-cloud-resilience-observability-edge`](../multi-cloud-resilience-observability-edge/proposal.md). This change owns Fastly API lifecycle and migration mapping proof; the related change owns the portable edge intent, native-edge composition, origin safety, and cross-provider certification gates.

Adobe Commerce Cloud includes Fastly in its standard edge path, and MageLift already imports projects shaped by Adobe Commerce Cloud and Upsun. Treating Fastly as an explicit edge adapter would make migration intent visible instead of losing it during import.

## What Changes

- Add a provider-neutral edge contract with a Fastly adapter.
- Represent Fastly service, domain, TLS, purge, and VCL intent without placing raw Fastly configuration in the portable core schema.
- Extend ACC and Upsun import mapping to preserve edge intent and report fields that require operator action.
- Add preview, validation, and cleanup behavior for Fastly resources.
- Keep CloudFront, Cloud Armor, and other provider edge implementations independent.
- Do not claim production CDN certification until a real Fastly account test passes.

## Capabilities

### New Capabilities

- `fastly-edge`: Fastly edge configuration and lifecycle behavior.
- `paas-edge-migration`: Migration mapping and unsupported-field reporting for ACC and Upsun.

### Modified Capabilities

None. The repository has no existing OpenSpec capability specs.

## Impact

The change affects the portable edge contract, provider adapters, PaaS importers, configuration validation, secrets handling, acceptance scripts, and migration documentation. Fastly credentials must remain secret references and must never be written to generated YAML.
