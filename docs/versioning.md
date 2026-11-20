# Versioning

MageLift uses [Semantic Versioning](https://semver.org/) for releases. The
CLI, SDK, and first-party provider modules publish matching versions. A
verified provider lock records the exact artifact used by a shop.
[Provider installation](install.md#providers) · [Publishing](publishing.md)

## Project contracts

| Surface | What to check when upgrading |
| --- | --- |
| `magelift.php` | The project declares a schema version. Check the field reference before changing versions. |
| CLI | Release notes cover changes to commands, flags, and exit codes. |
| Providers | First-party IDs are `aws`, `gcp`, and `azure`. Use the provider lock and CLI release together. |
| Plugins | The lockfile and SDK protocol declare their own versions. The CLI checks the plugin identity and advertised operations when it starts. |

Experimental runtimes can change between releases. Check the operation and
its test results before deploying a shop on an experimental target.
[Provider support](capability-matrix.md)

Release Please uses Conventional Commits to generate the changelog.
[Release workflow](https://github.com/magelift/magelift/blob/main/.github/workflows/release.yml)
