---
type: lesson
title: Effective config keeps preset under defaults
description: The typed effective configuration stores the selected preset in Config.Defaults.Preset.
tags:
- config
- cli
- presets
status: stable
generated:
  at: '2026-07-24'
---

The typed effective configuration stores the selected preset in Config.Defaults.Preset. Config.Preset is the environment overlay field and remains empty when only project defaults provide the preset. CLI status and platform code must read Defaults.Preset after resolution.
