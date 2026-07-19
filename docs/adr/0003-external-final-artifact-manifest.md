# ADR 0003: Finalize the artifact manifest outside the image

- Status: Accepted
- Date: 2026-07-18

## Context

The artifact manifest must identify the exact OCI image by digest. Embedding that final
manifest in the image would change the image digest, which would make the embedded
digest wrong. Rebuilding with the new value repeats the same cycle.

The build still needs a durable record of its inputs before BuildKit produces the OCI
digest. Release and promotion workflows then need one finalized manifest that binds
those inputs to the published image.

## Decision

The finalized artifact manifest is external to the application image. MageLift creates
it after BuildKit returns the OCI digest. The manifest may later be stored as an OCI
artifact or signed attestation associated with that digest, but its storage form does
not change this boundary.

Manifest creation has two steps:

1. `prepare` writes deterministic pre-digest metadata. It records the source revision,
   MageLift and build versions, Magento edition and version, PHP and extension data,
   enabled modules, sanitized build inputs, file checksums, static content matrix,
   required runtime capabilities, and compatibility result.
2. `finalize` accepts only the prepared output and successful BuildKit result metadata.
   It validates and adds the published OCI digest, then writes the final manifest.

The prepared output has a versioned schema and a checksum. Finalization verifies that
checksum and requires a valid digest from BuildKit's `containerimage.digest` field. It
does not inspect the working tree, rerun discovery, or accept environment configuration.
This makes finalization deterministic and prevents facts from changing between the
build and release record.

Neither step records secret values, secret references, credentials, tokens, or
temporary secret paths. Build inputs that affect the artifact must be represented by
safe facts such as dependency lock checksums and selected build options.

The image may contain a copy of the prepared pre-digest metadata for diagnostics. That
file must identify itself as pre-digest build metadata and must not use the finalized
artifact manifest media type or filename.

Finalization never rebuilds, retags, or mutates the image. Signing, promotion, and
rollback use the digest in the external finalized manifest.

## Consequences

An image digest remains stable while the final manifest can name it accurately. The
same signed digest and manifest pair can move through staging and production without a
rebuild.

Consumers need both the image and its finalized manifest. Release publication must
store them together and verify their binding before deployment. Until OCI artifact or
attestation publication is implemented, MageLift must keep the manifest in its release
metadata store and preserve its checksum and signature.

Pre-digest metadata inside an image is useful for inspection but is not proof of the
published digest. Tools must not treat it as the final release record.

## Alternatives considered

- Embedding the finalized manifest was rejected because it creates an impossible digest
  cycle.
- Rebuilding after finalization was rejected because the rebuilt image has a different
  digest and violates build-once promotion.
- Omitting the OCI digest from the manifest was rejected because a source revision or
  tag does not identify immutable image bytes.
- Mutating an image or tag after publication was rejected because promotion and signing
  must use immutable digests.

## Provenance

This is an original project decision based on OCI content-addressed image behavior and
the MageLift build-once release contract. No third-party source code was used.
