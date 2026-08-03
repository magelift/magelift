---
name: magelift-release
description: >-
  Cut MageLift releases: GoReleaser, Cosign identities, GHCR image names, and
  local release smoke. Use when tagging, fixing release workflows, or verifying
  packaging before v1.0.0-rc or stable tags.
---

# MageLift release

## Local smoke (serial)

```sh
make release-smoke
# or
./scripts/release-smoke-local.sh
```

Never run a full multi-platform `goreleaser release` on a laptop. See
`magelift-serial-builds`.

## Identities and registries

After the org move, public paths are:

| Kind | Value |
| --- | --- |
| Module | `github.com/magelift/magelift` |
| Repo | `https://github.com/magelift/magelift` |
| Cosign release identity | `https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/<tag>` |
| Cosign images identity | `https://github.com/magelift/magelift/.github/workflows/images.yml@refs/tags/<tag>` |
| GHCR | `ghcr.io/magelift/magelift-{runtime,builder,frankenphp-classic}` |

Do not reintroduce `acourtiol/magelift` in workflows or ldflags.

## Images workflow

`.github/workflows/images.yml` skips prerelease tags (semver `-rc` / `-beta`).
Stable semver tags only unless that `if` is intentionally changed.

## Gates

Before cutting a public tag, read `docs/release-readiness.md` and
`docs/publishing.md`. Do not claim hosted CI green while Actions minutes are
deferred without saying so.

## Homebrew

Cask publish stays optional until `magelift/homebrew-tap` and
`HOMEBREW_TAP_GITHUB_TOKEN` exist. Prefer GitHub Release archives first.
