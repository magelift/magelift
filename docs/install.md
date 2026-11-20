---
title: Install MageLift
description: Install the MageLift CLI from source or from a signed release.
---

# Install

Install the `magelift` CLI, then continue with [getting started](getting-started.md).

## From source

Requires a Go toolchain matching [`go.mod`](https://github.com/magelift/magelift/blob/main/go.mod):

```sh
git clone https://github.com/magelift/magelift.git
cd magelift
go install ./cmd/magelift
magelift version
```

Run these commands from the checkout. The Go workspace selects the modules
from that commit.

Use the source build for local work. Cloud commands still verify the provider
binary against a trusted lock and Cosign bundle.

## From a signed release

### Installer

```sh
curl -fsSL https://magelift.dev/install.sh | sh
```

The unpinned installer selects the latest stable release. To test a candidate,
select its exact tag with `MAGELIFT_VERSION`.

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

`v1.2.3` is a placeholder. Use the tag you intend to install.

### Release archive

Choose the archive for your operating system and architecture.

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

### Staying current

`magelift upgrade` checks for and installs signed releases with the same
verification as the installer. It copies your current binary as a backup,
swaps atomically (the installed command is never missing, even on a
crash), runs the new binary once to prove it works, and restores the
backup if that self-check fails. On Windows the swap cannot replace a
running executable; rerun the upgrade from a second copy in that case.

## Providers

The released CLI downloads the AWS, GCP or Azure plugin selected by your shop when a command first needs it. A project lock in `magelift.providers.lock` selects the exact version. Without a lock, MageLift uses the CLI’s release version. Help, version, doctor and local config validation work before a plugin is installed.

Run these commands online in the shop checkout to prepare an offline workstation.

```sh
magelift providers install
magelift extensions list
```

MageLift verifies the binary digest and Cosign signature, then caches the plugin by provider, version, operating system and architecture. Keep the project lock and verified cache when moving offline. If the CLI reports a damaged or unverified entry, check the lock and run `magelift providers install` to repair it. Community extensions use a custom CLI build. [Provider trust](adr/0014-distribution-trust.md) · [External extensions](adding-a-provider.md#develop-an-external-extension)

Signed release operations and remote sessions can offer to install missing
tools through the existing package-manager allowlist. Review the command
before confirming, or use `--yes` in a prepared CI environment. Docker and
cloud authentication remain prerequisites. `magelift doctor` reports missing
tools; only `doctor --install-dependencies` requests their installation.

Homebrew and Scoop release configuration lives in the project's
`magelift/homebrew-tap` and `magelift/scoop-bucket` repositories. Check the release notes for the operating systems used in verification. WSL is an alternative on Windows, with its own Docker access and cloud
authentication.

## Next

- Local Magento without cloud credentials: [local vs cloud](local-vs-cloud.md)
- Sample config: `examples/sample-shop/` in the repository
- Cloud preview: [getting started](getting-started.md#connect-the-cloud)
