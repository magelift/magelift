---
type: lesson
title: MageLift build identity test expectation failure 2026-07-18
description: After adding a dedicated build role, idempotency updates trust policies only for existing
  roles.
tags:
- bootstrap
- testing
- iam
generated:
  at: '2026-07-24'
---

After adding a dedicated build role, idempotency updates trust policies only for existing roles. Three roles created on first bootstrap, so the second run performs three trust updates, not four. Keep fake IAM call-count assertions aligned with create-versus-update behavior.
