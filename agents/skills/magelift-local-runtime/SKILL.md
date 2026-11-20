---
name: magelift-local-runtime
description: >-
  Start a Magento shop with MageLift and Docker. Use for local service settings, first install, logs, and stopping or resetting containers.
version: 1.0.0
---

# Run Magento locally

Install the shop's Composer dependencies, then start the local environment.

```sh
magelift doctor --local
magelift dev
```

`dev` prepares Compose, installs Magento if the local database is empty and
checks GraphQL. A generated admin password is stored in the ignored,
mode-0600 `.magelift/local.env` file. Repeating `dev` reconciles generated
Compose and PHP settings. If a generated file was manually edited, review and
move it aside before retrying. `MAGELIFT_LOCAL_APP_HTTP_PORT` changes the local
HTTP port. `MAGELIFT_LOCAL_BASE_URL` overrides the URL.

For separate steps, use `magelift local init`, `magelift local up --service app`
and `magelift local seed`. `local init` writes Compose using the selected
environment’s service versions.

Set local choices in the `local` block. Service selections also come from
the project's `services` settings. Put the variable names in `magelift.php`
and keep their credential values in the environment.

Add this fragment inside the existing project array in `magelift.php`:

```php
'local' => [
    'phpSettings' => ['memory_limit' => '1G'],
    'email' => ['mode' => 'mailpit'],
],
```

`mailpit` starts a digest-pinned SMTP sink on loopback. For an authenticated relay, use `credentialEnv` to name the environment
variable that supplies its password.

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

If a production service has a different local substitute, explain the
difference and check the production service in staging.
