---
type: lesson
title: MkDocs 2.0 breaks Material — docs toolchain exit options
description: MkDocs 2.0 (rewrite, no plugins, TOML config, no migration path) will never run Material for MkDocs; our pins protect today's builds, and the planned exits are Zensical or Astro Starlight.
tags: [mkdocs, material, docs, zensical, starlight]
status: stable
stale_after: 2027-02-04
generated:
  by: cursor/mac
  at: 2026-08-04
---

# MkDocs 2.0 breaks Material — docs toolchain exit options

Material for MkDocs 9.7.2+ prints a build warning about MkDocs 2.0. Facts from
the Material team's analysis (squidfunk blog, 2026-02-18, updated through
April 2026):

- MkDocs 2.0 is a ground-up rewrite by MkDocs' new maintainer: plugin system
  removed, navigation passed to themes as pre-rendered HTML (kills tabs and
  collapsible sections), TOML config incompatible with `mkdocs.yml`, closed
  contribution model, no license announced, no release date.
- Material for MkDocs will never be compatible with 2.0.
- MkDocs 1.x is unmaintained (18 months without releases as of early 2026);
  security posture uncertain.
- Material 9.7.5+ caps its dependency at `mkdocs<2`, so builds cannot
  accidentally pull the incompatible release.

MageLift posture (2026-08-04): exact pins (`mkdocs==1.6.1`,
`mkdocs-material==9.7.7` in `docs/requirements.txt`), so builds are stable.
Material-specific surface is minimal: `docs/overrides/main.html` (extrahead
only), `docs/stylesheets/magelift.css`, theme feature flags, pymdownx
extensions. No third-party plugins.

Exit options, to revisit when 2.0 gets a release date or Zensical matures:

1. **Zensical** (Material team's SSG) — targets drop-in MkDocs 1.x
   compatibility; smallest migration.
2. **Astro Starlight** — converges docs into `website/` (one npm toolchain,
   one design system; the landing is the only site). Cost: port nav config,
   the extrahead override, and pymdownx admonitions to Starlight syntax.
3. **Stay on 1.x/9.x** — acceptable medium term; risk is unmaintained-core rot.

Decision recorded 2026-08-04: stay put, revisit in ~6 months. To silence the
build warning: `NO_MKDOCS_2_WARNING=1`.

Also noted: March 2026 PyPI ownership incident around the `mkdocs` package is
a supply-chain argument for hash-pinning `docs/requirements.txt`
(`pip install --require-hashes`) if the docs stack grows more dependencies.
