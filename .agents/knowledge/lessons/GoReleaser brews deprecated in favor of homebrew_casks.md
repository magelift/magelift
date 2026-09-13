---
type: lesson
title: GoReleaser brews deprecated in favor of homebrew_casks
description: GoReleaser deprecated brews (formula generation) since v2.10 soft / v2.16 hard; CLIs now ship as Homebrew casks with binary stanzas, which also work on Linuxbrew.
tags: [goreleaser, homebrew, release]
status: stable
generated:
  by: cursor/mac
  at: 2026-08-04
---

# GoReleaser brews deprecated in favor of homebrew_casks

`brews` in `.goreleaser.yaml` triggers a deprecation warning from
`goreleaser check` (soft since v2.10, hard since v2.16). Deprecated options are
only removed on major versions, so `brews` still works in v2.17, but
GoReleaser's stated direction is casks.

What changed upstream: Homebrew casks now install plain binaries via `binary`
stanzas, including on Linuxbrew, which was the historical reason for
GoReleaser's "hackyish" formulas. For a CLI like magelift the new shape is:

```yaml
homebrew_casks:
  - name: magelift
    repository: { owner: magelift, name: homebrew-tap, branch: main,
                  token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}" }
    homepage: https://magelift.dev
    description: Open-source Magento CLI for your AWS or GCP account
    license: Apache-2.0
    binaries:
      - magelift          # plural since v2.12.6; singular binary is deprecated
```

Migration gotchas from the official deprecation page:

- Do **not** carry over `directory: Formula` — casks default to `Casks/`.
- Migrating an existing tap formula: add `tap_migrations.json`
  (`{"magelift": "magelift"}`) and delete `Formula/magelift.rb` so brew moves
  users to the cask. Fresh taps (ours) need no migration file.
- Scoop conventions: manifest goes in `bucket/`, and the Windows archive
  should be zip — added as a `format_overrides` entry for `goos: windows`.

Verified 2026-08-04 against https://goreleaser.com/deprecations#brews and
`goreleaser check` (v2.12.7 binary, config targets v2.17 schema).
