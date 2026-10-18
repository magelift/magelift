# ADR 0018: explicit Cloud SQL tiers select their compatible edition

Status: accepted, 2026-10-18.

## Context

The reference run encountered Enterprise Plus capacity failures in several zones.
Google currently supports MySQL 8.4 on Enterprise as well as Enterprise Plus.
The provider previously forced Plus for 8.4, rejecting dedicated custom tiers.
Changing the engine version would break the selected Magento recipe.

## Decision

Keep the existing MySQL 8.4 default: Enterprise Plus with
`db-perf-optimized-N-2`. An explicit `target.gcp.cloudSqlTier` dedicated-core
`db-custom-<cpus>-<memoryMB>` selects Enterprise. Validate the documented CPU,
memory and alignment limits before planning. Other new machine families remain
outside this change. MySQL 8.0 behavior is preserved.

The regional admission check still reads Google's tier catalog and fails on read
errors. Custom combinations are not enumerated there; a valid custom shape needs
a matching regional dedicated-core series entry. Exact predefined tiers keep
their existing regional checks. Neither check guarantees allocation capacity.

No YAML field, manifest, provider RPC or alternate state store is added.

## Consequences

Unchanged configuration keeps the same edition and tier. A deliberate custom
tier change on an existing Plus instance can request an edition downgrade and
cause downtime. Preview that change and test it outside production first.
Edition and tier remain visible to Pulumi; neither is ignored. Creation-only
zone handling from ADR 0017 remains unchanged.

Tests cover real resource inputs, default preservation, custom limits and
regional catalog handling. New live Enterprise allocation remains NOT_RUN until
the acceptance uses the published provider. A stockout is handled through the
existing diagnostic and retained state, without automatic retries.

## References

- [Google Cloud SQL editions](https://docs.cloud.google.com/sql/docs/mysql/editions-intro)
- [Google Cloud SQL machine series](https://docs.cloud.google.com/sql/docs/mysql/machine-series-overview)
- [Cloud SQL creation placement](0017-gcp-sql-zone-preference.md)
