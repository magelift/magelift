# MageLift Fastly adapter live acceptance — 2026-08-13

This is a disposable-account replacement-credential proof for the registered
`internal/external/fastly` adapter after Fastly CLI login was restored. It is
not a general Fastly certification claim and does not close WAF, cache-policy,
failover, rollback, Magento origin, or provider-wide edge coverage.

## Scope

The run used `scripts/fastly-acceptance-local.sh` with:

- Fastly CLI login session (no `FASTLY_API_TOKEN`, no `MAGELIFT_FASTLY_TOKEN_NAME`)
- disposable domain `ml-fastly-20260813.example.test`
- ownership marker `magelift/acceptance/live-20260813-authfix`
- HTTPS origin `https://magelift.dev/llms.txt` (product docs host, not Magento)
- Fastly CNAME target `x.sni.global.fastly.net`
- managed TLS / ACME DNS challenge through the Cloudflare acceptance boundary

The adapter created a new VCL service, origin backend, route snippet,
versionless domain, Fastly-managed TLS subscription, and purge request, then
removed only the marked resources.

`TestLiveFastlyAdapter` passed in 134.83 seconds. Wall clock for the wrapper,
including DNS prepare/cleanup, was 160.8 seconds.

The named stored token `default` still returned HTTP 401. The cell succeeded
only after MageLift stopped injecting `--token default` unless
`MAGELIFT_FASTLY_TOKEN_NAME` is set.

## Proofs

| Proof | Result |
| --- | --- |
| Replacement Fastly CLI login (omit `--token`) | PASS |
| Healthy origin preflight | PASS |
| Fastly backend and provider-owned route snippet | PASS |
| Managed TLS subscription and ACME DNS challenge | PASS |
| Assigned TLS route CNAME and HTTPS response through Fastly | PASS |
| Purge requested before verification | PASS |
| Exact ownership marker on created resources | PASS |
| Deferred domain/service/TLS teardown | PASS |
| Direct marked Fastly service inventory after teardown | 0 |
| Direct marked Fastly versionless-domain inventory after teardown | 0 |
| Direct TLS-subscription inventory after teardown | 0 |
| Exact disposable Cloudflare route and ACME records after teardown | 0 |
| Pre-existing unmarked Fastly service | preserved |

Independent post-run inventories (CLI login session, no `--token default`):

| Inventory | Result |
| --- | --- |
| Fastly services total | 1 |
| Services with marker `magelift/acceptance/live-20260813-authfix` | 0 |
| Unmarked service `yrrMKM93YoJjtikP1OGfXO` | preserved |
| Versionless domains for `ml-fastly-20260813.example.test` | 0 |
| TLS subscriptions for that domain | 0 |
| Cloudflare CNAME at the disposable hostname | 0 |
| Cloudflare CNAME at `_acme-challenge.ml-fastly-20260813.example.test` | 0 |

## Non-claims

- Magento application routing, cache-key/bypass policy, WAF, origin authentication, failover, rollback, and measured edge RTO
- Named stored token `default` (still 401; not used)
- Native CloudFront/GCP/Scaleway/OVH edge composition
