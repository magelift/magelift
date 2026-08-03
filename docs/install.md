---
title: Install MageLift
description: Install the MageLift Magento CLI. Use go install until the first GitHub Release, then prefer archives with checksums and SBOM.
---

# Install

Install the `magelift` CLI, then continue with [getting started](getting-started.md).

!!! note "No public release tag yet"
    GitHub Releases is empty until `v1.0.0-rc.1` ships. Until then, use
    [From source](#from-source) (`go install`). After the first tag, prefer a
    release archive (checksums + SBOM).

## Prefer a release archive

When a release exists for your OS and architecture:

1. Open [Releases](https://github.com/magelift/magelift/releases) and download the
   archive for your OS and architecture.
2. Verify the checksum against `checksums.txt` (and the Sigstore bundle when present).
3. Extract the `magelift` binary and put it on your `PATH`.
4. Run `magelift version`.

Release archives include checksums and an SBOM from GoReleaser.

## From source

Requires a Go toolchain matching [`go.mod`](https://github.com/magelift/magelift/blob/main/go.mod):

```sh
go install github.com/magelift/magelift/cmd/magelift@latest
magelift version
```

On memory-constrained machines, contributors may set `GOMAXPROCS=1 GOFLAGS=-p=1`
(see the repo [CONTRIBUTING](https://github.com/magelift/magelift/blob/main/CONTRIBUTING.md)).
Ordinary installs do not need it.

## Next

- Local Magento without cloud credentials: [local vs cloud](local-vs-cloud.md)
- Sample config: `examples/sample-shop/` in the repository
- Cloud preview: [getting started](getting-started.md#4-aws-preview-certified)
