## Why

OpenSpec is supposed to be MageLift's product contract, but `openspec/specs/` was empty: every requirement lived as a delta inside a change, completed changes were never archived, and several product areas from the intended CLI (environments, Magento build/deploy, email, security, distribution, compliance) existed only in docs, code, or conversation. Another agent cannot treat OpenSpec as the backlog until those requirements are in specs with priorities and testable acceptance criteria.

## What Changes

- Promote the 24 existing change specs into `openspec/specs/` as the current product baseline.
- Add the missing product capabilities listed below as testable requirements, not slogans.
- Tighten existing capabilities where the current text is too narrow or too vague.
- Replace `openspec/BACKLOG.md` with one ordered execution index that answers "what is the highest-priority incomplete OpenSpec task?"
- Leave the two in-progress implementation changes (`provider-native-lifecycle-adapters`, `multi-cloud-resilience-observability-edge`) as the owners of their remaining live-evidence tasks. This change does not re-checklist those rows.

No **BREAKING** CLI or YAML change is proposed. Command names stay with the existing tree (`magelift dev`, `env`, `deploy`, `health`, `doctor`). FrankenPHP worker mode stays P3; `frankenphp-classic` remains an evidenced alternative and must not block the nginx-fpm path.

## Capabilities

### New Capabilities

- `product-scope`: What MageLift is, who it is for, what it is not, provider order, and the P0–P3 rule.
- `cli-contract`: Scriptable CLI UX, non-interactive mode, exit codes, output formats, destructive confirmation, and actionable errors.
- `environment-model`: Project vs environment, fixed vs preview, inheritance, naming/tags, state, locking, drift, and lifecycle.
- `magento-build-deploy`: Magento-aware build and deploy phases analogous in purpose to `ece-tools`, without vendoring Adobe source.
- `php-runtime-customization`: PHP versions, `php.ini`, extensions, presets, and container consistency. FrankenPHP worker is P3.
- `transactional-email`: Pluggable SendGrid, SES, and provider equivalents; local SMTP/Mailpit; secrets and env-specific behavior.
- `magento-aware-security`: Least-privilege defaults plus Magento-safe WAF (admin, checkout, GraphQL, REST, static/media, payment callbacks).
- `data-operations`: Database dump, retrieve, restore, tunnels, credentials, and later sanitization.
- `compliance-enabling-controls`: Technical controls useful under SOC 2 / ISO 27001. MageLift does not claim to certify the customer.
- `installation-and-distribution`: Binaries, Homebrew cask, Scoop, checksums/signing, upgrade, changelog, provider packages.
- `engineering-standards`: Architecture constraints (KISS/YAGNI/capabilities), comments, user docs, humanizer, and watermarks.
- `cross-cutting-invariants`: Composition rules that fail when specs are implemented in isolation (delete vs backup, WAF vs webhooks, rollback vs migrations, and the rest of the audit list).

### Modified Capabilities

- `skill-installation`: User skills vs contributor-only skills. Contributor skills must not ship in the user bundle.
- `compatibility-catalog`: Supported/unsupported combinations, default stacks, and a dated expansion strategy (PHP, nginx, Varnish, Redis/Valkey, OpenSearch, MySQL/MariaDB, RabbitMQ/Artemis).
- `local-runtime-and-preflight`: Local command contract (`dev`, not `local`), Mailpit as an explicit gap, and local TLS/domain behavior.
- `operator-health-and-logs`: Layered health (infrastructure, service, Magento, dependency, deployment) and `doctor` vs `health`.
- `native-and-fastly-edge`: Magento-safe WAF is required for production edge; unmodified vendor CRS is not Magento-ready.
- `cost-and-budget-operations`: Unexpectedly expensive resource detection and preview-environment budget defaults.
- `efficient-cloud-certification`: Test pyramid, Floci/`act`, Cloudflare `acourtiol.com`, GCP→AWS→Scaleway→OVH order, and ~$200 credit discipline.
- `provider-extension-loading`: Independent provider versioning and first-party vs community release shape.
- `multi-cloud-runtime-architectures`: Explicit Magento production building blocks and managed-service equivalents.
- `resilience-and-disaster-recovery`: Bounded, observable auto-healing.
- `configuration-layers`: Schema versioning, unknown and deprecated fields, and explicit migrate.
- `provider-aware-remote-access`: Optional temporary management UIs.

## Impact

Planning artifacts only. No application code in this change. Implementation continues from `BACKLOG.md`, which points at existing open tasks first (GCP public TLS / Magento-origin / Armor data-plane) and then at the new P1–P3 rows introduced here.

Affected later: CLI exit-code docs, local Mailpit, cloud email adapters, dump retrieve, Homebrew/Scoop publish, compliance evidence export, and Magento-safe WAF data-plane (already owned in part by provider edge tasks).
