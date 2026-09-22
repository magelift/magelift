---
status: draft
slug: transparent-provider-delivery
---

# Intent: transparent first-party provider delivery

## Problem

The normal GCP path requires users or generated CI to run a separate provider
installation step before MageLift can execute the provider selected by
magelift.yaml. That exposes release plumbing as user work and creates states in
which the CLI is installed correctly but cannot run the selected cloud target.

Bundling every provider into the CLI would remove the command but scale poorly
when AWS follows and would discard the signed lock, cache, resolver, and
independent provider boundary already implemented.

## Evidence

[The architecture report](../report.md) records that rc.22 provider artifacts
are roughly 199–213 MB per platform while CLI archives are roughly 57–65 MB.

[Provider lock](../../internal/providerhost/lock.go),
[download](../../internal/providerhost/download.go),
[fetch](../../internal/providerhost/fetch.go), and
[resolution](../../internal/providerhost/resolve.go) already implement most of
the signed, versioned acquisition path. The production registry currently
reports a missing provider by asking the user to run providers install.

## Proposed outcome

On a clean supported machine, the first mutating MageLift command automatically
acquires the exact first-party provider selected by magelift.yaml, verifies its
publisher, signature bundle, digest, protocol, and required operations, installs
it atomically in the versioned cache, and executes it.

An existing project lock controls the exact provider used. Invalid existing
metadata or artifacts fail closed and are never silently replaced. Doctor
reports what will run without mutating the workstation. Explicit provider
installation remains available for offline preparation, cache prewarming, and
repair.

## Affected users and systems

GCP users, later AWS users, generated CI, release assets, providerhost,
registry/provider resolution, cache layout, installer and onboarding.

## Constraints

- Keep providers as separate signed binaries.
- Keep lockstep alpha release metadata without requiring a fat CLI archive.
- Preserve project-lock precedence and fail-closed trust.
- Download only over authenticated HTTPS from the trusted first-party release
  source.
- Do not add a bypass for signature, digest, or compatibility verification.
- One CLI invocation owns and terminates every provider process it starts.
- Normal users do not need Go, Cosign, Pulumi, or manual artifact placement.
- Community-provider trust is not broadened by this intent.

## Out of scope

- Independent first-party provider release cadence.
- A public third-party provider registry.
- AWS provider implementation.
- Builder/runtime base images.
- GCP Magento application acceptance.

## Open questions

- Should the automatic download happen at the start of every mutating command
  or in one shared pre-run hook after config resolution? The spec must choose
  one path so registry, backend, hooks, and operations cannot disagree.
