---
status: accepted
slug: v1-stable-cut
---

# Intent: first stable cut agencies can install

## Problem

There is no public tag. Releases is empty, install is `go install` from source, and `main` carries no stability promise. Agencies and SME teams without DevOps capacity cannot adopt a tool they must build from source with a Go toolchain. Every broader-support claim stays theoretical until one tagged, installable, checksummed CLI exists.

## Evidence

`README.md`: "Until `v1.0.0-rc.1`, install from source (Releases is empty)." `docs/versioning.md` defines the RC freeze surface (schemaVersion, CLI verbs, exit codes, provider IDs, certified cells) but no tag enforces it yet. `docs/release-readiness.md` gate board tracks trademark, license, signed artifacts, SBOM, provenance, green CI. `docs/publishing.md` and `.goreleaser.yaml` describe the intended archive flow; `make release-smoke` passed 2026-07-28 as a serial host-only check. Homebrew/Scoop stay blocked on a public tag per `openspec/BACKLOG.md`.

## Proposed outcome

`v1.0.0-rc.1` is tagged. GitHub Release archives ship with SHA-256 checksums, SBOM, SLSA provenance, and keyless Sigstore verification. `magelift upgrade` verifies a release before replacing the binary. Install docs point at archives first, `go install` second. The versioning freeze surface holds from that tag: additive YAML fields only, stable verb names and exit codes, provider IDs frozen. Homebrew cask follows only after a tested macOS install path.

## Affected users and systems

Every prospective user. Release Please, GoReleaser, GHCR container matrices, `magelift upgrade`, `docs/install.md`, website install pages, CI release workflows.

## Constraints

No tag until the release-readiness gates it names are documented: trademark and package-name clearance, GitHub/Packagist/GHCR/docs name ownership, full license and NOTICE review, signed artifacts with vulnerability gates, green required CI for the release scope, accurate non-affiliation language. Single CLI version for v1; independent provider releases are explicitly post-v1 (see `lean-core-provider-boundary`). Conventional Commits drive notes after the tag.

## Out of scope

New providers, new catalog cells, subprocess provider loading, community catalog, Windows package managers beyond archive download.

## Open questions

Which gate-board rows are still open at cut time, and who signs each one? Does the Homebrew cask ship with rc.1 or wait for a tested post-tag install?
