---
type: lesson
title: ACM DomainValidationOptions JSON can exist before ResourceRecord
description: A non-null DomainValidationOptions array is not enough to publish CNAMEs; wait until every option has ResourceRecord.Name and Value.
tags: [acm, dns, cloudflare, acceptance]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-13
---

# ACM DomainValidationOptions JSON can exist before ResourceRecord

`aws acm describe-certificate --query Certificate.DomainValidationOptions`
returns a JSON array as soon as the certificate exists. `ResourceRecord` is
often still null on that first response.

A wait loop keyed on `options != "" && options != "null"` exits immediately,
publishes zero Cloudflare CNAMEs, then burns the 20-minute ISSUED timeout
(`PENDING_VALIDATION`). That is what Magento Fargate Spot `20260813ak` did.

Wait until every validation option has `ResourceRecord.Name` and
`ResourceRecord.Value`. Refuse the ISSUED wait if no CNAME was published.
The CloudFront wrapper already waits on `DomainValidationOptions[0].ResourceRecord`.

# Related

* Relates to: [Cleanup ledger run IDs must be unique across providers](Cleanup%20ledger%20run%20IDs%20must%20be%20unique%20across%20providers.md)
