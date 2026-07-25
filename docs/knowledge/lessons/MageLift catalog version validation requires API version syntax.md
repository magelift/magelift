---
type: lesson
title: MageLift catalog version validation requires API version syntax
description: The AWS stack Spec validates managed service versions with API-shaped major.minor strings.
tags:
- lesson
- versions
- aws
- validation
generated:
  at: '2026-07-24'
---

The AWS stack Spec validates managed service versions with API-shaped major.minor strings. Adobe tables may display Valkey 8, but the internal version validator requires a concrete value such as 8.1; planner fixtures use Aurora 8.0.mysql_aurora.3.12, OpenSearch_3.1, Valkey 8.1, and AWS MQ RabbitMQ 3.13. Do not copy display-only major versions into tests without checking the provider API format.
