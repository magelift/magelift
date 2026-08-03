---
type: lesson
title: MageLift resolver bounded defaults 2026-07-18
description: The resolver now fills default preset topology values and Adobe compatibility service versions
  only when target.aws exists in the project or selected environment.
tags:
- magelift
- config
- precedence
- compatibility
status: stable
generated:
  at: '2026-07-24'
sources:
- id: roadmap-audit-2026-07-18
  resource: roadmap audit 2026-07-18
---

The resolver now fills default preset topology values and Adobe compatibility service versions only when target.aws exists in the project or selected environment. It preserves explicit built-ins, compatibility overlays, presets, and CLI overrides. It never invents credentials, certificates, external references, or benchmark-selected instance sizes.
