---
type: lesson
title: Floci 1.7.0 implements IAM OpenID Connect Provider APIs
description: Floci AWS 1.7.0 supports Create/Get/Tag OpenID Connect Provider. That closes the 1.5.33 UnsupportedOperation gap. It is not GitHub Actions token-exchange certification.
tags:
- floci
- iam
- oidc
- testing
status: stable
stale_after: 2027-02-18
generated:
  by: cursor-grok-4.6/darwin
  at: '2026-08-18'
---

Retest on digest-pinned `floci/floci:1.7.0` succeeded: `ListOpenIDConnectProviders`, `CreateOpenIDConnectProvider`, and `GetOpenIDConnectProvider` work. `tests/floci/oidc_test.go` (`TestIAMOpenIDConnectProviderAgainstFloci`) keeps the contract in CI. Do not weaken production `NoSuchEntity` handling. GitHub Actions OIDC token exchange and live account `Ensure` remain paid-only. The 2026-07-18 lesson about 1.5.33 `UnsupportedOperation` is historical.
