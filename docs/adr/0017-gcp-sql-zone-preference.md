# ADR 0017: Cloud SQL primary-zone preference on creation

- Status: Accepted
- Date: 2026-10-18

## Context

Cloud SQL can fail creation with ERROR_STOCKOUT in its automatically selected
zone, even when the requested tier is listed for the region. Acceptance
reproduced this on two supported hardware families. The existing GCP zones
setting reached networking and runtime placement but did not reach Cloud SQL.

## Decision

New Cloud SQL instances use the first resolved target.gcp.zones entry as their
primary location preference. The provider passes it through the existing
internal database component. No YAML key, manifest or RPC field is added.
An empty zone on a direct component call preserves Google default selection.

The zone property is creation-only through Pulumi IgnoreChanges on
settings.locationPreference.zone. Updating the CLI or reordering configured
zones must not implicitly relocate an existing database. Region, availability
type, backup settings, encryption and deletion protection are unchanged.
Regional instances keep Google-managed secondary placement.

Creation-capacity failures report project, environment, region, phase and a
next action. They are not automatically retried. Operators inspect the failed
operation and resource inventory, preserve state, then choose another compatible
zone before retrying the same artifact. A protected existing database is not
removed as a capacity workaround.

## Consequences

The selected zone is a request, not a capacity guarantee. Mock tests verify
inputs, the stack-to-component mapping and the ignore path. Live chosen-zone
creation and legacy-zone preservation remain acceptance checks. Existing
instances require a separately planned maintenance procedure for relocation.

## Alternatives

Continuing to leave placement entirely to Google repeats the observed failure.
Adding a second zone key would duplicate the existing placement setting.
Automatically changing regions or tiers could change cost and infrastructure
without a reviewed plan.

## Provenance

Original implementation using the public Cloud SQL locationPreference API and
Pulumi IgnoreChanges contract. Live ERROR_STOCKOUT evidence is retained privately.
