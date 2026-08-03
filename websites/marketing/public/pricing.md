# Pricing: MageLift

## MageLift software

- Price: $0
- License: [Apache-2.0](https://github.com/magelift/magelift/blob/main/LICENSE)
- Limits: none on the CLI (you operate inside your own cloud quotas)
- Includes: config validate, preview, deploy, destroy, day-2 ops on certified targets, ACC/Upsun-shaped YAML, local Compose path (`magelift dev`)

There is no MageLift SaaS subscription, seat fee, or "contact sales" plan for the open-source CLI.

## Cloud spend (AWS or GCP bills you)

MageLift creates resources in your account. Typical costs:

| Item | Who bills you | Notes |
| --- | --- | --- |
| Compute, database, network, logs | AWS or GCP | Prefer free-tier-safe shapes for previews when you can |
| Managed search / brokers | AWS or GCP | Prefer `searchMode: disabled` and `queueMode: db` until you mean to spend |
| MageLift CLI / YAML | Nobody | Free open source |

Estimate AWS catalog shapes:

```sh
magelift cost --env preview
```

Destroy when done:

```sh
magelift destroy --env preview --yes
```

## Support

- Community: [SUPPORT.md](https://github.com/magelift/magelift/blob/main/SUPPORT.md)
- Security: [SECURITY.md](https://github.com/magelift/magelift/blob/main/SECURITY.md)

## Related

- Compare to rented Magento PaaS: https://magelift.dev/docs/compare-paas/
- Install: https://magelift.dev/docs/install/
- Capability matrix: https://magelift.dev/docs/capability-matrix/
