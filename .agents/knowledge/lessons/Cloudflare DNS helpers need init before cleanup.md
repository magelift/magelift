---
type: lesson
title: Cloudflare DNS helpers need init before cleanup
description: Sourcing lib-cloudflare-dns.sh does not set CLOUDFLARE_ACCEPTANCE_ZONE; call cloudflare_acceptance_dns_init first or set -u aborts cleanup.
tags:
- gcp
- cloudflare
- destroy
- acceptance
generated:
  by: cursor-grok
  at: '2026-08-18'
---

`scripts/acceptance/lib-cloudflare-dns.sh` defines `cloudflare_acceptance_dns_cleanup_a` without calling `cloudflare_acceptance_dns_init`. The harness sources the lib and inits during startup. A one-off destroy script that only sources the lib under `set -u` dies at `CLOUDFLARE_ACCEPTANCE_ZONE: unbound variable` after GCP orphans are already gone. Call `cloudflare_acceptance_dns_init` before any validate/cleanup helper. `|| true` does not save a nounset abort in the function. Relates to GCP PSA force_clean after Magento KEEP cells.
