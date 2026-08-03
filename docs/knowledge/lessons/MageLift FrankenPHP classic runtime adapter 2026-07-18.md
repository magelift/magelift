---
type: lesson
title: MageLift FrankenPHP classic runtime adapter 2026-07-18
description: The selectable frankenphp-classic runtime is represented in the stack and ECS task definition
  alongside nginx-fpm.
tags:
- frankenphp
- debian
- docker
- runtime
generated:
  at: '2026-07-24'
---

The selectable frankenphp-classic runtime is represented in the stack and ECS task definition alongside nginx-fpm. The adapter is pinned to docker.io/dunglas/frankenphp:1.12.4-php8.5-trixie by multi-platform index digest sha256:a0ad00db3dd7f61b55a9951c9b6ef73ac8e1dd4c6de36979a0d1651b48da5f43, runs Debian Trixie as UID 10001, serves /app/pub on port 8080 with a Caddyfile, and does not enable worker mode. The image is a runtime base; the Magento pipeline must add application files and required extensions.
