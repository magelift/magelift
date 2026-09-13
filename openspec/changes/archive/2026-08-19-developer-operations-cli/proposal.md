# Proposal: developer operations CLI contracts

## Why

MageLift already exposes `cost`, `health`, `logs`, `ssh`, and `exec`, but the
operator contract is not complete enough for a small team to trust the CLI as
the daily interface to a deployed Magento environment.

The current audit found six concrete gaps:

- `cost` describes estimates and optional on-demand prices, but does not expose
  a provider-backed budget, actual spend, forecast, or budget state.
- `health` has configuration, output, and runtime modes, but no consistent
  operator report that distinguishes healthy, unhealthy, stale, and unavailable
  evidence across application, infrastructure, recovery, and edge checks.
- `logs --filter` is part of the public command surface, while the Kubernetes
  observer currently does not apply the filter. Provider adapters also need a
  consistent contract for ordering, pagination, redaction, and partial reads.
- `ssh` and `exec` are routed through runtime adapters, but the supported access
  capabilities, session-only preview, timeout, identity, and provider-specific
  failure messages are not documented as one contract.
- `tunnel` is currently a Fargate-only error path, so operators cannot reach
  private databases, queues, search services, or their verified management
  interfaces through a provider-owned access path.
- `doctor` validates configuration but does not preflight the local binaries
  needed by Docker-based development, Kubernetes access, or provider-native
  tunnel proxies. `dev` therefore fails later with a generic process error.
- The local Compose contract is useful but is not yet tracked as a verified
  compatibility matrix for the supported Magento releases, PHP and Composer
  versions, database and broker families, cache/search services, email
  adapters, and PHP settings or extensions.
- The current Adobe requirements snapshot lists RabbitMQ 4.2 for the supported
  2.4.6-p15 through 2.4.9 rows, while older MageLift evidence and one source
  catalog row used 4.3. Historical evidence cannot silently stand in for a
  current compatibility check.

Without these contracts, a command can return a plausible result while hiding
an unsupported capability or silently ignoring an operator's request. That is
especially dangerous for cost decisions, incident response, and production
access.

## What changes

- Define provider-neutral operator reports for health, logs, cost and budget
  state, and remote access.
- Make every unsupported, unavailable, stale, or partially observed result
  explicit in human and machine-readable output.
- Make log filters effective in every adapter that accepts them, or fail closed
  with a typed unsupported result before querying the provider.
- Extend cost reporting to distinguish estimates, live prices, actual or
  forecast spend, budget limits, and the timestamp and source of each value.
- Keep remote commands provider-neutral at the CLI boundary while requiring
  runtime adapters to return an argv-safe launcher, bounded context, and
  capability-specific diagnostics.
- Add provider-aware tunnels for private service endpoints. A tunnel may use
  Kubernetes port-forwarding or a provider-native proxy, but it must bind to
  localhost, validate ports and target ownership, and fail explicitly when a
  service or management UI is not provisioned.
- Add a dependency preflight report with executable, version, required versus
  optional capability, and platform-specific install hints. The CLI may offer
  an explicit package-manager install path only after confirmation; it must not
  execute arbitrary download commands or mutate a workstation silently.
- Make local Docker Compose services and Magento build inputs derive from the
  same compatibility catalog as cloud plans, with explicit intentional gaps for
  provider-only features and tests for database, broker, cache, search, email,
  PHP, Composer, settings, and extension combinations.
- Add contract, fake-adapter, provider acceptance, secret-safety, and generated
  documentation tests. Start live budget and operator checks with GCP, then use
  the existing AWS, Scaleway, and OVH credit limits deliberately.

## Capabilities

- `operator-health-and-logs`: dependable health summaries and filtered log
  retrieval with explicit evidence status.
- `cost-and-budget-operations`: safe cost and budget visibility without
  inventing data when a provider does not expose the requested signal.
- `provider-aware-remote-access`: predictable `ssh` and `exec` capability
  discovery and execution through provider/runtime adapters, including
  verified private tunnels for supported data and management services.
- `local-runtime-and-preflight`: actionable dependency checks and a Docker
  local runtime that is traceable to the Magento compatibility catalog.

## Impact

- `sdk/v1` and `internal/platform` gain only the smallest contracts needed to
  carry operator intent and evidence. Existing provider-neutral boundaries stay
  free of AWS, GCP, OVHcloud, Scaleway, Fastly, or New Relic fields.
- Provider modules implement capabilities through the existing registry rather
  than adding provider switches to the CLI.
- Existing commands retain their names and default output. New fields and
  subcommands must be deterministic and documented before they become part of
  the stable surface.
- No command may claim live spend, health, log filtering, remote access, or
  local compatibility solely because a configuration value or Pulumi output
  exists.
