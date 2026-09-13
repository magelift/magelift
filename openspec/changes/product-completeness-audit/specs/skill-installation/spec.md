## ADDED Requirements

### Requirement: User skills and contributor skills are separate trees

User skills MUST live under `agents/skills/` and are the only skills embedded in release binaries and listed by `magelift skills`. Contributor skills MUST live under `contrib/skills/` (and any personal toolchain such as humanizer, watermarks, Pulumi, or Golang skills). `magelift skills install` MUST refuse to install contributor skills. Contributor-only skills MUST NOT be copied into the user bundle.

#### Scenario: Release binary skill list

- **WHEN** a user runs `magelift skills list` from a release build
- **THEN** only first-party user skills are listed, and contributor paths are absent

#### Scenario: Install rejects a contributor skill name

- **WHEN** a user passes a contributor skill name to `magelift skills install`
- **THEN** the command fails before writing any agent directory and names the user-versus-contributor split
