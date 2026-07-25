---
type: lesson
title: MageLift day-2 Magento ports ADR 0009 2026-07-19
description: CLI day-2 commands (bootstrap/login/state/secrets/logs/exec/health runtime) resolve via platform.ModuleBootstrap|State|Secrets|RuntimeObserve.
tags:
- architecture
- ports
- cli
generated:
  by: cursor/wsl
  at: '2026-07-19T21:58:25.600023000+00:00'
---

CLI day-2 commands (bootstrap/login/state/secrets/logs/exec/health runtime) resolve via platform.ModuleBootstrap|State|Secrets|RuntimeObserve. AWS implements in internal/cloud/aws/ops; GCP stubs ErrNotSupported. Stack DIY name is project-env-provider-runtime (dots→dashes). FormatStackName + RequireOutputs after infra-only Up when outputs non-empty. Dual registry unchanged: ModuleRegistry is CLI path. Cost/CI still AWS-shaped (YAGNI until second certified).
