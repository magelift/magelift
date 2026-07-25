---
type: lesson
title: MageLift OSS multi-provider readiness 2026-07-19
description: CLI deploy path uses platform.ModuleRegistry + optional HasOps (AWS Magento steps in internal/cloud/aws/ops
  to avoid stack/deployment cycle; GCP Ops returns ErrNotSupported).
tags:
- oss
- multi-cloud
- ports
- golangci
generated:
  by: cursor/wsl
  at: '1784495802931202'
---

CLI deploy path uses platform.ModuleRegistry + optional HasOps (AWS Magento steps in internal/cloud/aws/ops to avoid stack/deployment cycle; GCP Ops returns ErrNotSupported). Factories take PlannedStack not awsstack.Spec. secretref accepts gcp-secret-manager:// with provider-aware config validation. OSS: issue/PR templates, golangci (govet/staticcheck/ineffassign/misspell/unused), mkdocs nav+CI docs job, CONTRIBUTING, adding-a-provider.md, examples/custom-cli. ModuleRegistry is production seam; infra.Registry is SDK discovery only.
