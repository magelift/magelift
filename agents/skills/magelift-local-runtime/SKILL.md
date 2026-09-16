---
name: magelift-local-runtime
description: >-
  Create and operate MageLift's local Magento Docker environment with the
  release-specific PHP, Composer, database, cache, search, and queue contract.
  Use when starting local development or checking a local compatibility gap.
version: 1.0.0
---

# Run Magento locally

Create the Compose file from the project configuration before starting
containers:

```sh
magelift doctor
magelift local init
magelift local up --service app
```

`local init` resolves a source-dated compatibility row. It pins the local service
images, records the selected PHP and Composer inputs, and creates the ignored
mode-0600 `.magelift/local.env` file. It never changes the cloud target.

Set local choices in the `local` block when the catalog supports them. Keep
email credentials as environment-variable names; do not put secret values in
`magelift.yaml`:

```yaml
local:
  database:
    family: mariadb
    version: "11.8"
  email:
    mode: mailpit
```

`mailpit` starts a digest-pinned SMTP sink on loopback. SMTP and SES
still take `credentialEnv` names, never secret values in `magelift.yaml`.

Seed a project only after installing its Composer dependencies and supplying a
local administrator password through the process environment:

```sh
export MAGELIFT_LOCAL_ADMIN_PASSWORD='use-a-local-password-with-16-or-more-letters'
magelift local seed
```

Useful day-to-day commands are `magelift local status`, `magelift local logs
--service app`, and `magelift local exec --service app -- bin/magento cache:flush`.
Use `magelift local reset --yes` only when removing the local database and other
Compose volumes is intended.

If planning reports an unsupported Redis, provider-managed service, PHP
setting, or extension contract, keep the explicit gap visible. Do not silently
substitute a different production service family.
