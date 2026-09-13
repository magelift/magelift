---
type: lesson
title: Magento exec known-content must skip CLI warning noise
description: 'magelift exec prints experimental-target warnings before PHP stdout; taking the first non-+/{ line treats warning: as the Magento label.'
tags:
- gcp
- magento
- ha
- openspec
status: stable
generated:
  by: cursor-grok/desktop
  at: '2026-08-18'
---

`gcha23` planted `magelift_seed_probe.label=tiny-fixture` and the web pod
came back 3/3 after pod-loss. `magento-seed-probe` still failed. The exec
transcript was:

1. `+ magelift exec ...`
2. `warning: target gcp/gke-standard is experimental: ...`
3. `Defaulted container "php-fpm" out of: php-fpm, web`
4. `tiny-fixture`

A parser that only skips `+` and `{` prefixes takes the warning line as the
observed label. Write the PHP scalar with a trailing newline so later JSON or
probe errors cannot glue onto the value.

Skip `warning:` and `Defaulted ` lines in both the harness awk and
`firstMagentoCatalogSKULine`. Do not treat a parser miss as missing Magento
content, and do not mark 3.6 complete from `gcha23`.

## Related

See [GCP HA Magento known-content must use the seed fixture](GCP%20HA%20Magento%20known-content%20must%20use%20the%20seed%20fixture.md).
