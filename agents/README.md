# Agents and skills

First-party user Agent Skills for MageLift. Use these with Cursor, Claude Code,
Codex, and other tools that load `SKILL.md` packages. They explain how to
configure, migrate, operate, and run MageLift locally.

## Layout

```
agents/
  README.md                 # this file
  skills/
    magelift-dependencies/  # prerequisite checks and explicit installation
    magelift-local-runtime/ # local Docker Compose workflow
    magelift-operate/       # day-to-day status, health, and safe changes
    magelift-configure/     # configuration, validation, and provenance
    magelift-migrate/       # ACC and Upsun migration workflows
```

Contributor workflows live under [`contrib/skills/`](../contrib/skills/). They
are not embedded in release binaries and cannot be installed by end users.

Each skill is a directory with a `SKILL.md` (Agent Skills format).

## Install them with MageLift

The release binary includes the skills and installs them without network access:

```sh
magelift skills list
magelift skills install --agent codex --scope project --mode symlink
magelift skills install --agent claude-code --scope global --mode copy
magelift skills verify --agent codex --scope project
```

Use `--skill magelift-operate` to install only one skill. The direct installer
refuses to replace an unrelated destination unless `--replace` is explicit.
Use `--backend skills-cli` from a source checkout when you want the official
skills CLI to manage the installation:

```sh
magelift skills install --backend skills-cli --agent codex --scope project
```

The skills CLI is optional. The direct installer is the release-safe default.

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

Root [`AGENTS.md`](../AGENTS.md) is the always-on router (hard constraints plus
the skill table). Load the matching `SKILL.md` for the task; do not paste skill
bodies into the session.

### Manual

Open the relevant `SKILL.md` and follow it. Skills are short enough to paste
into a chat when the tool has no skill loader.

## What we do not vendor

Third-party skill packs (generic Golang, marketing templates, etc.) stay off the
Apache-2.0 audited tree. Install those in your user skill directory if you want
them. Only MageLift-specific rules live here.

## Related human docs

- [CONTRIBUTING.md](../CONTRIBUTING.md)
- [Contributor skills](../contrib/skills/)
- [docs/adding-a-provider.md](../docs/adding-a-provider.md)
- [docs/release-readiness.md](../docs/release-readiness.md)
- [docs/publishing.md](../docs/publishing.md)
- [website/README.md](../website/README.md)
