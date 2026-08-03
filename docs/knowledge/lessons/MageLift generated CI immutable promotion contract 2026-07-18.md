---
type: lesson
title: MageLift generated CI immutable promotion contract 2026-07-18
description: Generated workflows validate the environment matrix, build and keylessly sign one OCI digest,
  expose that digest as a job output, deploy staging on main, and require a GitHub production environment...
tags:
- github-actions
- ci
- oidc
- release
- deployment
generated:
  at: '2026-07-24'
---

Generated workflows validate the environment matrix, build and keylessly sign one OCI digest, expose that digest as a job output, deploy staging on main, and require a GitHub production environment approval before promote and deploy. Pull-request previews are opt-in through the magelift-preview label and a matching closed pull request runs destroy for cleanup. Required repository variables include AWS region and OIDC role ARNs, image references, Pulumi backend URL, preview/staging environment names, and the release certificate identity.
