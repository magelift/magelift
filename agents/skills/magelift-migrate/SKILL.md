---
name: magelift-migrate
description: >-
  Migrate an Adobe Commerce Cloud, Platform.sh, or Upsun project to MageLift.
  Use when importing configuration, mapping services, or reviewing unmapped fields.
version: 1.0.0
---

# Migrate to MageLift

Use the importer to create a reviewable starting point. It is not a promise
that every source platform feature has a direct equivalent.

## Working rules

- Run `magelift init --from-acc` or `magelift init --from-upsun` into a new file.
- Review the generated `.unmapped.md` sidecar before changing credentials or DNS.
- Keep secrets as provider secret references. Never put API tokens, private keys,
  or Composer auth in YAML or evidence.
- Preserve Fastly service identity, domains, TLS intent, purge behavior, and
  custom VCL as an operator-owned reference when it cannot be translated.
- Validate the imported file before preview.

## Report

List every manual follow-up: DNS, certificates, edge rules, cron changes,
database import, media sync, search reindex, queue cutover, and rollback plan.
