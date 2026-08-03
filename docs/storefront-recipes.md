# Storefront recipes (docs only)

MageLift does **not** ship an in-core Next.js / PWA / OpenNext runtime. Magento
remains the commerce backend; storefront frameworks stay in your repo.

## Headless mode

Set `application.mode: headless` when Magento is API-only. Wire the storefront
to Magento GraphQL / REST using outputs from `magelift outputs` (base URL,
secrets via your secret store — never bake Composer auth into the frontend
image).

## Typical wiring

1. Deploy Magento with MageLift on a certified target.
2. Read `applicationURL` (and related) from `magelift outputs --env <env>`.
3. Configure the storefront's Magento backend URL + auth to Magento integration
   tokens stored in Secrets Manager / Secret Manager.
4. Keep CDN / edge for the storefront separate unless you intentionally share
   the MageLift edge cell (check the capability matrix before claiming it).

## Out of scope for v1.0

- Generating Next/PWA apps inside the CLI
- Signed remote storefront plugins
- Claiming Adobe PWA Studio compatibility beyond "bring your own frontend"

See [getting-started.md](getting-started.md) and [post-beta-roadmap.md](post-beta-roadmap.md).
