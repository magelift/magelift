---
type: lesson
title: Google-managed SSL certificates can take longer than 20 minutes
description: A Google-managed cert stays PROVISIONING until public DNS points at the forwarding IP; twenty minutes after the A record is not a TLS failure.
tags: [gcp, edge, tls, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-13
---

Cell `20260813p` applied a global HTTPS forwarding rule, created the Cloudflare
A record, then destroyed the stack because the managed certificate was still
not ACTIVE after 20 minutes. Independent inventory was empty. That is a wait
budget miss, not proof that managed TLS cannot complete.

Cell `20260813r` used a 60-minute wait. Public DNS-only A records matched the
forwarding IP on 8.8.8.8 and 1.1.1.1 while `managed.domainStatus` was still
`FAILED_NOT_VISIBLE`. The certificate later reached ACTIVE. Treat
`FAILED_NOT_VISIBLE` as retryable while `managed.status` is `PROVISIONING` and
public DNS is already correct.

Wait for ACTIVE after DNS is published. Keep TTL larger than the cert wait
plus apply and destroy. Do not leave a forwarding rule if the cert never
becomes ACTIVE.

Native Cloud CDN edge is HTTPS-only. Empty reply on port 80 is expected and
is not a managed-cert failure. See [GCP native edge managed certs wait on DNS
and 443 not port 80](GCP%20native%20edge%20managed%20certs%20wait%20on%20DNS%20and%20443%20not%20port%2080.md).

ACTIVE is not the same as the load balancer serving TLS. After ACTIVE, Google
Front Ends can take up to 30 minutes more before HTTPS 200. Wait for the data
plane, not only the certificate resource.
