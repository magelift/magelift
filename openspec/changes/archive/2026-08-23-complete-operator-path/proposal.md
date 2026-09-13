## Why

Magento/PHP teams with no cloud DevOps still cannot stay on MageLift + YAML from first clone to production: Magento runtime config is missing, `dev` collides with Magento developer mode, local Docker is not iso-prod, and several production-shaped cells exist only as tribal knowledge. There is no public release yet, so the product contract can be completed and the CLI renamed in place.

## What Changes

- Specify the whole operator path for SMEs and agencies (personas as an index, not duplicate MUST novels).
- Add Magento runtime YAML (`env.php` overlays, `CONFIG__*` / `MAGENTO_DC_*`, consumers, store URLs, CORS, admin `frontName`). Quality Patch IDs live only in `magelift.yaml` after import.
- **BREAKING:** replace `magelift dev` with `magelift local`. No alias. Local Docker follows the same YAML as cloud with named substitutes.
- **BREAKING:** web runtime is nginx-fpm only. FrankenPHP and Apache are omitted from schema (no migrate copy; unreleased).
- Map remaining product cells as P0–P3: Magento PHP storefront, Magento-safe WAF, SOC 2/ISO *controls* (not certificates), split cache/session, queue HA as a cold boundary, dense AWS profiles as experimental (warn, do not block), SQS/Pub/Sub as a Magento-module integration (not `queueMode`), lean signed plugins, residency, brownfield, purge, audit vs evidence. Adobe-unsupported stays fail-closed.
- Certification spend: GCP KEEP for slow infra; AWS, OVH, Scaleway, Cloudflare, Fastly, New Relic, SendGrid use thin paid credits. Mocks/Floci first.
- Implementation stays ordered in `openspec/BACKLOG.md`. This change is planning artifacts plus a BACKLOG rewrite, not application code.

## Capabilities

### New Capabilities

- `product-personas`: Index of SME/agency/operator journeys → existing specs → gap IDs. Does not restate deploy/WAF/health requirements.
- `magento-runtime-config`: Magento-shaped YAML for env.php overlays, secret-referenced CONFIG/MAGENTO_DC, consumers vs cron, store/website URLs, cookies, CORS, dual hostname, admin frontName.

### Modified Capabilities

- `product-scope`: nginx-only; `magelift local`; Magento PHP storefront first-class; non-provisioning BACKLOG allowed; paid integrations scarce.
- `cli-contract`: `local` replaces `dev`; edge purge; audit vs evidence; hide Pulumi from the happy path.
- `php-runtime-customization`: nginx-fpm only; FrankenPHP and Apache absent from the schema.
- `local-runtime-and-preflight`: `magelift local` iso-prod Docker; named cloud substitutes; no silent install.
- `magento-build-deploy`: certified integrated PHP storefront path; Hyvä Node compile out of band.
- `magento-patch-lifecycle`: Quality Patch IDs only in `magelift.yaml` after import.
- `configuration-layers`: example architecture YAML under `examples/`; Magento overlays; CORS/URLs.
- `environment-model`: multi-website domains; one Magento project file per client account.
- `multi-cloud-runtime-architectures`: split cache/session; dense experimental profiles; OpenSearch as Magento search vs a box.
- `cross-cutting-invariants`: refuse in-place RabbitMQ 1-node ↔ quorum.
- `compatibility-catalog`: nginx-only web; SQS/Pub/Sub never adobe-supported; Adobe-unsupported fail-closed + `allowUnsupported`; MageLift-experimental warn-only and non-blocking.
- `magento-aware-security`: Magento-safe WAF; soak-then-block is evidence not the production default; admin frontName from YAML.
- `native-and-fastly-edge`: Magento-safe WAF and purge; no unmodified vendor CRS as Magento-protected.
- `compliance-enabling-controls`: SOC 2/ISO control set + `audit`; region/backup residency; no certificate claim.
- `cost-and-budget-operations`: cost before apply, per environment/account.
- `data-operations`: brownfield Magento cutover as P2.
- `efficient-cloud-certification`: thin paid credits; GCP KEEP; paid integrations same as paid clouds.
- `warm-certification-sessions`: attach vendor live tests to packed GCP sessions.
- `external-service-certification`: Cloudflare/Fastly/New Relic/SendGrid scarce credits.
- `transactional-email`: SendGrid live attach-to-GCP-session; no extra Magento origin to send one email.
- `fastly-edge`: same spend rule.
- `provider-observability-and-new-relic`: same spend rule.
- `provider-extension-loading`: P2 lean core + community catalog; Magento stays in-process until Dial is proven.
- `installation-and-distribution`: signed provider download; no unsigned remote exec.
- `skill-installation`: user skills in `agents/skills/`; contributor skills stay in `contrib/skills/`; `.agents/skills/` remains gitignored third-party packs.
- `preview-environment-identity`: indexed from personas (no behavior change unless a gap appears).
- `provider-aware-ci-generation`: indexed from personas (CI is any system, generated workflow).

## Impact

Planning only in this change. Later implementation touches `internal/cli` (`dev` → `local`), generated CLI reference, JSON Schema (`application.webRuntime`), `internal/config`, `internal/localdev`, Magento YAML model, user skills, examples, `docs/cli-reference.md` via `make generate`, and `openspec/BACKLOG.md`.

No public users: **BREAKING** schema and command rename do not need a migrate command. Do not vendor ece-tools. Do not claim customer SOC 2 / ISO / GDPR certification. Do not deploy Next.js or PWA Studio.
