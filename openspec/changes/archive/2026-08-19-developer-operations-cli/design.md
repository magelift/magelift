# Design: developer operations CLI contracts

## Boundaries

The CLI owns intent validation, output formatting, exit-code mapping, secret
redaction, and context budgets. Provider and runtime modules own API calls,
provider query syntax, pagination, eventual consistency, and inventory details.
The shared contracts carry normalized facts and explicit unsupported states.

The design reuses the existing `RuntimeObserve`, `CostEstimator`, `ExecTarget`,
`health.Report`, output writer, and module registry. A new interface is added
only where two or more provider implementations need the same operation. A
provider with no supported capability returns the existing typed unsupported
path instead of receiving a fake implementation.

## Health and logs

`health` remains a single command with explicit evidence modes, but each mode
returns the same report shape. A report contains environment and target
identity, observation time, overall status, checks, source, freshness, and an
optional remediation reference. A check status is one of healthy, unhealthy,
degraded, stale, or unavailable. Configuration validity must not be presented
as runtime health.

The CLI keeps the existing unhealthy and unavailable exit-code behavior. A
degraded or stale report has a non-zero code chosen with the existing error
mapping and documented before implementation. JSON and YAML include the status
and every check; table output is a projection of the same report.

`LogQuery.Filter` is part of the adapter contract. A provider adapter either
translates it into a provider query safely or returns an explicit unsupported
error before reading logs. The normalized result carries event time, workload,
source, and message. Adapters must define ordering, duplicate handling,
pagination or bounded truncation, and behavior when one pod or stream cannot be
read. Messages and provider metadata are redacted before they reach output or
evidence.

## Cost and budgets

The existing estimate path remains useful and is labeled as an estimate. A
provider-backed cost report distinguishes:

- planned resource inputs and estimated cost;
- live price lookup and its source timestamp;
- actual or forecast spend, when the provider exposes it;
- budget limit, period, threshold state, and currency; and
- unavailable or stale values with the reason and next action.

The CLI never converts an estimate into actual spend and never reports a budget
as enforced unless the provider confirms the owned budget object or policy.
Budget reads and mutations use an ownership marker and an explicit scope. A
mutation must have a plan and confirmation path, must not log secret values, and
must refuse ambiguous or unowned objects. GCP is the first live implementation;
AWS follows within the credit cap, and OVHcloud and Scaleway expose explicit
unsupported rows until their authenticated APIs and semantics are verified.

## Remote access

`ssh` and `exec` continue to resolve through `RuntimeObserve.PrepareExec` and
return an argv-safe `ExecTarget`. The target records provider, runtime,
environment, service, container, launcher, arguments, timeout, and cleanup
paths. The CLI never builds a shell command by concatenating untrusted input.

`--session-only` remains a read-only preview. Preview output redacts tokens,
kubeconfigs, signed URLs, and credential references while preserving enough
argv shape for diagnosis. A missing or unsupported access path fails before a
provider mutation and states which capability or operator prerequisite is
missing. Context cancellation and timeout must reach the provider client and
the launcher process.

`tunnel` resolves through a separate optional runtime tunnel capability so
existing log and exec adapters do not need a breaking interface change. The
query uses logical targets such as `app`, `db`, `queue`, `queue-ui`, and
`search`; adapters resolve those targets to provider-owned services or
provider-native proxies. Local listeners bind to loopback by default, local
and remote ports are validated, and the returned launcher remains argv-safe.
Managed database access uses a provider-native proxy only when the adapter has
an explicit connection handle and the required launcher is available. A
missing service, an unprovisioned dashboard, or an adapter without a verified
path returns an explicit unsupported result; the CLI never falls back to a
public endpoint or invents a service name. Tunnel cancellation terminates the
launcher and removes temporary credentials or kubeconfig files.

## Local runtime and dependency preflight

`doctor` remains safe to run without provider mutation and adds a dependency
section selected by the requested command or target. A check records the
executable name, detected version when available, capability, required or
optional state, and an install hint for Homebrew, Scoop, or the detected system
package manager. Missing tools are actionable but do not trigger downloads by
default. An explicit install flag may invoke only an allowlisted package-manager
argv after confirmation; arbitrary shell strings and curl-piped installers are
not accepted.

Commands that need Docker, Compose, `kubectl`, `cloud-sql-proxy`, Pulumi, or a
provider session client run the same preflight before creating local volumes,
temporary credentials, provider sessions, or Automation API workspaces.
Release verification has a separate `cosign` preflight so cloud-only deploy,
promote, rollback, and self-update paths do not inherit Docker or Git
requirements. The standalone local-image builder uses only its Git and Docker
contract. Cloud-only commands do not require Docker.

GCP budget reads resolve the billing account attached to the configured project,
query only budgets scoped exactly to that project, and mark provider ownership
and deployment enforcement separately. Account-wide or multi-project budgets
are omitted. Current and forecast spend remain unavailable until a supported
billing report or export adapter supplies them.

The AWS budget adapter uses the official read-only Budgets operations with the
configured account ID, paginates within a bounded read limit, keeps only cost
budgets, and reads percentage notifications separately. AWS budgets are
account-scoped and do not carry a MageLift environment marker in this path, so
the normalized budget retains actual/forecast values when returned but marks
ownership and deployment enforcement false. OVHcloud and Scaleway return an
explicit unavailable report until their provider billing semantics are verified.

The local runtime uses the existing Compose template and build runner, but its
image tags, service families, PHP and Composer versions, extensions, PHP
settings, and email mode must be resolved from one compatibility catalog. The
project-level `local` block selects a verified service family/version or leaves
the catalog default in place; it cannot change the cloud target. A catalog row
names the supported Magento release range, required PHP and Composer ranges,
service family and version, local image, cloud mappings, and verification
status. Unsupported combinations fail during planning with the nearest
supported alternative; they are never silently downgraded.

The catalog is source-dated rather than inferred from provider defaults. The
current Adobe system-requirements snapshot is the authoritative input for
release rows; for example, its current 2.4.6-p15 through 2.4.9 rows list
RabbitMQ 4.2, while older MageLift runtime evidence used 4.3. The source
catalog and pinned provider defaults must follow the current row, and older
evidence remains historical until the current combination is reverified.

Each local service row must include a pinned image digest, health probe,
ports, credentials wiring, and the Magento connection shape. If a row has no
verified image or protocol contract, planning returns the gap before Compose
creates a volume. Managed Aurora, S3, ElastiCache, OpenSearch, MQ, email, and
other provider services are represented as explicit local substitutes or
unsupported provider-only capabilities rather than being silently replaced.

## Provider registration

Provider/runtime modules advertise operator capabilities through the existing
module registry. The CLI asks the registry for a capability and does not branch
on provider names. Capability descriptors identify whether a result is read,
write, or interactive, the required credential references, the supported
environment classes, and the evidence level.

## Verification and rollout

Offline fake-adapter tests cover status normalization, filters, redaction,
pagination, unsupported paths, timeouts, and exit codes. Provider contract tests
run against the official SDK clients with no network. Live checks create only
marker-owned objects, record source and observation timestamps, and assert exact
cleanup through the owning service.

The generated CLI reference, provider capability matrix, and user-facing skill
bundle are updated from the same contract data. Maintainer implementation
guidance stays in contributor documentation and is not shipped as an
installable end-user skill.
