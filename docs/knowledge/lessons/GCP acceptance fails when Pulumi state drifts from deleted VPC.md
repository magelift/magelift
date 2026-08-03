---
type: lesson
title: GCP acceptance fails when Pulumi state drifts from deleted VPC
description: After manual orphan cleanup of mlacc-* GCP resources, Pulumi stack may still report VPC/secrets
  as unchanged.
tags:
- gcp
- pulumi
- acceptance
generated:
  by: cursor/wsl
  at: '1784489733726763'
---

After manual orphan cleanup of mlacc-* GCP resources, Pulumi stack may still report VPC/secrets as unchanged. Next deploy then fails with SCP 404 network, SecretVersion 404, Cloud SQL missing PSA. Before acceptance up: refresh+destroy or remove stack when assert_clean is green but stack has resources. Destroy can also fail on PSA while soft-deleted producers linger — retry peering delete.
