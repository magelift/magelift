---
type: lesson
title: Local Cosign keyless uses GitHub OAuth identity
description: Local cosign sign --yes against ECR uses Sigstore device flow.
tags:
- cosign
- sigstore
- acceptance
- ghcr
- ecr
generated:
  at: '2026-07-24'
---

Local cosign sign --yes against ECR uses Sigstore device flow. Verified identity Subject is the operator GitHub OAuth email (Issuer=https://github.com/login/oauth, not token.actions.githubusercontent.com). For magelift promote/acceptance set MAGELIFT_AWS_CERTIFICATE_IDENTITY and MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER accordingly. Digest and tlog index retained in offline acceptance notes (account IDs not published).

# Related

* Relates to: Projects/magelift/Findings/AWS acceptance status 2026-07-19
