---
type: reference
title: Knowledge bundle authoring conventions
description: How to read and write this bundle. Update or skip; create a note only for a durable fact the code and docs do not already state.
tags: [okf, knowledge]
status: stable
---

# Knowledge conventions

This directory is an [OKF v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md) bundle: markdown files with YAML frontmatter. Read and write them directly. A clone does not need a home-directory vault.

Default action is **UPDATE or SKIP**. Create a file only when all of these hold: the fact will still be true after the task, nothing in this bundle or in `AGENTS.md`, the human docs, or the code already says it, and it is one concept. Two new files is the cap for one change. A third means merge. Session diaries, plans, and todos do not belong here.

`docs/` is for humans. Do not copy it into a note.

## Recall

Read `index.md`, grep, then open only the matching notes. Current source supersedes a note as soon as they disagree. Do not load the whole bundle.

## Note

One concept per file. The filename is the title. Use `lessons/` for a durable rule and `reference/` for how something works.

```yaml
---
type: lesson            # lesson | reference | source | adr
title: EXIT traps cannot be the cleanup authority
description: One sentence. This is the index line.
tags: [cleanup]
status: stable          # draft | stable | deprecated
generated:
  by: cursor
  at: 2026-10-19
---
```

`type` is required. `status` is only `draft`, `stable`, or `deprecated`. Add `stale_after: YYYY-MM-DD` when the fact is pinned to a version or a price. `verified: [{by: human:<name>, at: YYYY-MM-DD}]` means a person checked it.

When a note becomes false, write the replacement, link the old file, and set the old `status` to `deprecated`.

Link from the bundle root, for example [EXIT traps](/lessons/EXIT%20traps%20cannot%20be%20the%20cleanup%20authority.md). A missing target can stay. Do not use `[[wikilinks]]`.

## After a write

Append `log.md` under `## YYYY-MM-DD`, then:

```sh
.agents/knowledge/tools/okf.py .agents/knowledge
```

The script rewrites each `index.md` from frontmatter and checks that every note has a `type`. It needs Python 3 and PyYAML. Fix errors. A broken-link warning is not a reason to delete the link.
