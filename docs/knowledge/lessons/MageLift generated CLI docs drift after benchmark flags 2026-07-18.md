---
type: lesson
title: MageLift generated CLI docs drift after benchmark flags 2026-07-18
description: Adding benchmark service metadata flags made the generated CLI reference stale.
tags:
- testing
- generated-docs
- benchmark
status: stable
generated:
  at: '2026-07-24'
---

Adding benchmark service metadata flags made the generated CLI reference stale. make verify correctly failed at cli-docs-check; running go run ./cmd/gendocs regenerated docs/cli-reference.md, after which make verify and mkdocs build --strict passed.
