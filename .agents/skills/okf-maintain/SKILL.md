---
name: okf-maintain
description: >-
  Maintain the MageLift OKF bundle at .agents/knowledge/. Default is UPDATE or
  SKIP. Create a note only for a durable fact that the code, the human docs,
  and the bundle do not already state. Use when writing or editing
  .agents/knowledge/, after a verified change that supersedes a note, or when
  asked to compact, prune, or maintain that knowledge bundle.
version: 1.0.0
---

# OKF maintain

Authoring rules are `.agents/knowledge/conventions.md`. Read that file before
the first write this session. Do not copy it into a note. This bundle does
not use a home-directory vault.

## Completion

At verified completion, read `index.md` and grep the terms this change
affects. UPDATE a note that is now wrong, or SKIP. Do not write a recap, a
session diary, or a checkpoint file. Knowledge work does not add a delivery
gate.

## Keep a note

Keep it only when an agent in a year would need it to avoid a failure or
apply a rule that the code, `AGENTS.md`, and the human docs do not already
state. Drop git history, session recaps, changelogs, in-flight plans, and
clones of docs or code.

## Write

1. Grep the bundle and read its `index.md`. UPDATE the matching note, or
   SKIP. CREATE only when `conventions.md` says the fact belongs here. Two
   new files is the cap. A third means merge. Updates do not count.
2. One concept per file, under `lessons/` or `reference/`. `type` is
   `lesson`, `reference`, `source`, or `adr`. `status` is `draft`,
   `stable`, or `deprecated`. No `notes/` directory.
3. When a note becomes false, write the replacement, link the old file, and
   set the old `status` to `deprecated`.
4. Append `.agents/knowledge/log.md` under `## YYYY-MM-DD`, then:

```sh
.agents/knowledge/tools/okf.py .agents/knowledge
```

The script needs Python 3 and PyYAML. Fix errors. A broken-link warning is
not a reason to delete the link.

## Compact

When asked, or when `okf.py` reports `notes/` or a duplicate title: move a
`notes/` file into `lessons/` or `reference/`, or delete it when it fails
the keep test. Merge duplicate titles and deprecate the loser. Deprecate a
`stale_after` note you cannot re-verify. Delete session diaries. Then run
`okf.py` again.
