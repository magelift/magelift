---
type: lesson
title: MageLift implementation architecture
description: Use a native Go CLI and Pulumi Automation API/components in Go.
tags:
- magelift
- go
- pulumi
- php
- architecture
status: stable
generated:
  at: '2026-07-24'
---

Use a native Go CLI and Pulumi Automation API/components in Go. Ordinary users see one versioned root YAML and a separate Composer-installed Magento-first PHP build package; no TypeScript user requirement. Monorepo contains modular CLI/engine, PHP build package, container images, schemas, docs, examples. Advanced customization uses a versioned Go extension API and resource transforms. Default Pulumi backend is bootstrapped versioned S3 with KMS secrets encryption and built-in DIY locking/history. GitHub Actions/OIDC is the only certified CI provider in v1. Build once and promote the same signed OCI digest across environments. AWS account per environment.

# Related

* Relates to: Projects/magelift/Lessons/MageLift certified AWS service profiles
* Relates to: Projects/magelift/Lessons/MageLift reference and clean-room policy
