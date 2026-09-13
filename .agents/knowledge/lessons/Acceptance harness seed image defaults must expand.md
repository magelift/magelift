---
type: lesson
title: Acceptance harness seed image defaults must expand
description: Escaped ${VAR:-default} in aws-acceptance-local.sh made kubectl run receive a literal invalid image name.
tags: [aws, eks, acceptance, kubectl, bash]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-14
sources:
  - id: kubectl-run
    resource: https://kubernetes.io/docs/reference/kubectl/generated/kubectl_run/
    title: kubectl run
---

# Acceptance harness seed image defaults must expand

`kubectl run --image` needs a real OCI reference. A Bash default written as
`"\${MAGELIFT_AWS_ACCEPTANCE_SEED_IMAGE:-public.ecr.aws/docker/library/mysql:8.4}"`
does not expand. kubectl then reports `Invalid image name` / invalid
reference format.

Magento EKS Auto Mode `20260813aw` got past qualified Pulumi kubeconfig, then
failed seed import on that literal string. The ECS seed path already used the
unescaped form. Escape only inside single-quoted heredocs; live harness
assignments must expand.
