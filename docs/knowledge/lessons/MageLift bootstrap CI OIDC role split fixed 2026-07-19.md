---
type: lesson
title: MageLift bootstrap CI OIDC role split fixed 2026-07-19
description: Bootstrap now creates separate StateRole (recovery), CIRole (GitHub Actions Pulumi/ECS/managed
  resources), and BuildRole (read MageLift-scoped secrets).
tags:
- bootstrap
- oidc
- iam
- ci
status: stable
generated:
  at: '2026-07-24'
---

Bootstrap now creates separate StateRole (recovery), CIRole (GitHub Actions Pulumi/ECS/managed resources), and BuildRole (read MageLift-scoped secrets). Docs map identity.ciRoleArn and identity.buildRoleArn. Older notes claiming a single state-only OIDC role cannot run generated CI are outdated for current code; OIDC provider Create/Get still needs real AWS or an emulator that implements IAM OIDC (Floci 1.5.33 does not; Ministack documents CreateOpenIDConnectProvider).
