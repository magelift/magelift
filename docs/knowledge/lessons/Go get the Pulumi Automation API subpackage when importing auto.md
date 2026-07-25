---
type: lesson
title: Go get the Pulumi Automation API subpackage when importing auto
description: The first internal/automation build failed after adding github.com/pulumi/pulumi/sdk/v3 because
  the module-level go get did not populate every go.sum entry required by go/auto.
tags:
- pulumi
- go-modules
- automation-api
- test-failure
status: stable
generated:
  at: '2026-07-24'
---

The first internal/automation build failed after adding github.com/pulumi/pulumi/sdk/v3 because the module-level go get did not populate every go.sum entry required by go/auto. When importing Automation API, run go get github.com/pulumi/pulumi/sdk/v3/go/auto@the-pinned-version so all transitive workspace and Git dependencies are recorded.
