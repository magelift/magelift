## ADDED Requirements

### Requirement: Generated CI uses magelift local and cloud verbs

Generated workflows MUST call `magelift local` for laptop-equivalent jobs when those jobs exist, and MUST NOT call `magelift dev`. Cloud jobs MUST use `magelift deploy`, `health`, and preview identity flags with `--no-interaction`.

#### Scenario: Generated workflow has no magelift dev

- **WHEN** `magelift ci generate` writes a workflow
- **THEN** the file does not invoke `magelift dev`
