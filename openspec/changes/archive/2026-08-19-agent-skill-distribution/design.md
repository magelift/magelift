## Context

See `proposal.md` for the motivation. The tracked source of truth is `agents/skills/` with five MageLift-specific skills. The repository ignores `.agents/`, `.claude/`, and `.cursor/`, which is appropriate for generated installations. The external `skills` CLI supports project or global installation, agent selection, symlink or copy mode, and local repository paths. See the [Skills CLI reference](https://www.skills.sh/docs/cli) and the [Skills CLI repository](https://github.com/vercel-labs/skills).

## Goals / Non-Goals

**Goals:**

- Make the tracked skills useful to both users and contributors.
- Offer one MageLift command with explicit scope and agent selection.
- Keep direct installation deterministic and usable without Node.js.
- Treat skill text as supply-chain-sensitive content and verify it before writing.

**Non-Goals:**

- Replacing the `skills` ecosystem or its registry.
- Installing arbitrary third-party skills through MageLift.
- Executing scripts from skill packages.
- Committing generated agent directories.

## Decisions

### 1. Direct installer is the default

The MageLift CLI will install the bundled first-party files directly when it can resolve the source locally or from a MageLift release archive. The `skills` CLI is an optional backend, selected explicitly. This keeps the Go binary useful on hosts without Node.js and avoids making an unrelated npm package part of the release runtime.

### 2. Use a small agent path registry

The installer will map supported agent IDs to project and global directories. Unknown agent IDs fail with a list of supported choices. The registry will live in one place and have table-driven tests for path safety.

### 3. Verify before write

The manifest will include SHA-256 digests for all skill files. Installation stages content in memory or a temporary directory, validates frontmatter and digests, then writes or links the complete skill directory. A conflict requires an explicit replace flag.

### 4. Keep skills user-facing

The five skills will describe when a user should invoke them, what MageLift contract they protect, and what evidence a contributor must provide. Maintainer-only paths, stale status claims, and instructions that assume a private checkout will move into contributor documentation or disappear.

## Risks / Trade-offs

- [A release archive lacks the skill source] -> Include the manifest and skill files in release artifacts, and fail with a direct download instruction rather than guessing a repository path.
- [Agent path conventions change] -> Version the path registry and keep installation tests per agent.
- [A skill contains unsafe instructions] -> Verify source identity and digest, never execute skill content, and keep third-party installation outside this command.
- [Symlinks are unsupported] -> Fall back to copy mode only when the user selected automatic fallback or `--copy`.

## Migration Plan

1. Rewrite and test the five tracked skills.
2. Add manifest generation and release packaging.
3. Implement list, install, and verify commands with direct copy mode.
4. Add explicit `skills` CLI delegation and document both paths.
5. Run installation tests in a temporary project and global directory without touching the user's existing skills.
