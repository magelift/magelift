---
type: lesson
title: Acceptance shell checks must run under Bash
description: MageLift acceptance scripts use Bash semantics and ad hoc checks must avoid zsh reserved variables.
tags:
- acceptance
- bash
- shell
status: stable
generated:
  by: codex/desktop
  at: '2026-08-04'
---

The acceptance scripts have Bash shebangs and use Bash arrays and
`PIPESTATUS`. Ad hoc checks launched by the default zsh shell can fail before
testing anything: `status` is read-only in zsh, and `path` is tied to the
shell's `PATH`. Run these scripts with their shebang or an explicit `bash`, and
use names such as `pipeline_status` and `item_path` for temporary variables.
