---
name: magelift-configure
description: >-
  Configure a MageLift PHP project and choose Magento services. Use when
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
- Treat the catalog as a dated recommendation. Review warnings for choices outside the snapshot; provider and implementation constraints still apply.
- Review the selected service versions and provider adapter. Existing and
  external services need an endpoint and any required secret reference.
- Disposable previews can use smaller capacity than production. Keep their
  data and credentials separate unless the operator explicitly chooses otherwise.

## Start from an existing shop

Run `magelift init` from the shop repository root (the parent of `--config`).
It requires composer.json, composer.lock, and `bin/magento`, reads the
locked Magento metapackage (or `magento/magento2-base` for source checkouts),
and writes staging, UAT, production and preview environments on GCP by default. It does not
read `app/etc/env.php`, auth.json, or execute shop scripts; existing
`app/etc/config.php` is optional and unchanged. Use `--provider aws` for ECS
Fargate or `--provider azure` for AKS. `init` selects a PHP branch for a known
release and defaults to PHP 8.5 for an unknown line. The bundled nginx-fpm
recipe builds PHP 8.3, 8.4 or 8.5 according to `build.php`. Review placeholders,
commit `magelift.php`, then bootstrap the environment you intend to use.

The project file is executable PHP. Review it before running MageLift, as you
would any other code from a repository. Environment commands write managed
changes to the adjacent `magelift.environments.php`; keep it with the project
in version control when those changes are needed by CI.

Check provider constraints and preview capacity changes before applying them.
An available service version does not guarantee regional capacity.

## Build the shop

Run `magelift build` from the shop repository after committing the PHP project, the Composer
lock and existing module configuration. The bundled nginx-fpm recipe supports PHP 8.3, 8.4, and 8.5 on Linux amd64. Recipes and pinned upstream inputs ship with
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
magelift --config magelift.php --env staging config validate
magelift --config magelift.php --env staging preview
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
An untagged MAGELIFT_RELEASE_IMAGE repository gets the selected source SHA tag;
an explicitly tagged destination is preserved, including registries with ports.
Artifact handoff uses repository@sha256 references: the publication tag is
removed from the reference passed to deploy, while its digest stays unchanged.
The generated build job installs a pinned Docker Engine before Buildx; it
does not depend on the runner image's older bundled Docker version.

The generated workflow validates pull requests without cloud credentials. A
reviewer applies `magelift-preview` to a same-repository PR targeting `main`
to build its current head. That build can access configured build credentials,
including Composer secrets; apply the label only to code you trust. Remove and
reapply the label after reviewing a new head; removing it also requests preview
cleanup. Protect the `build`,
`preview-build`, `preview` and stable deployment environments in GitHub.
Configure provider credentials per environment. Azure deploy jobs need the
state-account and tenant variables and Pulumi passphrase secret described in
`docs/bootstrap.md`.
