# ADR 0014. Verify releases before execution

Accepted on 2026-09-17.

The installer, updater and provider loader download executables. They verify
who published the release and whether the downloaded bytes match that release
before running them.

| Path | Verification |
| --- | --- |
| Install | The script bootstraps pinned Cosign, verifies the Sigstore bundle on `checksums.txt` and checks the archive hash. |
| Upgrade | The updater verifies the bundle and archive, then swaps the executable. It keeps a backup and restores it if the new executable fails its self-check. |
| Load a provider | The CLI checks the executable's digest and Cosign bundle against its lock before starting it. |
| Download a provider | The installer downloads over HTTPS into the user cache and exposes the verified executable and bundle together. |

A project lock takes priority over the lock for the CLI release. If the
project lock is invalid, MageLift reports the error. A development build needs
an explicit trusted lock or provider installation. Help and local
configuration validation work without a provider. [Install providers](../install.md#providers)

Offline verification uses a public Sigstore trust root embedded in the CLI.
The root snapshot was authenticated through Sigstore TUF on 2026-10-17.
MageLift checks its digest and uses it with pinned Cosign. Provider installs
serialize per cache entry. The CLI re-verifies a cached executable before reuse.
Use `magelift providers install` to repair a damaged cache entry.

The release tags match across the core, SDK and first-party providers. The
module versions reach the Go proxy before the root release builds. The release
workflow builds and signs the archive and provider bundles.
[Publishing](../publishing.md) · [Provider executables](0008-provider-load-path.md)
