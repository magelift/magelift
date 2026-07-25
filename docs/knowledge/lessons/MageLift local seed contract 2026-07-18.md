---
type: lesson
title: MageLift local seed contract 2026-07-18
description: The local development workflow now includes magelift dev seed.
tags:
- local-development
- secrets
- docker-compose
- magento
status: deprecated
generated:
  at: '2026-07-24'
---

The local development workflow now includes magelift dev seed. It requires an existing bin/magento and generated Compose file, starts the app profile, runs setup:install through a fixed shell wrapper with all non-secret options as positional arguments, and supplies MAGELIFT_LOCAL_ADMIN_PASSWORD from the ignored mode-0600 .magelift/local.env. Passwords are restricted to 16-128 alphanumeric characters, never printed, and never placed in Docker command arguments. The command refuses to overwrite app/etc/env.php and is non-interactive. Docker Compose env_file paths are relative to the generated Compose file directory.

# Related

* Supersedes: Projects/magelift/Lessons/MageLift runtime env.php injection remains an implementation gap 2026-07-18
