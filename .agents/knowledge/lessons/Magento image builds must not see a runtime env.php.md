---
type: lesson
title: Magento image builds must not see a runtime env.php
description: DI compile and static content run in a CLI builder with no database. The runtime env.php and its auto-prepend stay on the serving image.
tags: [magento, docker, build, env-php]
status: stable
generated:
  by: cursor/devbox
  at: '2026-09-21'
---

# Magento image builds must not see a runtime env.php

A working production Magento shop builds one image that contains code, `vendor`, DI compile, and static content. That build has no database. `app/etc/env.php` is not in the image.

The builder is a PHP CLI image. It is not the runtime image. Composer install uses a BuildKit secret mount for `auth.json` and a cache mount for the Composer download cache. Application source is copied after the lockfile layer, keyed by the git SHA, so a code change does not rebuild PHP extensions. Then a build script runs, still with no database:

1. Refresh modules (`module:enable --all`) so vendor modules exist in `config.php` before compile.
2. `setup:di:compile`.
3. `setup:static-content:deploy`.

`setup:upgrade`, config import, and cache work stay out of this phase. They need a database and a real `env.php`.

The runtime PHP ini auto-prepends a script that resolves `env.php` placeholders, including the install date and `127.0.0.1`, into `MAGENTO_DC__OVERRIDE`. Magento then treats the process as an installed shop and opens a database connection. The builder stage therefore starts from the extension image, with its own ini and no `env.php`. The serving image keeps the template. The application Dockerfile puts that template back after the build, because Magento may write a cache-only `env.php` during compile.

A composer skeleton's `config.php` lists modules and no websites. `setup:static-content:deploy` resolves the default website from that file and fails with "The default website isn't defined" when it is absent. Before static content, write Magento's single-store scaffold (admin plus base, `is_default` on base) only when `config.php` has no websites. A shop that already dumped a default website is left as it is. Websites with no default are an error, not something to patch over.

Write `env.php` only when a database and the crypt key exist. Do not put an install date into a file the compiler will load.

Related: [Magento deploy writes env.php before traffic moves](Magento%20deploy%20writes%20env.php%20before%20traffic%20moves.md).
