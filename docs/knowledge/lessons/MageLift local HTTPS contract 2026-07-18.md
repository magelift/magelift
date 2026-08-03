---
type: lesson
title: MageLift local HTTPS contract 2026-07-18
description: The FrankenPHP classic image now copies its Caddyfile to the official /etc/frankenphp/Caddyfile
  path, exposes HTTP 8080 and a localhost-only HTTPS listener on 8443 using tls internal, stores certif...
tags:
- local-development
- frankenphp
- https
- caddy
- failure-correction
status: stable
generated:
  at: '2026-07-24'
---

The FrankenPHP classic image now copies its Caddyfile to the official /etc/frankenphp/Caddyfile path, exposes HTTP 8080 and a localhost-only HTTPS listener on 8443 using tls internal, stores certificate material in /tmp/magelift-caddy, disables host trust-store installation, and gives the non-root user ownership of /config and /data. Generated local Compose maps MAGELIFT_LOCAL_HTTPS_PORT (default 8443) to the container. The listener is for local secure-cookie and integration testing only; it does not prove production TLS. A first implementation copied /etc/caddy/Caddyfile and failed because the FrankenPHP entrypoint loads /etc/frankenphp/Caddyfile; a live TLS smoke test caught and fixed this.
