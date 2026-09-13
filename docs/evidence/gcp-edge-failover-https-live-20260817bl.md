# GCP native edge URL-map failover HTTPS — 2026-08-17 (`20260817bl`)

Status: **PASS** for health-gated two-origin URL-map failover through viewer
HTTPS. Magento origin, Cloud Armor, failback, and 5xx automatic backend
failover remain open.

| Field | Value |
| --- | --- |
| Project | `<gcp-project>` |
| Run ID | `20260817bl` |
| Marker | `magelift/gcp/edge/20260817bl` |
| Domain | `ml-gcp-edge-20260817bl.example.test` |
| Forwarding IP | `<redacted>` (destroyed) |
| Primary origin | `www.google.com` Internet FQDN NEG |
| Secondary origin | `developers.google.com` Internet FQDN NEG |
| Native provider | `cloud-cdn` |
| Cloud Armor | not attached |
| trafficImpact | `failover-https` |
| Pre-failover title | `Google` |
| Post-failover title | `Google for Developers \| Build with Gemini` |
| rtoSeconds | `116` |
| Wrapper | `scripts/gcp-edge-acceptance-local.sh` |
| Wall clock | 3127.5s (exit 0) |

## Observed result

1. The wrapper created two CDN-enabled backend services and Internet FQDN
   NEGs, then the Go cell applied the owned URL map, target HTTPS proxy,
   forwarding rule, and purge. Both origin health probes used
   `MageLift-origin-health/1` before apply and before failover.
2. Public DNS matched the forwarding IP while the managed certificate was still
   `PROVISIONING` / `FAILED_NOT_VISIBLE`. The certificate reached `ACTIVE`.
   Alias HTTPS then returned `SSL_ERROR_SYSCALL` until Google Front End lag
   cleared, then `200`.
3. Viewer HTML through the alias matched the primary title `Google`. Failover
   updated the URL-map default backend to the secondary service, invalidated
   `/*`, and the alias title became `Google for Developers | Build with Gemini`
   in 116 seconds.
4. Destroy removed the owned URL map, HTTPS proxy, and forwarding rule.
   Wrapper deleted the certificate, both backend services, both NEGs, and the
   DNS record. Independent post-run `gcloud` lists for the run marker were
   empty. Domain A lookup was empty.

The Go destroy line still prints `trafficImpact=not-run`; the wrapper value
`failover-https` is the cell outcome.

## Explicit non-claims

- Magento origin
- Cloud Armor attach or data plane
- Failback / rollback traffic
- Automatic 5xx or health-check-driven load-balancer failover without an
  operator URL-map transition
- Physical zone failure

## Related failed attempt

`20260817bk` reached alias HTTPS `200` and the primary title, then refused
failover because the pre-transition ownership check required the URL map to
already point at the destination backend. Cleanup emptied that run. Do not
reuse `bk`.
