---
type: lesson
title: GCP Magento candidate health needs LoadBalancer base_url
description: Seed dumps keep Magento base_url on localhost; CheckRuntime treats a LoadBalancer 302 to localhost as unhealthy until config:set + cache:flush.
tags: [gcp, gke, magento, health, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-20
---

# GCP Magento candidate health needs LoadBalancer base_url

Installed-schema seeds ship `web/unsecure/base_url` and
`web/secure/base_url` as `http://localhost:8080/`. After
`deploy:candidate` migrate/cutover, GKE Standard (and Autopilot once the
Service has an IP) exposes a LoadBalancer. `CheckRuntime` GETs that origin
and records `runtime.web` unhealthy on a loopback redirect.

`gcha32` passed kube-only `day2:health`, search, and `migrate:dump`, then
failed candidate health for ~10 minutes on `Magento HTTP redirected to
localhost`. `gcap27` later got `GET /` 200 after Magento `config:set` of
both base URLs to the public LB and `cache:flush`.

`deploy:candidate` now applies the LoadBalancer URL before
`run_runtime_health_with_retry`. Native `edge:traffic` may overwrite with
`https://ORIGIN_DOMAIN/` afterward. Magento HTTP 200 stays required after
candidate, not on `day2:health`.

See [GKE Magento runtime health probes LoadBalancer HTTP when applicationURL is empty](GKE%20Magento%20runtime%20health%20probes%20LoadBalancer%20HTTP%20when%20applicationURL%20is%20empty.md).
