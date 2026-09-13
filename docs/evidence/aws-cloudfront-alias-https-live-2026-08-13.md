# AWS CloudFront alias HTTPS acceptance — 2026-08-13

This is a bounded CloudFront alias-traffic cell. It proves ACM DNS issuance,
origin-group apply, a Cloudflare CNAME from the custom domain to the
distribution hostname, HTTPS `200` through that alias, disable-then-delete
destroy, and exact ACM/DNS/distribution cleanup. It does not claim Magento
origin routing, WAF block hits, failover RTO, or measured edge RTO.

## Scope

| Field | Value |
| --- | --- |
| Run ID | `20260813t` |
| Marker | `magelift/aws/cloudfront/20260813t` |
| Domain | `ml-cf-20260813t.example.test` |
| ACM | `arn:aws:acm:us-east-1:<aws-account>:certificate/93dfea92-aaf6-40c5-bd09-9a7e350c38ed` |
| Distribution hostname | `d30djbjlwm0jtk.cloudfront.net` |
| Origins | `example.com` / `www.example.com` origin group |
| Alias HTTPS | `200` |
| WAF | not attached |
| Wrapper | `scripts/aws-cloudfront-acceptance-local.sh` |
| Wall clock | 415.7s (exit 0) |

## Observed result

1. ACM DNS validation issued.
2. Apply reached Deployed as `d30djbjlwm0jtk.cloudfront.net`.
3. Cloudflare CNAME `ml-cf-20260813t.example.test` → that hostname; `curl`
   `https://ml-cf-20260813t.example.test/` returned HTTP 200 with verified TLS.
4. Destroy disabled then deleted the distribution. Independent post-run
   checks: no magelift-owned distributions; ACM ARN
   `ResourceNotFoundException`; no `ml-cf-*` certificates.

The wrapper reports `cleanup=verified trafficImpact=alias-https`. The Go
destroy line still prints `trafficImpact=not-run` because alias proof is
wrapper-owned.

## Explicit non-claims

- WAF attach or block data-plane (covered separately by `20260813s`)
- Failover RTO and rollback traffic
- Origin authentication
- Magento origin routing
