---
type: lesson
title: Fargate Varnish VSM must use -n /tmp not /dev/shm
description: 'From magento-aws-stack / backend-m2-b2b: Varnish VSM default can land on Fargate /dev/shm
  (64 MiB, sharedMemorySize unsupported).'
tags:
- varnish
- fargate
- e2e
- reference
status: stable
generated:
  at: '2026-07-24'
---

From magento-aws-stack / backend-m2-b2b: Varnish VSM default can land on Fargate /dev/shm (64 MiB, sharedMemorySize unsupported). Their entrypoint uses varnishd -n /tmp/varnish. MageLift official varnish image entrypoint appends hyphen flags — set Command [-n,/tmp/varnish,-p,vsl_space=4m], mount shared tmp volume at /tmp, VARNISH_SIZE lowercase m. Refs also split Varnish into its own ECS service with much larger memory; colocated preview tasks need lean malloc. Wire awslogs before debugging start failures.
