---
type: lesson
title: MageLift live pricing and CLI upgrade contracts 2026-07-18
description: MageLift keeps account-free cost output as the default and adds cost --live through the AWS
  Price List API in us-east-1, filtering the target region and paginating GetProducts.
tags:
- cost
- pricing
- upgrade
- release
- security
generated:
  at: '2026-07-24'
---

MageLift keeps account-free cost output as the default and adds cost --live through the AWS Price List API in us-east-1, filtering the target region and paginating GetProducts. It prices bounded on-demand capacity inputs, keeps serverless/workload-dependent capacity in estimated items, leaves data transfer, requests, storage growth, logs, WAF, CloudFront, and NAT unsupported, and never treats a budget as a forecast. MageLift upgrade queries GitHub releases, validates semantic tags and HTTPS assets, verifies checksums.txt, extracts the matching archive binary, and atomically replaces the executable. Release Please uses a separate RELEASE_PLEASE_TOKEN because GITHUB_TOKEN-created tags do not trigger another workflow; GoReleaser creates archives, checksums, SBOMs, Homebrew casks, and the release workflow attests the checksum manifest.
