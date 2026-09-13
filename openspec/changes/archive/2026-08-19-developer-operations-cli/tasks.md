## 1. Shared operator contracts

- [x] 1.1 Inventory the existing health, log, cost, budget, `ssh`, and `exec`
  command behavior and record compatibility constraints before changing public
  output.
- [x] 1.2 Add normalized status, freshness, source, capability, and redaction
  types only where existing SDK/platform types cannot carry the required facts.
- [x] 1.3 Add deterministic JSON, YAML, and table snapshots plus documented exit
  code tests for healthy, degraded, unhealthy, stale, unavailable, partial, and
  unsupported results.

## 2. Health and logs

- [x] 2.1 Make the Kubernetes observer apply `LogQuery.Filter` safely and add
  tests for matching, non-matching, time-window, limit, ordering, and partial
  pod reads.
- [x] 2.2 Audit AWS, GCP, OVHcloud, and Scaleway log adapters for equivalent
  filter, pagination, redaction, and timeout behavior; add explicit unsupported
  paths where provider syntax cannot be translated.
- [x] 2.3 Reconcile health modes into one report contract and add application,
  infrastructure, recovery, edge, and observability evidence statuses without
  turning configuration validity into runtime health.
- [x] 2.4 Add fake-adapter and provider contract tests, then run bounded GCP
  health/log cells with exact ownership-scoped cleanup where resources are
  required. The shared Kubernetes observer now sets the
  `PodLogOptions.Container` value from the workload-matching container, with
  single- and multi-container tests; the focused fake/provider suites pass.
  The [2026-08-15 GCP operator matrix](../../../docs/evidence/gcp-operator-matrix-live-2026-08-15.md)
  records complete logs, runtime health, exec, search health, and final
  `assert_clean ok` for marker `gcpop20260815b`.

## 3. Cost and budget operations

- [x] 3.1 Extend cost output to label estimates, live prices, actual spend,
  forecasts, budget state, currency, source, timestamp, and freshness.
- [x] 3.2 Implement the first read-only GCP budget and spend adapter with
  ownership and scope checks, then prove it against the configured project
  without logging credentials. The project-scoped Budget API read path now
  resolves both the configured project ID and numeric project number, keeps
  exact single-project scopes, and explicitly reports no MageLift ownership,
  no deployment enforcement, and no actual/forecast spend from this API. The
  live CLI proof is recorded in
  [`gcp-budget-live-2026-08-15.md`](../../../docs/evidence/gcp-budget-live-2026-08-15.md);
  a separate billing export/report adapter remains required before claiming
  actual or forecast spend.
- [x] 3.3 Implement the read-only AWS Budgets adapter within the live credit
  cap and add explicit unavailable rows for OVHcloud and Scaleway until their
  authenticated budget semantics are verified. AWS reads account-scoped cost
  budgets and percentage notifications with per-budget actual/forecast values,
  while keeping MageLift ownership and deployment enforcement false; live
  account proof remains part of 5.3.
- [x] 3.4 Confirm that the stable CLI exposes no budget mutation. `cost
  --budget` is read-only, and `monthlyBudgetCents` is a planning input rather
  than a provider budget; therefore there is no mutation path requiring plan,
  confirmation, idempotency, reconciliation, or cleanup behavior.

## 4. Provider-aware remote access

- [x] 4.1 Audit every module's `PrepareExec` result for argv boundaries,
  launcher identity, timeout propagation, credential references, service
  support, and redaction.
- [x] 4.2 Add contract tests for `ssh`, `exec`, `--session-only`, cancellation,
  unsupported runtimes, and shell-metacharacter arguments.
- [x] 4.3 Implement the minimum missing access adapters for GCP, OVHcloud, and
  Scaleway where the provider/runtime officially supports the target; keep
  unavailable paths explicit.
