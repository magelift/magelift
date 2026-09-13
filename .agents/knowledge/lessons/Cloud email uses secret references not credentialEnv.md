---
type: lesson
title: Cloud email uses secret references not credentialEnv
description: Cloud `email.credential` is a provider-matching secret reference; `local.email.credentialEnv` is workstation-only. Resolve success is not certified SendGrid delivery.
tags:
- email
- sendgrid
- secrets
- openspec
status: stable
generated:
  by: cursor-grok/desktop
  at: '2026-08-18'
---

Cloud Magento SMTP is top-level `email` on the project or an environment overlay. `sendgrid` and `ses` require `email.credential` as `aws-secrets-manager://` / `ssm://` on AWS or `gcp-secret-manager://` on GCP. Plaintext is rejected before Magento SMTP is written.

`local.email` stays the workstation overlay and uses `credentialEnv`. It is not a cloud delivery adapter.

A preview environment that omits `email` resolves to `disabled` and does not inherit production SendGrid or SES credentials. Configuration validation succeeding is not certified delivery; that stays the session-1 GCP origin cell or typed unsupported.

# Related

[GCP Magento destroy must pass --destroy-backups](/lessons/GCP%20Magento%20destroy%20must%20pass%20--destroy-backups.md)
