---
status: draft
slug: yaml-v4-final
---

# Intent: drop the YAML v4 release-candidate pin when v4 is final

## Problem

MageLift uses `go.yaml.in/yaml/v4` because v3 is frozen except for security and upstream recommends v4 for new work. There is no final v4 yet. The pin is `v4.0.0-rc.6`. Each RC bump has to pass strict-decoding, merge, and provenance tests. Operators and the schema cannot be on a "YAML 4" final until that tag exists.

## Evidence

`docs/dependency-policy.md` states the RC must pass those tests before each update. `go.mod` after the 2026-09-12 bump still requires `go.yaml.in/yaml/v4 v4.0.0-rc.6`. A final v4.0.0 (or later) on the module proxy: not checked at the moment of writing beyond that bump's `go list`.

## Proposed outcome

`go.mod` pins a final YAML v4. Magelift.yaml decode, merge, and provenance tests pass without RC exceptions. Dependency policy no longer talks about a release candidate.

## Affected users and systems

`magelift.yaml` loading, config schema, CLI, generated JSON Schema. Every command that reads YAML.

## Constraints

Strict decoding stays. No secret values in YAML. RC must not be replaced by a different incompatible YAML library. Public config contract stays YAML-only. Blocked on upstream and off the v1 path (see ROADMAP.md): there is nothing to do until the module proxy shows a final v4 tag, and the current RC pin passes strict-decoding, merge, and provenance tests.

## Out of scope

Migrating shops off YAML. json/v2. Encoding Magento `env.php`.

## Open questions

If final v4 changes RC decode behaviour, do we need a `magelift.yaml` schema version bump, or is it a lockfile-only bump?

## Status check 2026-09-15

Proxy still lists `v4.0.0-rc.6` as latest; no final tag. Intent stays
draft, blocked on upstream. Re-check with the end-of-roadmap sweep.
