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

- Run `magelift init --from-acc` or `magelift init --from-upsun` into a new PHP
  project. Select the destination with `--provider aws`, `gcp` or `azure`;
  use `--app` to select a Magento app in a multi-app Upsun project.
- The importer reads source-platform YAML such as `.magento.app.yaml` and
  `.upsun/*.yaml`; the MageLift project it creates is PHP.
- When the import reports unmapped fields, review the adjacent
  magelift.unmapped.md before changing credentials or DNS. A custom output
  name receives a corresponding named report.
- Keep secrets as provider secret references. Never put API tokens, private keys,
  or Composer auth in the project file or evidence.
- Preserve Fastly service identity, domains, TLS intent, purge behavior, and
  custom VCL as an operator-owned reference when it cannot be translated.
- Validate the imported file and compare environments before preview. The
  importer maps configuration; it does not move database or media data.

## Report

List every manual follow-up: DNS, certificates, edge rules, cron changes,
database import, media sync, search reindex, queue cutover, and rollback plan.
