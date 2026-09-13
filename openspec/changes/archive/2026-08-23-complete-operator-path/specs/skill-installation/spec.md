## ADDED Requirements

### Requirement: Third-party agent packs stay out of the user bundle

User skills MUST remain under `agents/skills/` and ship in the CLI. Contributor Magelift skills MUST remain under `contrib/skills/` for this change. `.agents/skills/` MUST stay gitignored for third-party packs. `magelift skills` MUST NOT install contributor or third-party packs.

#### Scenario: User skill list ignores .agents

- **WHEN** a user runs `magelift skills list`
- **THEN** only `agents/skills/` names appear
