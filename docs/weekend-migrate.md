# Rehearse a PaaS migration

Allow time for one full restore and application test before booking the production cutover. The schedule below is an example for a staging rehearsal.

## Friday evening: config onramp

1. Export ACC / Upsun config you are allowed to share internally.
2. `magelift init --from-acc` or `--from-upsun` → review generated `magelift.php`.
3. Map secrets to Secrets Manager / Secret Manager references (no plaintext).
4. `magelift config validate --env preview` (or staging).

Details: [migrating-from-paas.md](migrating-from-paas.md).

## Saturday: disposable cloud preview

1. Bootstrap once per account/region.
2. Build/promote a digest; deploy **preview** in your cloud account (AWS Fargate
   recommended for first rehearsal).
3. Import a **sanitized** dump (`magelift env import-dump` / harness dump cell).
4. Optional media sync listing-diff locally; live paid media later if needed.
5. `magelift health` / smoke the storefront or API.

Use `examples/sample-shop/magelift.php` as a starting point for the rehearsal. Destroy the preview when idle.

## Sunday: DNS rehearsal (not production)

1. Point an operator-owned preview FQDN at the stack LB (`scripts/cutover-dns-cloudflare.sh`
   or your DNS provider).
2. Verify TLS and Magento base URLs.
3. `--cleanup` the rehearsal record.
4. Write the production cutover runbook (TTL, rollback DNS, maintenance window).

Do **not** treat a preview-host rehearsal as a production storefront cutover.

## After the weekend

- Attach existing AWS VPC/RDS only when greenfield is wrong:
  [brownfield-attach.md](brownfield-attach.md)
- Cost narrative: `magelift cost` + [compare-paas.md](compare-paas.md)
- Storefront frameworks stay external: [storefront-recipes.md](storefront-recipes.md)

Independent of Adobe Inc. Product names are descriptive only.
