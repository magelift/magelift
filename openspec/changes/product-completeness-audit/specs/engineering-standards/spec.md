## Purpose

Records architecture, documentation, and contributor-prose rules that keep MageLift maintainable without turning OpenSpec into an implementation diary.

## ADDED Requirements

### Requirement: Simple implementations over speculative ones

MageLift MUST prefer the smallest correct change: stdlib and existing modules before new dependencies, concrete types before a one-implementation interface, and explicit provider capabilities before generic conditionals. Cloud-neutral concepts MUST live in generic contracts. Provider differences MUST NOT leak into generic commands as `if provider ==` branches. Comments MUST explain non-obvious decisions, provider or Magento quirks, invariants, and security-sensitive behavior. Comments MUST NOT narrate what the next line does.

#### Scenario: A new provider service is added

- **WHEN** a provider gains a managed service with no portable equivalent
- **THEN** the adapter exposes a named capability or namespaced field, and generic commands either use that capability or return typed unsupported

### Requirement: User documentation exists where operators need it

User documentation MUST cover install, getting started, configuration, local vs cloud, operations, and the capability matrix for certified cells. MageLift MUST NOT require documentation that restates code without adding an operator-facing fact. Generated CLI reference MAY exist; handwritten pages MUST stay consistent with the command tree.

#### Scenario: Certified cell is documented

- **WHEN** a matrix cell is marked certified
- **THEN** the capability matrix names the cell, its evidence class, and the commands that work on that target

### Requirement: Humanizer and watermarks apply to contributor prose, not user skills

Contributor sessions that write user-facing documentation, website copy, or comments longer than a short status MUST run the humanizer skill against that prose before it is committed. Applicable documentation and comments MUST be cleaned of AI provenance marks using the watermarks skill when that skill is part of the contributor toolchain. Humanizer and watermarks MUST remain contributor-only; they MUST NOT appear in `magelift skills list` or the user skill bundle under `agents/skills/`.

#### Scenario: User skill list excludes contributor toolchain

- **WHEN** a user runs `magelift skills list` from a release binary
- **THEN** the list contains only MageLift user skills and does not include humanizer, watermarks, Pulumi, or Golang contributor skills
