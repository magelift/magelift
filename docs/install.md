---
title: Install MageLift
description: Install the MageLift Magento CLI. Use go install until the first GitHub Release, then the verified installer or a release archive.
---

# Install

Install the `magelift` CLI, then continue with [getting started](getting-started.md).

!!! note "No stable release yet"
    Every published release is currently a prerelease, and prereleases
    never become the default install channel. Install one with an
    explicit `MAGELIFT_VERSION` (see below), or use
    [From source](#from-source) (`go install`) until the first stable tag.

## Installer

```sh
curl -fsSL https://magelift.dev/install.sh | sh
```

The installer downloads the release archive, bootstraps a pinned Cosign
to verify the Sigstore bundle on `checksums.txt`, checks the archive
SHA-256, and installs the binary. Every step is mandatory: a missing
bundle, a mismatched checksum, or a failed signature aborts the install.
No signing tooling is needed on your machine.

```sh
curl -fsSL https://magelift.dev/install.sh | MAGELIFT_VERSION=v1.2.3 sh
curl -fsSL https://magelift.dev/install.sh | MAGELIFT_INSTALL_DIR=$HOME/bin sh
magelift version
```

## Release archive

When a release exists for your OS and architecture:

1. Open [Releases](https://github.com/magelift/magelift/releases) and download the
   archive for your OS and architecture.
2. Download `checksums.txt` and `checksums.txt.sigstore.json`.
3. Verify the bundle with Cosign against the release workflow identity,
   then check the archive SHA-256:
   ```sh
   cosign verify-blob --bundle checksums.txt.sigstore.json \
     --certificate-identity "https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/v1.2.3" \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     checksums.txt
   grep " magelift_1.2.3_linux_amd64.tar.gz$" checksums.txt | sha256sum -c -
   ```
4. Extract the `magelift` binary and put it on your `PATH`.
5. Run `magelift version`.

Release archives include checksums and an SBOM from GoReleaser.

## Staying current

`magelift upgrade` checks for and installs signed releases with the same
verification as the installer. It keeps your current binary as a backup,
swaps atomically, runs the new binary once to prove it works, and restores
the backup if that self-check fails.

## Providers

Release bundles ship the provider plugins beside the CLI. When a lockfile
points elsewhere, fetch them explicitly after install:

```sh
magelift providers install
```

Providers come from the target named in your `magelift.yaml`, verified by
digest and signature before they land in the cache. There is no flag to
select providers.

## From source

Requires a Go toolchain matching [`go.mod`](https://github.com/magelift/magelift/blob/main/go.mod):

```sh
go install github.com/magelift/magelift/cmd/magelift@latest
magelift version
```

This is also the development path: building from source never enters
release verification. There is no bypass flag.

On memory-constrained machines, contributors may set `GOMAXPROCS=1 GOFLAGS=-p=1`
(see the repo [CONTRIBUTING](https://github.com/magelift/magelift/blob/main/CONTRIBUTING.md)).
Ordinary installs do not need it.

## Next

- Local Magento without cloud credentials: [local vs cloud](local-vs-cloud.md)
- Sample config: `examples/sample-shop/` in the repository
- Cloud preview: [getting started](getting-started.md#4-cloud-preview-certified)
