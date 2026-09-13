## Purpose

Lets MageLift users install and verify the first-party skills for the agents they use without copying repository directories by hand.

## Requirements

### Requirement: Skill manifest

MageLift MUST expose a manifest containing each first-party skill name, description, release version, file digest, and supported installation source.

#### Scenario: User lists bundled skills

- **WHEN** the user runs `magelift skills list`
- **THEN** MageLift prints the skill names, descriptions, version, and verification state

### Requirement: Project and global scopes

The installer MUST support a project-local scope and a user-global scope. The two scopes MUST use the selected agent's documented directory and MUST never write outside that directory or the MageLift source tree.

#### Scenario: User installs project skills for Codex

- **WHEN** the user selects project scope and the Codex agent
- **THEN** the installer writes only the project Codex skill directory and reports the paths

#### Scenario: User installs global skills for Claude Code

- **WHEN** the user selects global scope and Claude Code
- **THEN** the installer writes only the user's Claude skill directory

### Requirement: Verified installation

The installer MUST verify skill names, frontmatter, file digests, and destination path safety before writing. It MUST refuse a path traversal or digest mismatch.

#### Scenario: Skill content is modified

- **WHEN** a source skill does not match the manifest digest
- **THEN** installation fails before changing the destination

### Requirement: Optional skills CLI integration

MageLift MAY delegate installation to the official `skills` CLI when explicitly requested or when the user chooses that backend. Direct installation MUST remain available without Node.js or network access when the bundled skills are present.

#### Scenario: `skills` CLI is unavailable

- **WHEN** the user requests installation from a local MageLift checkout and `npx skills` is absent
- **THEN** MageLift uses its direct copy or symlink path

### Requirement: Safe link and copy behavior

The installer MUST support symlink or copy mode, report which mode was used, and refuse to overwrite an unrelated existing skill without an explicit replace flag.

#### Scenario: Existing skill belongs to another source

- **WHEN** a destination contains a skill with a different manifest identity
- **THEN** MageLift stops and identifies the conflict

### Requirement: Verification without execution

`magelift skills verify` MUST inspect installed files and metadata without executing skill scripts or following arbitrary commands from a skill body.

#### Scenario: Installed skill has an unexpected file

- **WHEN** verification finds a file outside the manifest
- **THEN** it reports the drift and exits non-zero

### Requirement: User skills and contributor skills are separate trees

User skills MUST live under `agents/skills/` and are the only skills embedded in release binaries and listed by `magelift skills`. Contributor skills MUST live under `contrib/skills/` (and any personal toolchain such as humanizer, watermarks, Pulumi, or Golang skills). `magelift skills install` MUST refuse to install contributor skills. Contributor-only skills MUST NOT be copied into the user bundle.

#### Scenario: Release binary skill list

- **WHEN** a user runs `magelift skills list` from a release build
- **THEN** only first-party user skills are listed, and contributor paths are absent

#### Scenario: Install rejects a contributor skill name

- **WHEN** a user passes a contributor skill name to `magelift skills install`
- **THEN** the command fails before writing any agent directory and names the user-versus-contributor split

### Requirement: Third-party agent packs stay out of the user bundle

User skills MUST remain under `agents/skills/` and ship in the CLI. Contributor Magelift skills MUST remain under `contrib/skills/` for this change. `.agents/skills/` MUST stay gitignored for third-party packs. `magelift skills` MUST NOT install contributor or third-party packs.

#### Scenario: User skill list ignores .agents

- **WHEN** a user runs `magelift skills list`
- **THEN** only `agents/skills/` names appear
