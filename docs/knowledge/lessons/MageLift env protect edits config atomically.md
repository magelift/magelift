---
type: lesson
title: MageLift env protect edits config atomically
description: The env protect command is a local configuration operation.
tags:
- cli
- config
- safety
- atomic-write
generated:
  at: '2026-07-24'
---

The env protect command is a local configuration operation. It validates a stable environment name and existing effective configuration, requires exactly one of --on or --off, requires --yes before disabling production protection, updates only the selected YAML overlay, preserves file permissions, and replaces the file atomically. Environment creation and destruction remain separate until their infrastructure semantics are implemented.
