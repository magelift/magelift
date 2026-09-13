---
type: lesson
title: GCP native edge managed certs wait on DNS and 443 not port 80
description: Native Cloud CDN edge is HTTPS-only, so curl to :80 is empty. Keep Cloudflare DNS-only, treat FAILED_NOT_VISIBLE as retryable, and wait up to 3600s after public DNS matches the forwarding IP.
tags: [gcp, edge, tls, cloudflare, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-18
sources:
  - id: google-managed-certs
    resource: https://cloud.google.com/load-balancing/docs/ssl-certificates/google-managed-certs
    title: Use Google-managed SSL certificates
  - id: google-ssl-troubleshooting
    resource: https://cloud.google.com/load-balancing/docs/ssl-certificates/troubleshooting
    title: Troubleshoot SSL certificates
---

`gcap24` Magento origin HTTPS was already working while the Cloud CDN Google-managed
certificate stayed `PROVISIONING`. `curl` to the CDN forwarding IP on port 80
returned an empty reply. That is the native edge adapter, not a missing HTTP-01
listener.

The adapter inserts one global forwarding rule with `PortRange: "443"`
(`internal/cloud/gcp/edge/client.go`). Google's troubleshooting page requires
port 443 on the target HTTPS proxy, not port 80. HTTPS-only cells
`20260813r` and `20260813y` reached `ACTIVE` without an HTTP frontend. Do not
abort the wait, add a port-80 forwarding rule, or classify the cell as a TLS
failure because `:80` is silent.

There are two certificates. They are not interchangeable:

- GKE `ManagedCertificate` on the origin Ingress (`kubectl get managedcertificate`).
  `FailedNotVisible` is retryable until
  `MAGELIFT_GCP_ACCEPTANCE_ORIGIN_CERT_WAIT_SECONDS` (default 3600).
- Compute `sslCertificates` on the CDN front end
  (`gcloud compute ssl-certificates describe --global`).
  `FAILED_NOT_VISIBLE` is retryable while `managed.status` is `PROVISIONING`
  until `MAGELIFT_GCP_EDGE_CERT_WAIT_SECONDS` (default 3600).

Origin `Active` does not mean the CDN cert is `ACTIVE`.

Next time, before extending or killing the wait:

1. Cloudflare A is DNS-only (`proxied: false`). Orange-cloud puts Cloudflare in
   the CA path; Google documents that a CDN in front of the load balancer can
   fail multi-perspective validation.
2. `dig @8.8.8.8` and `dig @1.1.1.1` return only the forwarding IP. Extra A
   records or a mismatched AAAA keep the domain `FAILED_NOT_VISIBLE`.
3. The certificate URL is on the target HTTPS proxy and the forwarding rule is
   TCP 443 to that proxy.
4. Leave `:80` empty. Poll `managed.status` / `managed.domainStatus`, not HTTP
   on 80.

Do not exit on `FAILED_NOT_VISIBLE` while status is still `PROVISIONING` and
those four checks pass. After `ACTIVE`, Google Front Ends can still reset TLS
for up to 30 minutes. If the checklist stays green and 3600s expire still
`PROVISIONING`, that is the same provider-gated issuance class as
`20260815-edge-1` / `20260815-edge-2`, not a missing port-80 fix.

See [Google-managed SSL certificates can take longer than 20 minutes](Google-managed%20SSL%20certificates%20can%20take%20longer%20than%2020%20minutes.md).
