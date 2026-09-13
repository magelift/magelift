---
type: lesson
title: Do not rematerialize magelift destroy after orphan secret delete
description: After force-clean deletes Magento secrets, a later magelift destroy previews CREATE of Cloud SQL instead of finishing teardown.
tags:
- gcp
- destroy
- pulumi
- acceptance
generated:
  by: cursor-grok
  at: '2026-08-18'
---

Once producers are gone and Magento secrets have been deleted outside Pulumi, `magelift destroy` is not a safe retry. On `gcap24` it previewed `+ 7 to create` (Cloud SQL, Valkey, secret versions) and failed reading `gcap24-preview-magento-crypt-key`. The preview did not apply. Finish with `force_clean_orphans` (include Cloud Armor), WIF, buckets, and origin DNS. Inventory is authoritative. Relates to PSA error 9 after Cloud SQL delete and `PULUMI_K8S_DELETE_UNREACHABLE`.
