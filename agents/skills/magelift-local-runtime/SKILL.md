---
name: magelift-local-runtime
description: >-
  Create and operate MageLift's local Magento Docker environment with the
  release-specific PHP, Composer, database, cache, search, and queue contract.
  Use when starting local development or checking a local compatibility gap.
version: 1.0.0
---

# Run Magento locally

After installing the shop's Composer dependencies, start the local environment:

```sh
magelift doctor --local
magelift dev
```

`dev` prepares Compose, installs Magento if the local database is empty and
checks GraphQL. A generated admin password is stored in the ignored,
mode-0600 `.magelift/local.env` file. Repeating `dev` reconciles generated
Compose and PHP settings. If a generated file was manually edited, review and
move it aside before retrying. `MAGELIFT_LOCAL_APP_HTTP_PORT` changes the local
HTTP port; `MAGELIFT_LOCAL_BASE_URL` overrides the URL.

For separate steps, use `magelift local init`, `magelift local up --service app`
and `magelift local seed`. `local init` resolves a source-dated compatibility
row and does not change the cloud target.

Set local choices in the `local` block. Service selections also come from
the project's `services` settings. Keep email credentials as environment-variable
names; do not put secret values in `magelift.php`.

Add this fragment inside the existing project array in `magelift.php`:

```php
'local' => [
    'phpSettings' => ['memory_limit' => '1G'],
    'email' => ['mode' => 'mailpit'],
],
```

`mailpit` starts a digest-pinned SMTP sink on loopback. SMTP and SES
still take `credentialEnv` names, never secret values in `magelift.php`.

Seed a project only after installing its Composer dependencies and supplying a
local administrator password through the process environment:

```sh
export MAGELIFT_LOCAL_ADMIN_PASSWORD='LocalPassword123456'
magelift local seed
```

For a fresh shop using Smile ElasticSuite, install the extension and use an OpenSearch image with the ICU and phonetic analysis plugins. Run `magelift dev --search-engine elasticsuite` or pass the same flag to `magelift local seed`. An existing installation keeps the search adapter selected in Magento.

Useful day-to-day commands are `magelift local status`, `magelift local logs
--service app`, and `magelift local exec --service app -- bin/magento cache:flush`.
Use `magelift local reset --yes` only when removing the local database and other
Compose volumes is intended.

If planning reports an unsupported Redis, provider-managed service, PHP
setting, or extension contract, keep the explicit gap visible. Do not silently
substitute a different production service family.
