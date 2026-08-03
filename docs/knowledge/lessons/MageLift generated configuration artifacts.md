---
type: lesson
title: MageLift generated configuration artifacts
description: MageLift derives schema/magelift.schema.json and docs/configuration.md from YAML model metadata
  in internal/config.
tags:
- configuration
- json-schema
- documentation
- generation
status: stable
generated:
  at: '2026-07-24'
---

MageLift derives schema/magelift.schema.json and docs/configuration.md from YAML model metadata in internal/config. Run go generate ./internal/config to refresh and go run ./cmd/genconfig --check to detect drift. The generator accepts an explicit repository root because go generate executes from the package directory. It handles strict objects, maps, slices, pointers/nullability, required fields, constants, enums, patterns, and minimum constraints without a runtime dependency.
