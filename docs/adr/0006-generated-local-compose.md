# ADR 0006. Generate Compose for local development

Accepted on 2026-08-22.

Magento developers can start the shop before connecting a cloud account.
`magelift dev` resolves the project, generates Compose, starts the selected
services and checks the store. A checked-in Compose file could drift from
the shop's service settings.

`magelift local init` writes `.magelift/compose.local.yml` and a private
`.magelift/local.env`. The app profile mounts the shop source into the
nginx/PHP-FPM runtime. Search and queue have their own profiles. Host ports
bind to loopback, and the HTTPS listener supports local cookie checks.

`magelift local seed` installs into an empty local database and protects an
existing `app/etc/env.php`. `magelift local down` keeps volumes so the next
startup can reuse the database. Use `magelift local reset --yes` when you
intend to remove the local data. [Local and cloud environments](../local-vs-cloud.md)
