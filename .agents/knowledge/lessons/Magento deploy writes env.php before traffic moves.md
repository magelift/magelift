---
type: lesson
title: Magento deploy writes env.php before traffic moves
description: A one-shot deploy task generates env.php, checks the database, and runs setup:upgrade. Serving rolls only if that task exits 0. The crypt key is a one-time random.
tags: [magento, deploy, env-php, secrets, cicd]
status: stable
generated:
  by: cursor/devbox
  at: '2026-09-21'
---

# Magento deploy writes env.php before traffic moves

A working production shop does not boot Magento from an `env.php` baked into the image. After the image exists, a one-shot deploy task runs and the serving rollout waits for it.

Order:

1. Apply infrastructure only when it changed, or when a human forces it.
2. Build and push the image. The tag is the git SHA. Each environment rebuilds its own image. Do not copy a staging image into production.
3. Run the deploy task on the new image. If it fails, stop. Do not update web or cron.
4. Roll web and cron with a circuit breaker (`rollback = true`), `minimumHealthyPercent` 100, and a short deregistration delay. This is a rolling update, not a second target group.
5. Smoke the public health URL. Invalidate the CDN after the new tasks are up.

The deploy task, not every container boot, writes `env.php` from environment variables and secret-manager values, checks that Magento's real database connection opens, imports config, and runs `setup:upgrade --keep-generated`. Web and cron symlink that file from shared storage. A local generate is only the fallback when that file is missing. Maintenance mode is off unless the change is a destructive schema cut. A versioned cache prefix replaces a flush.

The Magento crypt key is a random of length 32 with no symbols, stored in the secret manager, and injected as `CRYPT_KEY`. The secret payload is ignored on later applies so Terraform or Pulumi cannot rotate it. Rotating the key makes existing ciphertext unreadable. An admin password, if the shop creates an admin user, is a separate random. App-specific salts stay in the shop, not in the platform.

GitHub Actions assumes a role with OIDC. The role ARN in the workflow is not a secret. The trust policy is limited to that repository. The role and its policies live in an account-global stack that a human applies. The pipeline must not be able to edit the role it assumes. Production is a protected environment (reviewers, branch limit), and a local production deploy asks for an explicit confirmation. Do not cancel an in-progress deploy when a second push arrives.

Build Magento images on the CPU they will run. Emulating that architecture for DI compile is slow and unreliable.

Creating a container registry is not the same permission as pushing to it. A writer role can push an image and still be denied `repositories.create`. The identity that runs the documented create command needs the admin (or create) role.

Related: [Magento image builds must not see a runtime env.php](Magento%20image%20builds%20must%20not%20see%20a%20runtime%20env.php.md).
