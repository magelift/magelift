---
status: draft
slug: copy-link-application-image
---

# Intent: independent Magento rootfs layer on application images

## Problem

The application Dockerfile copies Magento `rootfs/` with `COPY --chown`. That copy is tied to the preceding `env.php` stash `RUN`. Rebuilds that only change shop files still wait on earlier layers. Dockerfile frontend 1.27 already allows `COPY --link`. Magelift did not take it because the env.php stash/restore can break if `--link` replaces `/app` instead of overlaying it.

## Evidence

`internal/build/pipeline/assets/application.Dockerfile` stashes `/app/app/etc/env.php`, then `COPY --chown=10001:10001 rootfs/ /app/`, then restores env.php. Syntax is `docker/dockerfile:1.27`. Whether `--link` plus `--chown` keeps the runtime env.php contract: not checked on a real Magento rootfs.

## Proposed outcome

Shop-file-only rebuilds reuse the Magento copy layer when the runtime base and env.php dance are unchanged. Built images still pass the env.php secret scan and Magento binary checks in that Dockerfile. No change to how operators invoke `magelift build`.

## Affected users and systems

`magelift build` application images. `internal/build/pipeline`. Image health and Floci fixtures that consume those images.

## Constraints

Keep the runtime `env.php` from the pinned runtime base, not Magento's build-time file. No secrets in the image. Dockerfile frontend stays a pinned 1.x (currently 1.27). Do not require labs channel.

## Out of scope

Rewriting php-runtime or FrankenPHP Dockerfiles for `--link`. Changing Magento compile. Multi-stage Magento builds.

## Open questions

Does `COPY --link --chown` onto an existing `/app` from `RUNTIME_BASE` overlay or replace? If `--link` cannot keep the env.php restore, is independent layering still worth a different stash path?
