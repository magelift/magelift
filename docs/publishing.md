# Publish a release

MageLift releases include the CLI and separate plugins for AWS, Google Cloud,
and Azure. GoReleaser builds the platform archives. The release workflow signs
checksums and provider locks, generates software bills of materials, and
tests the signed CLI installer and all three signed provider plugins against
draft release assets before the CI gate publishes the release.

Run the [contributor checks](https://github.com/magelift/magelift/blob/main/CONTRIBUTING.md)
and `make release-smoke` before requesting approval for a tag. The smoke check
builds the CLI and all three provider binaries for the local platform and
checks the archive and checksum files. Verify the selected commit's full CI
run before publishing. [How to report provider results](capability-matrix.md)

After approval, tag the root module, SDK, and all provider modules at the same
commit. Push the SDK and provider tags first. Push the root tag on its own
so GitHub starts `release.yml`. The workflow waits for the modules to reach
the Go proxy before building. Release Please manages the root changelog
and version. Module tags use the corresponding prefix.

- `vX.Y.Z`
- `sdk/vX.Y.Z`
- `providers/aws/vX.Y.Z`
- `providers/gcp/vX.Y.Z`
- `providers/azure/vX.Y.Z`

Use the [release runbook](https://github.com/magelift/magelift/blob/main/.agents/skills/magelift-release/SKILL.md)
for maintainer steps. Public tags and publication need explicit approval.
[Install](install.md) · [Distribution trust](adr/0014-distribution-trust.md)
