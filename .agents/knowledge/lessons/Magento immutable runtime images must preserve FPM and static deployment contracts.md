---
type: lesson
title: Magento immutable runtime images must preserve FPM and static deployment contracts
description: A Magento image can pass Kubernetes readiness while still failing application traffic if PHP-FPM strips injected endpoint variables or generated static deployment metadata is absent.
tags: [magento, php, fpm, containers, kubernetes, runtime, acceptance]
status: stable
generated:
  by: codex
  at: 2026-08-17
---

# Magento immutable runtime images must preserve FPM and static deployment contracts

Kubernetes readiness only proves that the container process and configured
readiness endpoint are available. For a Magento image using environment-backed
`app/etc/env.php` resolution, PHP-FPM must retain the endpoint environment in
the worker process (`clear_env = no`). Otherwise the resolver can run in the
container but Magento receives an empty database/cache endpoint and falls back
to local defaults, commonly surfacing as a localhost connection refusal.

The same immutable image must contain the generated static deployment marker,
`pub/static/deployed_version.txt`, plus its static content. Without that
marker, Magento may connect to the database successfully and still return a
500 while rendering the storefront with an “unable to retrieve deployment
version” exception.

Acceptance should inspect the actual running image and exercise an application
request after seed import. A healthy Deployment, a successful database-grant
Job, and a successful CLI command are necessary but do not substitute for the
HTTP render check. Record the exact image digest and keep endpoint/base-URL
mutations explicit; do not silently treat a seeded localhost URL or a manual
configuration mutation as platform-provided public URL injection.

The rejected shortcut is to certify the original image based on pod readiness
or to repair the running pod with an ad hoc file copy. The durable fix belongs
in the reproducible image build/runtime contract, while a bounded acceptance
mutation may be used only to make the seeded database's public URL explicit
for a direct edge probe. See the [EBS identity lesson](EKS%20EBS%20CSI%20add-ons%20need%20a%20pod%20identity%20role.md).
