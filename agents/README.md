# Agents and skills

First-party Agent Skills for MageLift. Use these with Cursor, Claude Code, Codex,
and other tools that load `SKILL.md` packages. They encode project rules that
generic coding agents miss (serial builds, certified-cell honesty, provider
boundaries).

## Layout

```
agents/
  README.md                 # this file
  skills/
    magelift-contribute/    # PRs, verify gate, layout
    magelift-serial-builds/ # local build/test parallelism policy
    magelift-provider/      # adding a cloud adapter
    magelift-release/       # tags, GoReleaser, Cosign, GHCR
    magelift-site/          # marketing + docs site build/deploy
```

Each skill is a directory with a `SKILL.md` (Agent Skills format).

## Wire them into your tool

### Cursor

Symlink or copy into the project skills path (gitignored locally):

```sh
mkdir -p .cursor/skills
for d in agents/skills/*/; do
  ln -sfn "$(cd "$d" && pwd)" ".cursor/skills/$(basename "$d")"
done
```

Or install via the skills CLI if you prefer that workflow.

### Claude Code

```sh
mkdir -p .claude/skills
for d in agents/skills/*/; do
  ln -sfn "$(cd "$d" && pwd)" ".claude/skills/$(basename "$d")"
done
```

Do not commit `.claude/` or `.cursor/`; they stay local. The source of truth is
`agents/skills/`.

### Codex / other AGENTS.md consumers

Point the session at this file and the skill bodies under `agents/skills/`.
There is no root `AGENTS.md` in this repository on purpose.

### Manual

Open the relevant `SKILL.md` and follow it. Skills are short enough to paste
into a chat when the tool has no skill loader.

## What we do not vendor

Third-party skill packs (generic Golang, marketing templates, etc.) stay off the
Apache-2.0 audited tree. Install those in your user skill directory if you want
them. Only MageLift-specific rules live here.

## Related human docs

- [CONTRIBUTING.md](../CONTRIBUTING.md)
- [docs/adding-a-provider.md](../docs/adding-a-provider.md)
- [docs/release-readiness.md](../docs/release-readiness.md)
- [docs/publishing.md](../docs/publishing.md)
- [websites/marketing/README.md](../websites/marketing/README.md)