- [x] 4.4 Replace the unconditional tunnel error with a capability-driven
  tunnel command. Support verified Kubernetes service forwarding for the
  application, queue, queue management UI, and search API, plus the GCP
  private Cloud SQL Auth Proxy path; validate loopback ports, session-only
  output, cancellation, cleanup, and explicit unsupported dashboard/database
  paths.

## 5. Release and documentation gates

- [x] 5.1 Regenerate command reference, capability matrix, and user-facing
  skill documentation from the verified contracts; keep contributor guidance
  outside the installable skill bundle.
- [x] 5.2 Run Go tests, static checks, workflow validation, OpenSpec strict
  validation, secret-safety checks, and local command smoke tests.
- [x] 5.3 Run the GCP-first live operator matrix, record evidence and exact
  cleanup, then decide whether AWS, Scaleway, and OVH cells fit their remaining
  credit and time budgets. The 12-cell GCP preview matrix passed with 42
  disposable resources and exact marker cleanup; the [live record](../../../docs/evidence/gcp-operator-matrix-live-2026-08-15.md)
  documents the Service Networking dependency recovery and final inventory.
  AWS, Scaleway, and OVH cells were not started in this pass so their limited
  credits remain reserved for targeted provider-specific gates; their broader
  matrices remain open elsewhere in OpenSpec.

## 6. Local runtime and dependency preflight

- [x] 6.1 Inventory every external executable used by cloud, tunnel, build,
  import, and local Compose commands; classify it as required or optional and
  record verified Homebrew, Scoop, and system install hints. The inventory also
  covers indirect Pulumi Automation API use, cosign verification, and the
  standalone local-image builder.
- [x] 6.2 Add a provider-neutral dependency check/report with version probes,
  capability ownership, actionable install hints, and stable JSON/YAML/table
  output. Keep automatic installation opt-in, allowlisted, confirmed, and
  free of arbitrary shell execution.
- [x] 6.3 Gate `dev`, local image builds, Kubernetes exec/log/tunnel paths, and
  GCP database tunnels with the preflight; add tests that prove missing tools
  fail before local volume, provider-session, backend, journal, or standalone
  image-build mutation. Pulumi-backed lifecycle, output, health, media, and
  preview-sweep paths and cosign verification paths are included.
- [x] 6.4 Reconcile the Docker Compose template and build protocol with the
  compatibility catalog for supported Magento releases, PHP/Composer versions,
  database families, RabbitMQ and Artemis, Valkey and Redis, OpenSearch,
  Varnish/nginx, email modes, PHP settings, and PHP extensions.
  Use the current Adobe system-requirements snapshot as the source of truth;
  historical rows that used RabbitMQ 4.3 do not certify the current 4.2 row.
  Catalog-backed local rows cover 2.4.6-p15 through 2.4.9 and cross-check their
  PHP, Composer, database, cache, search, queue, web-server, and web-cache
  choices against the shared source-dated compatibility catalog before Compose
  is written. RabbitMQ 4.2 and Artemis 2.38.0 are verified through their local
  image, health, port, and Magento connection contracts; Artemis uses STOMP and
  Jolokia. Valkey is the supported cache row. Redis 7.2 remains an explicit
  unsupported exception that requires `compatibility.allowUnsupported: true`.
  nginx runs the PHP-FPM runtime image, Varnish 8 runs as the optional front
  cache, and both have tested health paths. SMTP, SendGrid, and SES normalize to
  Magento `system.smtp`; the generated `MAGENTO_DC__OVERRIDE` carries the same
  tree into bind-mounted checkouts and resolves credentials from an environment
  variable inside the app container. Mailpit remains an explicit gap. The
  generated PHP settings file is mounted into the app, the FrankenPHP image is
  verified against the
  supported local extension baseline, and the isolated build pipeline checks
  the builder's PHP, Composer, and extension response before BuildKit creates
  the application image.
- [x] 6.5 Document local-versus-cloud deltas and generate the user-facing
  dependency and local-runtime skill pages without placing contributor-only
  guidance in the installable skill bundle.
