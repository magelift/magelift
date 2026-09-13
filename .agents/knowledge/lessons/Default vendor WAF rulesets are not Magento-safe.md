---
type: lesson
title: Default vendor WAF rulesets are not Magento-safe
description: Magento 2 needs count-overrides and 64 KB body inspection; unmodified AWS CommonRuleSet, Cloud Armor sensitivity 4, and generic CRS PL2+ block admin WYSIWYG, uploads, and GraphQL.
tags: [waf, magento, aws, gcp, fastly, edge]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-13
sources:
  - id: adobe-fastly-waf
    resource: https://experienceleague.adobe.com/en/docs/commerce-on-cloud/user-guide/cdn/fastly-waf-service
    title: Adobe Commerce Fastly WAF
  - id: adobe-graphql-waf
    resource: https://experienceleague.adobe.com/en/docs/commerce-knowledge-base/kb/how-to/how-to-bypass-waf-for-graphql-requests
    title: Adobe KB GraphQL WAF false positives
  - id: aws-upload-waf
    resource: https://repost.aws/knowledge-center/waf-upload-blocked-files
    title: AWS re:Post WAF file-upload false positives
  - id: aws-body-limit
    resource: https://docs.aws.amazon.com/waf/latest/developerguide/web-acl-setting-body-inspection-limit.html
    title: AWS WAF CloudFront body inspection limit
  - id: armor-tuning
    resource: https://docs.cloud.google.com/armor/docs/rule-tuning
    title: Cloud Armor WAF sensitivity and exclusions
  - id: armor-graphql
    resource: https://docs.cloud.google.com/armor/docs/content-parsing
    title: Cloud Armor JSON and GraphQL parsing
---

Do not attach an unmodified AWS `AWSManagedRulesCommonRuleSet`, Cloud Armor
sensitivity-4 OWASP set, or generic CRS paranoia 2+ to a Magento 2 edge.

Magento admin WYSIWYG posts HTML, catalog import and media exceed the AWS 8 KB
`SizeRestrictions_BODY` / 16 KB CloudFront default, GraphQL JSON repeats
characters that trip SQLi/XSS, and Magento admin URIs look like file paths.
AWS documents count-overrides for `SizeRestrictions_BODY`,
`CrossSiteScripting_BODY`, `SQLi_BODY`, and `GenericLFI_BODY` on uploads, and
allows raising CloudFront inspection to 64 KB. Cloud Armor should run
sensitivity 1 with `STANDARD_WITH_GRAPHQL` parsing. Adobe's Fastly Magento WAF
is a Magento-tuned policy on cache misses, including admin; do not default to
bypassing `/graphql`. Magento admin front names are customized, so do not
hardcode `/admin` in rate-limit paths.

The portable policy is `internal/edge/waf` (`waf/magento-safe`). The 2026-08-13
`20260813s` cell proved WAF attach only with whole-group count-mode CommonRuleSet
and is not this policy. The 2026-08-13 `20260813w` cell attached the Magento-safe
WebACL live (`OverrideAction.None` on managed groups, Magento BODY count-overrides,
`KB_64`) and left empty inventories.
