---
status: done
slug: web-runtime-image-naming
---

# Intent: name the nginx image php-nginx

## Problem

The nginx-based PHP image is named `php-runtime` while its siblings are named by web server (`php-apache`, `frankenphp-classic`), so humans cannot tell from the name which web server an image runs. The asymmetry spans bake targets, local tags, published GHCR repos, Go defaults, and docs.

## Evidence

- `docker-bake.hcl` targets: `php-runtime` (+ `php-builder` inheriting it, + `-supported` matrix variants) next to `php-apache` / `php-apache-supported` and `frankenphp-classic` / `frankenphp-classic-supported`.
- `.github/workflows/images.yml` publishes `ghcr.io/magelift/magelift-runtime`, `magelift-builder`, `magelift-frankenphp-classic` (matrix `image: runtime|builder|frankenphp-classic`).
- `internal/toolchain/toolchain.go` defaults: `DefaultBuilderTag = "magelift/php-builder:local"`, `DefaultRuntimeTag = "magelift/php-runtime:local"`.
- `images/README.md` documents `php-runtime` as "the common PHP-FPM and nginx base".
- No stable images have shipped yet: `images.yml` skips prerelease tags (`if: ${{ !contains(github.ref_name, '-') }}`), so renaming the GHCR repo is still free — after the first stable tag it becomes a migration with aliases.
- User decisions this session: full rename including the GHCR repo name; keep `php-apache` as a maintained experimental adapter.

## Proposed outcome

The nginx image is named `php-nginx` consistently everywhere: bake targets and local tags, the published GHCR repo, Go default tags, image labels, workflows, Dependabot entries, and docs. `php-apache` stays as a maintained experimental adapter with Dependabot coverage, the Varnish base pin is confirmed, and the `:local`-vs-GHCR default-image policy is recorded for the post-stable decision instead of living only in review notes.

## Affected users and systems

Contributors building images (`docker buildx bake`, `make image-test`); the Go toolchain resolver and its tests; `images.yml` publish matrix; Dependabot docker entries; `images/README.md` and any docs naming `php-runtime`/`magelift-runtime`; future consumers pulling versioned GHCR images.

## Constraints

- Full rename including GHCR (validated); keep `php-apache` (validated).
- Renaming is names only: no Dockerfile behavior change, no digest bumps beyond what Dependabot owns, no `:local` default semantics change.
- Verify no `ghcr.io/magelift/magelift-runtime` packages exist before assuming the rename is alias-free; if any do, the plan must handle them.
- Human pages touched go through humanizer, then remove-ai-marks.
- Image matrix stays PHP 8.2–8.5 on Debian Trixie; capability tiers unchanged (a rename recertifies nothing).
- Record the default-image policy (`:local` bake now, GHCR-versioned defaults after stable) in the repo; do not implement GHCR pull in this change.

## Out of scope

- Implementing GHCR pull or versioned image defaults in the CLI (post-stable work; this change only records the policy).
- Apache certification or new images/variants.
- Base-image digest bumps (Dependabot's job).
- Deploy story cleanup (covered by `github-pages-deploy`); docs/site hygiene (covered by `site-docs-hygiene`).

## Open questions

- Exact GHCR repo name: `ghcr.io/magelift/magelift-nginx`? Default: yes, mirroring `magelift-frankenphp-classic` and `magelift-builder`.
- The builder inherits the nginx runtime target: does `magelift-builder` keep its name (build runner is runtime-agnostic conceptually) while its base becomes `php-nginx`? Default: builder keeps its name; only its base target changes.
- Do `io.magelift.*` label keys/values need renaming, or only documentation of them? Default: rename user-visible values, keep stable key names; spec confirms per label.
