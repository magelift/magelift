---
type: lesson
title: AWS live acceptance traps
description: AWS live acceptance fails first on bootstrap-before-harness, base64 cache secrets, VPC OpenSearch admission, and blank amazon-mq instance type.
tags: [aws, acceptance, opensearch, cache, mq, secrets]
status: stable
generated:
  by: cursor/devbox
  at: 2026-09-15
---

# AWS live acceptance traps

Proven across the September 2026 AWS live runs (mlaw1). Each of these failed
at least one full-cycle run before the fix. Merged into one note per the
two-new-files cap; split if any section grows its own follow-up.

## Never `magelift bootstrap` before the harness

The harness is self-sufficient: it creates its own state bucket
(`ensure_state_bucket`), KMS/keychain secrets, and ownership marker. Running
`magelift bootstrap` first breaks the pre-create gate twice: its state bucket
lacks the run ownership marker, and its three GitHub-OIDC roles
(`magelift-<project>-preview-{build,ci,deploy}`) trip `assert_clean` with no
allowlist. Fix is deleting the three roles (detach policies first) and
relaunching the harness alone.

## Cache secrets must be hex, never base64

ElastiCache `auth_token` rejects `/`, which base64 regularly contains. Five
consecutive base64 generations were luckily `/`-free before a run failed
preview on it. Generate with `openssl rand -hex 24` (same ARN, no YAML
change).

## VPC OpenSearch: omit VpcId, create the legacy SLR

The provider rejects an explicit `vpc_options.vpc_id` ("Value for
unconfigurable attribute") — it is derived from the subnets, so drop it
(fixed 095e0d7). Domain CREATE then still fails ValidationException ("must
enable a service-linked role ... to access your VPC") even when
`AWSServiceRoleForAmazonOpenSearchService` exists: the VPC path checks the
legacy `es.amazonaws.com` role, so create
`AWSServiceRoleForAmazonElasticsearchService` once per account.

## Native Magento search hostname needs `https://`

`SearchClient::buildOSConfig` takes the scheme from the hostname and defaults
to http, so an unprefixed AWS hostname becomes `http://host:443` and the
HTTPS-only domain answers 400 on the first full deploy after provisioning
(search cells use `--infra-only`, so they never validate). Prefix `https://`
on native hostname bindings when httpsMode==1; ElasticSuite keeps bare
host:port plus flag (fixed 4a63fc7).

## amazon-mq needs `catalog.rabbitMq.instanceType`

`queueMode:amazon-mq` fails admission ("requires engine, version, and
instance type") because the preview preset blanks `rabbitMQInstanceType`.
Engine version comes from the app matrix automatically (2.4.9 -> 4.2); set
`catalog.rabbitMq.instanceType=mq.m7g.large` (schema-valid) explicitly.

## Verify + recreate secrets after EVERY run

The EXIT trap deletes prerequisite secrets on full-cycle failure (gone
without tombstones) but the gate-fail path leaves them. Relaunching without
rechecking fails on missing secrets either way.
