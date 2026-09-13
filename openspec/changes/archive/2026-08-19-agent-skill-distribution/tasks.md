## 1. Skill content

- [x] 1.1 Audit the tracked skills for maintainer-only assumptions and stale certification claims.
- [x] 1.2 Rewrite each skill with user and contributor activation rules.
- [x] 1.3 Add frontmatter, version metadata, and content tests.

## 2. Manifest and installer

- [x] 2.1 Add manifest generation with file digests and release metadata.
- [x] 2.2 Add `magelift skills list`.
- [x] 2.3 Add `magelift skills install` with project/global scope, agent selection, copy mode, and conflict handling.
- [x] 2.4 Add `magelift skills verify` with path and digest checks.
- [x] 2.5 Add the optional `npx skills` backend without making Node a hard dependency.

## 3. Packaging and verification

- [x] 3.1 Include the skill bundle and manifest in release archives.
- [x] 3.2 Add tests for Codex, Claude Code, Cursor, and generic `.agents/skills` paths.
- [x] 3.3 Verify global and local installation in temporary directories.
- [x] 3.4 Update README and install documentation.
