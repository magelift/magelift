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

The proxy negative-caches unknown revisions for ~30 minutes, so
staggered tag-then-request poisons fresh versions. Stage sums
first, push every tag at once, then request nothing for five
minutes. Order matters; never move a tag:

1. Full local gates green on HEAD, working tree clean.
2. Build the staging tool first
   (`go build -o /tmp/modproxy ./cmd/modproxy`): after the
   bump, requires name nonexistent versions and nothing in
   the root module compiles, including the tool.
3. Bump requires to `<v>` (root: SDK; provider: SDK plus root)
   and commit the bump alone.
4. Stage a file proxy from that commit
   (`/tmp/modproxy --root . --ref HEAD --version <v>
   --out /tmp/proxy`). Staging reads `git archive` output
   and prunes only nested modules: anything else diverges
   from proxy construction (rc.7 died on a pruned
   `website/`).
5. Download the sums from the staged proxy with an ISOLATED
   module cache (`GOMODCACHE=$(mktemp -d)`,
   `GOPROXY=file:///tmp/proxy`, `GOSUMDB=off`). Never stage
   into the shared cache: staged bytes under a real future
   version poison every later honest check on the machine.
   Then `go mod tidy` the same way, then ASSERT zero
   superseded magelift lines remain in every go.sum (tidy
   prunes inconsistently: rc.9 died on lines one tidy kept
   and CI's dropped; delete leftovers by hand when the
   graph provably dropped them). Commit the manifests.
6. Stability gate (non-optional): re-stage from the manifest
   commit, re-download every magelift sum, and require
   `git diff --exit-code` empty. Any go.sum edit after
   staging invalidates staged sums (rc.10 died on exactly
   that: a prune landed after the last stage). Loop back
   to 5 on any diff.
7. Verify the root and provider build `GOWORK=off` against
   the staged proxy, then verify the tag tree locally the
   way CI will: fresh clone at HEAD plus `go mod tidy`
   plus `git diff --exit-code` in each module and the
   linter. Only then tag.
5. Tag `sdk/<v>`, `<v>`, and `providers/gcp/<v>` at that commit
   and push all three in one `git push`. The release starts; its
   visibility-wait sleeps first, then polls.
6. Wait five minutes in silence (no proxy, sumdb, or CI requests:
   an early 404 seeds a 30-minute negative). Then one primer per
   module (`go mod download <module>@<v>` with default proxy and
   sumdb); all three must pass.
7. Only then dispatch full CI on the release tag
   (`gh workflow run ci.yml --ref <v> -f all=true`); the publish
   gate requires actual success on the tag commit, and CI jobs
   fail the same visibility race when dispatched too early.
8. Verify the pristine consumer before trusting the candidate:
   `go install` the CLI and provider mains at `<v>` plus an SDK
   scratch build, all `GOWORK=off` from the proxy. The release
   pipeline repeats this before publish; run it locally too.

## Gates

Before cutting a public tag, read `docs/release-readiness.md` and
`docs/publishing.md`. Do not claim hosted CI green while Actions minutes are
deferred without saying so.

Workflow `run` blocks execute under `bash -e`: never end a loop body
with a bare `&&` list — when the test is false it becomes the step's
exit code and fails a step that actually succeeded (seen on rc.5).
Use a plain `if`.

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
