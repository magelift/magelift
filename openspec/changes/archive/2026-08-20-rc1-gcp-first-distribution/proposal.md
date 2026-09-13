## Why

RC1 work is blocked by Cartesian live evidence: a single OpenSpec checkbox still
demands paid create/destroy on AWS, GCP, Scaleway, OVH, Fastly, and New Relic,
while CI still pins Floci 1.5.33 and treats floci-gcp as nonexistent. The default
binary also links every cloud SDK, so users download adapters they will never
use and community providers still need a custom CLI. We need a GCP-first packed
certification path that still ships every first-party adapter in `v1.0.0-rc.1`,
with Magento certified on two origins and a signed, independently versioned
provider host.

## What Changes

- Define the RC1 product bar: first-party adapters for GCP, AWS, Cloudflare,
  SendGrid, New Relic, Fastly, OVHcloud, and Scaleway all ship. GCP-first is
  live-test order, not a reduced catalog.
- Certify Magento on GCP GKE Autopilot and AWS ECS Fargate. Prove Cloudflare,
  SendGrid, New Relic, and Fastly on one warm GCP Magento origin. Ship OVH and
  Scaleway as complete adapters with Magento experimental.
- Replace per-cell create/destroy with packed KEEP sessions and move GCS,
  Secret Manager, Pub/Sub, Logging, and Monitoring API contracts to
  digest-pinned floci-gcp. Pin latest Floci AWS and floci-gcp images.
- Specify dual-mode `sdk.Module` loading: in-process for tests and harnesses;
  signed Cosign-verified subprocess binaries plus `magelift.providers.lock` for
  the published CLI. Unsigned remote execution stays refused.
- Split remaining live-evidence tasks in the two in-progress changes so a GCP
  cell cannot keep an AWS/OVH checkbox open. Allow a parallel local track for
  health layers, dump retrieve, and rollback-vs-schema.
- Supersede ADR 0007's AWS-as-v1 / GCP-as-v1.1 / no-download-plugin default
  with this RC1 distribution and certification order.

No **BREAKING** CLI command or YAML schema rename. Selecting an uninstalled
provider MUST fail closed with an install hint instead of silently executing
unsigned code.

## Capabilities

### New Capabilities

- (none)

### Modified Capabilities

- `product-scope`: RC1 adapter set; Magento certified on GCP then AWS; OVH and
  Scaleway Magento experimental; BACKLOG allows one cloud objective plus
  parallel non-provisioning tracks.
- `installation-and-distribution`: Homebrew installs core only; signed
  first-party provider/integration artifacts and a lockfile; in-process
  fallback is a one-release ceiling if extract lags.
- `provider-extension-loading`: dual-mode in-process vs signed subprocess;
  digest-pinned first-party and trusted community install allowed; unsigned
  remote still refused.
- `efficient-cloud-certification`: Floci AWS 1.7.0 and floci-gcp 0.7.0 in the
  pyramid; packed GCP then AWS sessions; integrations attach to the GCP
  origin; emulators never certify Autopilot, Armor, managed TLS, Memorystore,
  Magento Cloud SQL PITR, or regional DR.
- `transactional-email`: SendGrid cloud delivery is an RC1 integration proven
  on the packed GCP Magento origin, not a later P2 afterthought.

## Impact

- OpenSpec: this change's deltas, `openspec/BACKLOG.md`, and remaining
  checkboxes in `provider-native-lifecycle-adapters` and
  `multi-cloud-resilience-observability-edge`.
- Docs: ADR 0011 superseding ADR 0007; `docs/gcp-experimental.md`,
  capability matrix, Floci pins.
- CI/tests: `docker-compose.floci.yml`, new floci-gcp compose and
  `tests/floci-gcp`, parallel GitHub Actions jobs, Makefile targets.
- Later apply (not this change's planning): Go workspace split,
  `magelift-provider-*` release assets, HashiCorp go-plugin host. Magento
  cells stay in-process until that host has a GCP smoke.
