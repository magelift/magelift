# ADR 0018. Choose the Cloud SQL edition from the requested tier

Accepted on 2026-10-18.

A shop can keep MySQL 8.4 when a Cloud SQL Enterprise Plus shape is
unavailable. MySQL 8.4 defaults to Enterprise Plus with
`db-perf-optimized-N-2`. An explicit supported `db-custom-<cpus>-<memoryMB>`
tier selects Enterprise.

The provider checks the CPU and memory limits and queries the regional tier
catalog. A custom size needs a compatible dedicated-core series in that
catalog. A predefined tier needs its own entry. Available capacity still
depends on the region and zone.

Pulumi plans edition or tier changes. Switching an existing Plus instance
to a dedicated custom tier can request a downgrade and downtime. Rehearse the
change outside production.

Tests check defaults, valid custom sizes, resource arguments and catalog
validation. A development run with provider build `0660426` allocated
an Enterprise instance with the explicit tier.
[Zone selection](0017-gcp-sql-zone-preference.md) · [Cloud SQL editions](https://docs.cloud.google.com/sql/docs/mysql/editions-intro)
