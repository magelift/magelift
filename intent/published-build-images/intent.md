---
status: draft
slug: published-build-images
---

# Intent: published Magento build foundation

## Problem

The current GCP onboarding asks users to clone MageLift, build MageLift's
builder and runtime base images, inspect their digests, and then build the shop
image. This makes framework image production part of every user's first deploy,
expands the number of mutable inputs, and makes a clean-machine proof validate
maintainer image construction and shop deployment at the same time.

Creating another bundle or manifest would duplicate artifact contracts that
already exist.

## Evidence

[GCP onboarding](../../docs/onboarding.md) currently contains Docker Buildx
commands for MageLift builder and runtime images before the shop build.

[ArtifactManifest](../../build/src/Artifact/ArtifactManifest.php) already
records the Magento, PHP, Composer, module, checksum, static-content, capability,
and image facts required by the application boundary.
[ImmutableArtifactContract](../../sdk/artifact.go) already binds image,
manifest, input, provenance, and signature identities.

The clean-room production lessons
[build without runtime env.php](../../.agents/knowledge/lessons/Magento%20image%20builds%20must%20not%20see%20a%20runtime%20env.php.md)
and
[deploy before traffic moves](../../.agents/knowledge/lessons/Magento%20deploy%20writes%20env.php%20before%20traffic%20moves.md)
establish the required lifecycle split.

## Proposed outcome

MageLift releases publish tested, immutable builder and runtime images for the
small supported compatibility catalog. Each entry has digests, architecture,
Magento/PHP/Composer compatibility, build-package version, SBOM, provenance,
signature, and support status.

A shop uses those released inputs to build its own immutable application image
and the existing external artifact manifest. Users no longer clone MageLift or
build framework base images on the supported path. Builds remain free of the
runtime database, env.php, and plaintext secrets.

## Affected users and systems

Magento developers and CI; build package; build pipeline; container registry;
release workflow; image scanning and provenance; GCP onboarding; later AWS
onboarding.

## Constraints

- Reuse ArtifactManifest, ImmutableArtifactContract, and BuildArtifact.
- Do not introduce a second application manifest.
- Publish a small tested catalog, not arbitrary PHP/Magento combinations.
- Base-image and application-image digests are immutable.
- Shop source and Composer credentials remain user-owned.
- Composer credentials use the existing secret mount and never enter image
  layers, manifest, logs, or evidence.
- Runtime env.php and cloud endpoints are materialized only during deployment.
- Advanced base-image overrides must narrow or invalidate certification claims.

## Out of scope

- Replacing the Magento-specific builder with Cloud Native Buildpacks.
- Building the user's application inside a hosted MageLift service.
- GCP provisioning or the public alpha tag.
- New Magento/PHP compatibility rows.
- Provider-specific application images.

## Open questions

- Which registry namespace and retention policy should host the first-party
  builder/runtime catalog? The spec must select one immutable public path and
  define how security fixes deprecate, rather than mutate, older digests.
