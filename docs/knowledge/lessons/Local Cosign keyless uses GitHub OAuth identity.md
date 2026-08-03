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

Local cosign sign --yes against ECR uses Sigstore device flow. Verified identity Subject=alex.courtiol@gmail.com Issuer=https://github.com/login/oauth (not token.actions.githubusercontent.com). For magelift promote/acceptance set MAGELIFT_AWS_CERTIFICATE_IDENTITY and MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER accordingly. Digest signed: 669890779205.dkr.ecr.eu-north-1.amazonaws.com/magelift-acceptance@sha256:162122c42d266638e2e4cae35f9688fde1036136e3015f760409f044c3020fcf tlog index 2200856878.

# Related

* Relates to: Projects/magelift/Findings/AWS acceptance status 2026-07-19
