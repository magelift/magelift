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
- Use the latest patch record for 2.4.6 through 2.4.9. Do not use a version range.
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

## Build the shop

Run `magelift build` from the shop repository after committing YAML, the Composer
lock and existing module configuration. The default community recipe is PHP 8.5
with nginx-fpm on Linux amd64. Recipes and pinned upstream inputs ship with
the CLI. The user's Docker engine/CI builds the builder, runtime and application
images. Local builds do not publish. `magelift build --push --image <registry>/<repository>:<tag>`
publishes the application into that user-owned repository and signs it.
Builder/runtime intermediates stay on the runner; the final build consumes the
bundled pinned runtime recipe. Optional `build.cache` configures separate registry
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
