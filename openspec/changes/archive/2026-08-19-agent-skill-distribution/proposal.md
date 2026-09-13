## Why

MageLift's tracked skills are useful to contributors, but users have to copy or symlink them by hand. The repository README also describes a source layout that is easy to miss when MageLift is installed as a release binary.

## What Changes

- Rework the five first-party MageLift skills for end users and contributors, with clear activation rules and fewer maintainer-only assumptions.
- Publish a skill manifest and version metadata with the MageLift release.
- Add `magelift skills list`, `magelift skills install`, and `magelift skills verify`.
- Support project-local and user-global installation, with explicit agent selection.
- Integrate with the `skills` CLI when it is available, while keeping a direct copy or symlink path for offline and no-Node environments.
- Verify skill names, source digests, and destination paths before writing.
- Keep `.claude/`, `.cursor/`, `.agents/`, and other generated agent directories out of the MageLift source tree.

## Capabilities

### New Capabilities

- `skill-installation`: Discover, verify, and install MageLift's first-party skills.

### Modified Capabilities

None. The repository has no existing OpenSpec capability specs.

## Impact

The change affects `agents/skills`, the release archive, CLI command registration, platform-specific agent path mapping, documentation, and release verification. It adds no runtime dependency to deployed MageLift stacks.
