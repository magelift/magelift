---
type: lesson
title: example.com Cloudflare 403s Google Front End IPs
description: GCP alias HTTPS through an Internet FQDN NEG to example.com completed TLS then returned Cloudflare 403; CloudFront can 200 the same origin because Cloudflare allows it.
tags: [gcp, edge, cloudflare, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-13
---

Cell `20260813u` reached Google-managed cert ACTIVE. After Google Front Ends
began serving TLS, `curl --resolve` to the forwarding IP returned HTTP 403 with
body `<center>cloudflare</center>`. Direct `https://example.com/` from the
runner is 200. CloudFront alias `20260813t` also got 200 through example.com.

Cloudflare bot/IP controls 403 Google Front End source addresses. example.com
is not a valid GCP alias-HTTPS origin. Use a non-Cloudflare HTTPS origin that
returns 2xx on `/` (the wrapper uses `www.google.com` when alias traffic is
on and the origin is still `example.com` or `www.wikipedia.org`), and send
`Host: <origin>` as a backend custom request header so the load balancer does
not present the Magelift hostname to that origin.

`www.wikipedia.org` 403s Go's default User-Agent (`Go-http-client/1.1`) and
is not a valid origin-health target until the probe sends
`User-Agent: MageLift-origin-health/1`. The alias origin for that path is
`www.google.com`.
