---
type: lesson
title: MageLift gendocs must not link Pulumi cloud SDKs
description: cmd/gendocs via cli.New used to import all stack modules (AWS/GCP/OVH/Scaleway Pulumi SDKs),
  producing a ~593MB binary and ~8.5GB peak RSS.
tags:
- ci
- gendocs
- pulumi
- oom
generated:
  by: cursor/wsl
  at: '2026-07-20T12:18:26.034850000'
---

cmd/gendocs via cli.New used to import all stack modules (AWS/GCP/OVH/Scaleway Pulumi SDKs), producing a ~593MB binary and ~8.5GB peak RSS. GitHub-hosted runners OOM/shutdown on that link (exit 143 / hang 12-30+ min). Fix: register StackModules only in cmd/magelift (NewWithModules); keep internal/cli free of cloud stack imports; CLI unit tests use a stubAWSModule. gendocs is ~58MB / under 1GB RSS after. Pre-existed before OVH merge; OVH/Scaleway made it worse.
