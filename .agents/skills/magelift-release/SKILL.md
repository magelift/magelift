---
name: magelift-release
description: Prepare and verify MageLift releases, module tags, GoReleaser archives, and Cosign signatures.
metadata:
  version: "1.0.0"
---

# Prepare a release

Read [Publishing](../../../docs/publishing.md) and the
[distribution trust decision](../../../docs/adr/0014-distribution-trust.md).
Run `make verify-community`, `make release-smoke`, and the relevant Floci
provider checks. The release workflow installs the signed CLI and all three
provider plugins from the draft assets before publication. Ask before pushing
release tags or publishing.

## Stage the Go modules

Build `cmd/modproxy` before editing the required versions. The root module
requires the SDK release. Each provider requires that SDK and the matching
root module.

```sh
go build -o /tmp/magelift-modproxy ./cmd/modproxy
/tmp/magelift-modproxy --root . --ref HEAD --version vX.Y.Z --out /tmp/magelift-proxy
```

Stage from the intended commit. Use a temporary `GOMODCACHE` and the local
file proxy when downloading unpublished module sums. Keep staged versions
out of the shared cache. Run tidy and verify the CLI and provider builds
with `GOWORK=off`. Re-stage after changes to the module manifests and confirm
the staged requirements and sums still match.

After approval, tag the same commit for the CLI, SDK, and three provider
modules. Push the SDK and provider tags first. Push the CLI tag by itself
to start the release workflow. GitHub skips tag push events when a push
includes more than three tags. [GitHub push events](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#push)

Wait five minutes after the CLI tag before querying the Go proxy. The proxy
caches early misses. Verify each module resolves, then dispatch full CI with
`gh workflow run ci.yml --ref <tag> -f all=true`.

## Verify the artifacts

Check the published Go modules, the draft archives, and the provider lock
and signatures. The verification identity is
`https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/<tag>`.
Shop workflows sign application images with the shop repository's
identity. Report the version and the checks you actually ran.
