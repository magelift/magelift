---
type: lesson
title: MageLift runtime env injection contract 2026-07-18
description: Runtime tasks keep ReadonlyRootFilesystem enabled.
tags:
- runtime
- secrets
- magento
- aws
generated:
  at: '2026-07-24'
status: deprecated
---

Runtime tasks keep ReadonlyRootFilesystem enabled. The certified image contains a non-secret env.php scaffold with MAGE_MODE and document_root_is_pub. The stack requires target.aws.encryptionKeySecretArn, injects MAGENTO_DC_CRYPT__KEY, and injects RDS managed-secret JSON fields through ECS Secrets Manager selectors such as :password::. Capability endpoints and non-secret Magento DC values are exposed as MAGENTO_DC_* environment variables. The managed database secret remains available as MAGELIFT_DATABASE_CREDENTIALS for compatibility. Build tests reject source env.php and verify the image scaffold contains no secret material.

# Related

* Supersedes: Projects/magelift/Lessons/MageLift runtime env.php injection remains an implementation gap 2026-07-18
