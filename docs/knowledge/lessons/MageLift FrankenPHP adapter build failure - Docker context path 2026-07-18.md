---
type: lesson
title: 'MageLift FrankenPHP adapter build failure: Docker context path 2026-07-18'
description: The first FrankenPHP classic Docker build used COPY Caddyfile while docker-bake.hcl set the
  repository root as context, so BuildKit could not find /Caddyfile.
tags:
- docker
- frankenphp
- failed-attempt
generated:
  at: '2026-07-24'
---

The first FrankenPHP classic Docker build used COPY Caddyfile while docker-bake.hcl set the repository root as context, so BuildKit could not find /Caddyfile. Use COPY images/frankenphp-classic/Caddyfile for this root-context target. The corrected image built successfully and reported FrankenPHP 1.12.4 with PHP 8.5.8.
