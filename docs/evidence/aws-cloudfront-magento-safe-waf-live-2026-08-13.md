# AWS CloudFront Magento-safe WAF — 2026-08-13

This is a bounded CloudFront plus WAFv2 cell. It proves ACM DNS issuance, a
CLOUDFRONT-scope Magento-safe WebACL (`waf/magento-safe`: managed groups in
protect mode with Magento BODY/URI/QUERY count-overrides and 64 KB CloudFront
body inspection), origin-group apply with that WebACL attached, Deployed
polling, `/*` invalidation, disable-then-delete destroy, and exact
ACM/WAF/distribution cleanup. It does not claim Magento traffic, alias DNS
routing, WAF block data-plane hits, failover traffic impact, or measured edge
RTO.

The earlier `20260813s` cell remains attach-only whole-group CommonRuleSet
count mode. `20260813v` issued ACM then failed `CreateWebACL` on AssociationConfig
map key `CloudFront`; that run ID is not reused.

## Scope

| Field | Value |
| --- | --- |
| Run ID | `20260813w` |
| Marker | `magelift/aws/cloudfront/20260813w` |
| Domain | `ml-cf-20260813w.example.test` |
| ACM | `arn:aws:acm:us-east-1:<aws-account>:certificate/d439fbc2-e6b3-44ac-9bfd-0c9db795061e` |
| WebACL | `magelift-cf-waf-20260813w` `72604bca-4d1b-4c1e-9170-c81d23b4917a` |
| WebACL ARN | `arn:aws:wafv2:us-east-1:<aws-account>:global/webacl/magelift-cf-waf-20260813w/72604bca-4d1b-4c1e-9170-c81d23b4917a` |
| Distribution hostname | `d3rlll5vii9uia.cloudfront.net` |
| Origins | `example.com` / `www.example.com` origin group |
| WAF policy | `waf/magento-safe` (groups `OverrideAction.None`, Magento count-overrides including `SizeRestrictions_BODY` and `SQLi_BODY`, `DefaultSizeInspectionLimit=KB_64`) |
| Wrapper | `scripts/aws-cloudfront-acceptance-local.sh` |
| Wall clock | 461.9s (exit 0) |

## Observed result

1. ACM DNS validation issued.
2. `CreateWebACL` succeeded with AssociationConfig key `CLOUDFRONT`. Direct
   `GetWebACL` after create confirmed Magento-safe protect mode
   (`bodyInspection=KB_64`, no whole-group Count, Magento BODY overrides in
   Count). The WebACL was attached; apply proofs included `aws.cloudfront.waf`.
   Distribution reached Deployed as `d3rlll5vii9uia.cloudfront.net`.
3. Purge completed, then disable-then-delete destroy.
4. Independent post-run checks: no magelift-owned distributions; `GetWebACL` for
   `72604bca-…` returned `WAFNonexistentItemException`; `DescribeCertificate` for
   the ACM ARN returned `ResourceNotFoundException`; Cloudflare validation CNAME
   list was empty.

The run reports `cleanup=verified waf=attached trafficImpact=not-run`.

## Explicit non-claims

- Viewer traffic through the alias
- WAF block/count data-plane hits
- Origin authentication
- Failover RTO and rollback traffic
- Magento origin routing
