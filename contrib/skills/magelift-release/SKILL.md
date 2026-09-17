---
name: magelift-release
description: >-
  Cut MageLift releases: GoReleaser, Cosign identities, GHCR image names, and
  local release smoke. Use when tagging, fixing release workflows, or verifying
  packaging before v1.0.0-rc or stable tags.
version: 1.0.0
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
| GHCR | `ghcr.io/magelift/magelift-{nginx,builder,frankenphp-classic}` |

Do not reintroduce `acourtiol/magelift` in workflows or ldflags.

## Images workflow

`.github/workflows/images.yml` skips prerelease tags (semver `-rc` / `-beta`).
Stable semver tags only unless that `if` is intentionally changed.

## Candidate tag sequence

Requires must name tags that exist with complete manifests, but sums
need the tags first. Order matters; never move a tag:

1. Full local gates green on HEAD, working tree clean.
2. Tag and push `sdk/<v>` at HEAD (the SDK has no MageLift
   requires, so its content is final).
3. Bump requires to `<v>` (root: SDK; provider: SDK plus root),
   download the SDK sums, verify the root builds `GOWORK=off`,
   commit.
4. Tag and push `<v>` at that commit. The release starts; its
   visibility-wait step polls the proxy and sumdb before packaging
   (fresh tags 404 for minutes).
5. Download the root sums into the provider, commit, tag and push
   `providers/gcp/<v>`. Module tags may point at different commits;
   each module version is immutable and complete on its own.
6. Poll module visibility until two consecutive
   `go mod download <module>@<v>` passes 60s apart for all three
   modules (fresh tags 404 for minutes and flap into view).
   Only then dispatch full CI on the release tag
   (`gh workflow run ci.yml --ref <v> -f all=true`); the publish
   gate requires actual success on the tag commit, and CI jobs
   fail the same visibility race when dispatched too early.
7. Verify the pristine consumer before trusting the candidate:
   `go install` the CLI and provider mains at `<v>` plus an SDK
   scratch build, all `GOWORK=off` from the proxy. The release
   pipeline repeats this before publish; run it locally too.

## Gates

Before cutting a public tag, read `docs/release-readiness.md` and
`docs/publishing.md`. Do not claim hosted CI green while Actions minutes are
deferred without saying so.

## Homebrew

Cask publish stays optional until `magelift/homebrew-tap` and
`HOMEBREW_TAP_GITHUB_TOKEN` exist. Prefer GitHub Release archives first.

## Use this skill when

- You are preparing a release candidate, release archive, or container image.
- You are checking signing identity, version injection, or release smoke.
- You are changing bundled skill or extension artifacts.

## Leave behind

- A reproducible local smoke result and the exact version inputs used.
- Signed immutable artifacts with the expected public paths.
- No claim that a hosted workflow passed unless its logs were actually checked.
