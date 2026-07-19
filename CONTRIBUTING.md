# Contributing

Thank you for improving MageLift. The project is pre-alpha; discuss substantial
changes in an issue before investing in an implementation.

## Principles

- Keep user projects YAML-only for the normal path.
- Prefer provider-specific, testable implementations over leaky abstractions.
- Make configuration and deployment behavior deterministic and explainable.
- Never copy source, documentation, identifiers, secrets, or proprietary assets
  from private or employer-owned repositories.
- Record design inputs in `docs/provenance.md` and preserve third-party notices.
- Never commit credentials, customer data, build artifacts, or Pulumi state.

## Changes

1. Create a focused branch from `main`.
2. Add tests and documentation with behavior changes.
3. Run `make verify` and any component-specific integration checks.
4. Use a Conventional Commit subject, such as `feat(config): explain provenance`.
5. Open a pull request describing risk, compatibility impact, and verification.

Commits included in signed releases must be attributable. Maintainers may require
signed commits or a Developer Certificate of Origin sign-off before public release.
Contributions are submitted under Apache-2.0 unless explicitly stated otherwise.

## Compatibility and security

Do not silently weaken compatibility, protection, validation, or secret-handling
rules. Report suspected vulnerabilities through the private process in
`SECURITY.md`, not a public issue.

All contributors must follow `CODE_OF_CONDUCT.md`.
