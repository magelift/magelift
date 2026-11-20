# ADR 0017. Choose the Cloud SQL primary zone at creation

Accepted on 2026-10-18.

Cloud SQL returned `ERROR_STOCKOUT` when it chose a zone automatically,
including for tiers listed in the region. The shop can express a zone
preference with the placement settings it already uses.

The provider passes the first resolved `target.gcp.zones` entry to Cloud SQL
when it creates an instance. With no zone preference, Google selects the zone.
Google also selects the secondary location for regional instances. Pulumi
ignores changes to `settings.locationPreference.zone` after creation to avoid
relocating a running database when the list is reordered.

A zone preference cannot guarantee capacity. If creation fails, review the
failed operation and resources already created. Retry the same image after
resolving the failed operation and selecting another compatible zone.
MageLift reports the environment and phase and leaves retry timing to the
operator.

Pulumi tests check the resolved zone and the ignore path. The original
capacity failure was observed in a real project. [Cloud SQL setup](../bootstrap.md#cloud-sql-placement-and-capacity)
