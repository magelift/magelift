---
type: knowledge
title: AWS ECS tmpfs supports executable Varnish cache mount 2026-07-18
description: AWS ECS LinuxParameters.tmpfs accepts containerPath, mountOptions, and size in MiB.
tags:
- aws
- ecs
- fargate
- security
- source
generated:
  at: '2026-07-24'
sources:
- id: https-docs-aws-amazon-com-amazonecs-latest-apireference-api-linuxparameters-html
  resource: https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_LinuxParameters.html
---

AWS ECS LinuxParameters.tmpfs accepts containerPath, mountOptions, and size in MiB. The documented mount options include rw, exec, mode, uid, and gid. MageLift uses this for the integrated Varnish sidecar while retaining readonlyRootFilesystem.
