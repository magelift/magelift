---
status: done
slug: ai-skills-tracks
---

# Intent: two skill tracks, one for users and one for contributors

## Problem

The project promises AI readiness but the two audiences need different things: shop teams want skills installed into their Magento repos so agents use Magelift correctly; contributors need repo workflows (certify, provider, release, site) that must never ship in the user binary. Mixing the tracks would leak maintainer runbooks into shops or starve contributors of procedure.

## Evidence

`agents/skills/` holds five user skills embedded in release binaries (`magelift-configure`, `magelift-dependencies`, `magelift-local-runtime`, `magelift-migrate`, `magelift-operate`) with `manifest.json`, installed via `magelift skills install --agent --scope --mode` and checked by `skills verify`. `contrib/skills/` holds seven contributor skills never embedded (`magelift-certify`, `magelift-contribute`, `magelift-extend`, `magelift-provider`, `magelift-release`, `magelift-serial-builds`, `magelift-site`). `AGENTS.md` routes by task with a skill table. Whether agents succeed on a fresh shop repo using only installed user skills: not checked.

## Proposed outcome

Shop teams install user skills into their Magento projects from the release binary with no network access, and agents then configure, migrate, run local, and operate through documented Magelift commands only. Contributors symlink contributor skills locally and follow one procedure per task (add a provider, certify a cell, cut a release). Each new v1 intent updates the skill that teaches it, so skills never drift behind the CLI they describe.

## Affected users and systems

Shop developers using Cursor, Claude Code, Codex, and other `SKILL.md` consumers; project contributors. `agents/`, `contrib/skills/`, `magelift skills` commands, `agents/manifest.json` generation (`make generate`), root `AGENTS.md` router.

## Constraints

User skills ship in the binary; contributor skills never do. Do not commit `.cursor/`, `.claude/`, caches, credentials, or third-party skill packs; do commit `.agents/knowledge/` updates. Human docs and website copy go through humanizer then remove-ai-marks. New or changed CLI behavior in any v1 intent updates the matching skill in the same change.

## Out of scope

New skill hosts beyond the documented agents, auto-updating installed skills, skills for experimental EU providers beyond honest scope notes.

## Open questions

What is the acceptance test for a user skill: a scripted agent run against a fixture shop, a checklist, or both? Who owns the per-intent skill update when an intent touches several skills?
