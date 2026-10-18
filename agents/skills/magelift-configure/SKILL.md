---
name: magelift-configure
description: >-
  Configure MageLift YAML and choose an Adobe-compatible architecture. Use when
  selecting PHP, database, search, queue, cache, Varnish, edge, or provider options.
version: 1.0.0
---

# Configure MageLift

Use this skill before preview or deploy when the service shape is changing.

## Working rules

- Start with `magelift compatibility catalog` for the exact Adobe patch line.
- Use the latest patch record for 2.4.7 through 2.4.9. Do not use a version range. Magento 2.4.6 is unavailable because it requires PHP 8.2.
- Separate three facts: Adobe lists the service, MageLift implements the service,
  and the selected provider exposes the required product.
- Keep `compatibility.allowUnsupported` off unless the exception is deliberate,
  documented, and visible in the planned artifact.
- Prefer database-backed queues, no managed search, and small replicas for
  disposable previews.
- AWS Magento env must match MageLift's product contract for the selected
  YAML: writer host + secret JSON for RDS/Aurora; provisioned OpenSearch is
  in-VPC HTTPS Magento can query (no SigV4 sidecar); AOSS still needs a SigV4
  sidecar. Private sibling shops may illustrate one of those shapes. They are
  not the only target and they do not certify a cell.

## Start from an existing shop

Run `magelift init` from the shop repository root (the parent of `--config`).
It requires composer.json, composer.lock, and `bin/magento`, reads the
locked Magento metapackage (or `magento/magento2-base` for source checkouts),
and writes a minimal single-environment starter on GCP by default. It does not
read `app/etc/env.php`, auth.json, or execute shop scripts; existing
`app/etc/config.php` is optional and unchanged. Use `--provider aws` for the
experimental AWS starter. If the locked release is outside the compatibility
catalog or needs a PHP branch other than 8.5, `init` exits before writing;
author YAML manually with the right `build.php` instead. After a successful
`init`, review placeholders, commit `magelift.yaml`, then run
`magelift bootstrap --env test`.

For GCP MySQL 8.4, the default tier remains Enterprise Plus N2. An explicit
`target.gcp.cloudSqlTier: db-custom-2-7680` selects Enterprise dedicated core
with 2 vCPUs and 7680 MB memory. Custom sizes are validated, and regional support
does not guarantee capacity. Shared-core and N4 custom tiers are outside this
override. Preview an explicit tier change on an existing Plus database and test
outside production: an edition downgrade can cause downtime. No automatic tier
change follows a stockout.

## Build the shop

Run `magelift build` from the shop repository after committing YAML, the Composer
lock and existing module configuration. The default community recipe is PHP 8.5
with nginx-fpm on Linux amd64. Recipes and pinned upstream inputs ship with
the CLI. The user's Docker engine/CI builds the builder, runtime and application
images. Local builds do not publish. `magelift build --push --image <registry>/<repository>:<tag>`
publishes the application into that user-owned repository and signs it.
GitHub Actions signing uses the job's OIDC identity and requires `id-token:
write`; it does not impersonate the deployment account for signing. Explicit
token overrides remain available for advanced automation.
Builder/runtime intermediates stay on the runner; the final build consumes the
bundled pinned runtime recipe. Rebuild the application in user CI to receive
runtime recipe fixes; promotion of an older digest keeps its original bytes.
Versioned static URLs resolve to prebuilt files, without runtime static generation.
Optional `build.cache` configures separate registry
cache references; keep private layers and release caches out of fork jobs.
`build.composer.credentials: env://COMPOSER_AUTH` accepts CI credentials through
the existing private secret mount. `build --export-recipe <new-directory>` exports
the recipe for diagnostics without Docker or cloud credentials. Authenticate Docker first; MageLift has no required
image registry. No framework source checkout is needed.

Existing disabled modules and declared locales/themes are preserved. Build-time
credentials use secret references and private mounts. Advanced base overrides
must be supplied together; pushed builds require registry digests. Explicit ARM
overrides remain outside alpha qualification. The package/build component also
works without invoking cloud provisioning.

Optional `build.cloudPatches: true` delegates patch application to the shop's
locked `magento/magento-cloud-patches` executable (one `ece-patches apply` after
`composer install`). List Quality Patch IDs under `build.qualityPatches`; when
cloud patches are enabled they ride the same upstream pass as Cloud-required
patches and `m2-hotfixes`, not a separate MageLift stage. Lock
`magento/quality-patches` when IDs are declared. MageLift does not certify this
path; stderr announces the scope when `magelift build` runs.

## Check the result

```sh
magelift compatibility catalog 2.4.8-p5
magelift compatibility validate --release 2.4.8-p5 \
  --component search --option elasticsearch --version 8
magelift --config magelift.yaml --env staging config validate
magelift --config magelift.yaml --env staging preview
```

If a choice is Adobe-supported but the target reports unavailable, do not mark
the cell certified. Choose another target or add a provider extension.

## Generate user CI safely

`magelift ci generate` creates the workflow when absent and leaves identical
content untouched. If an existing workflow differs, it preserves that file and
writes a candidate beside it. Review the reported diff before rerunning the same
command with `--replace`. Keep any original `--path` and version flags.
`magelift ci validate` checks project configuration, presence and YAML syntax;
it does not validate arbitrary GitHub Actions semantics. Add `--check-template`
only when exact MageLift template freshness is intended.

An installed release defaults to its own immutable version; the CLI normalizes
the optional leading v to the release tag. A development binary needs an
explicit --magelift-version naming an available signed release. A single test
environment uses push or manual dispatch without production selection inputs.
Keep generated build results and credential files outside the checkout when
customizing CI. The build requires a clean committed Git source.
An untagged MAGELIFT_RELEASE_IMAGE repository gets the source GITHUB_SHA tag;
an explicitly tagged destination is preserved, including registries with ports.
Artifact handoff uses repository@sha256 references: the publication tag is
removed from the reference passed to deploy, while its digest stays unchanged.
The generated build job installs a pinned Docker Engine before Buildx; it
does not depend on the runner image's older bundled Docker version.
