---
type: lesson
title: GKE Magento runtime health probes LoadBalancer HTTP when applicationURL is empty
description: Autopilot SkipAwait leaves applicationURL empty. kube Observe GETs the Service LoadBalancer IP as runtime.web and treats a localhost redirect as unhealthy.
tags: [gcp, gke, magento, health]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-19
---

`gcap27` catalog health was kube-only because Autopilot SkipAwait never
filled `applicationURL`. `CheckRuntime` now appends `runtime.web` when the
web Service has a non-loopback LoadBalancer IP (or when `applicationURL` is a
usable http(s) URL). A `302` to `localhost` is unhealthy. No URL omits the
Magento check instead of guessing.

Stack kubeconfig is still a one-hour OAuth token. Live `magelift health` after
that window needs `MAGELIFT_KUBECONFIG` from `gcloud container clusters
get-credentials`.

## Related

See [GCP Autopilot destroy after one hour uses DELETE_UNREACHABLE](GCP%20Autopilot%20destroy%20after%20one%20hour%20uses%20DELETE_UNREACHABLE.md).
