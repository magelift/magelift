# MageLift architecture and first-release audit

Date: 2026-09-16. Scope: the working tree, including uncommitted work, current
planning documents, and relevant archived verification reports. This is an
engineering review, not a production certification or a claim that every code
path was exercised. The proposed release sequence is in [ROADMAP.md](ROADMAP.md).

## Verdict

Keep the monorepo, Go, Pulumi Automation API, and YAML as the supported user
interface. Those are reasonable choices. The central problem is the breadth of
the promised product relative to the completeness of its operating model.

The current program does not yet deliver the lean core and autonomous,
independently released provider plugins described in the product vision. It has
useful foundations for that architecture, a substantial implementation, and
valuable acceptance evidence. It also retains compiled provider registrations,
core-owned provider knowledge, a narrow subprocess proof, and installation and
module-publication gaps. Calling that boundary finished would be misleading.

I would not freeze these contracts as stable v1 now. I would release a narrowly
supported public alpha after proving one complete GCP Autopilot path, then use
pilot feedback and AWS ECS parity to decide whether the product deserves a
stable contract. GCP is recommended because its existing functional search and
operator evidence reduces the amount of new proof needed. This is an engineering
sequence, not evidence that GCP is the best commercial launch market.

There is no reason to rewrite the whole system. There is a strong reason to stop
adding providers, service combinations, and certification machinery until the
shipped installation can deploy and operate one real shop reliably.

## What the product should promise

A Magento development team should be able to operate a supported architecture
without first becoming a cloud infrastructure team. That is a useful product.
"No DevOps knowledge" becomes dangerous if it implies no operational owner,
no backup policy, no incident response, or no understanding of destructive
changes. MageLift must own the automation, safe defaults, explanations, and
recovery workflow; the customer still owns their account, data, approvals, and
business continuity decisions.

The cost comparison with a PaaS needs more discipline. A CLI can eliminate much
of the platform premium and remove deployment friction. It does not by itself
replace on-call coverage, upgrades, incident response, or a service agreement.
Compare total operating cost and responsibility, not simply the AWS invoice
against a PaaS subscription. Upsun documents monitoring and security services
for its dedicated offering; the exact comparison depends on the purchased tier.
See [Upsun's dedicated-environment documentation](https://fixed.docs.upsun.com/dedicated-environments/security-monitoring.html).

Start pilots with a few agency technical leads who can diagnose and report
failures. Measure time from a clean workstation to a working shop, time to
recover a failed deployment, restore success, and actual monthly cost. A long
provider list and an elaborate internal certification system do not validate
whether anyone will trust this product with their storefront.

## Evidence and limits

The review traced production registration, configuration, the SDK bridge,
subprocess loading, provider operations, deployment orchestration, runtime
health, Magento lifecycle commands, packaging, evidence generation, and the
current intent records. Findings below distinguish source observations from
risks inferred from those observations.

Commands actually run during this audit session before this planning rewrite:

| Check | Observed result | What it establishes |
| --- | --- | --- |
| `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB go test ./internal/deploy ./internal/config ./internal/platform ./internal/providerhost ./sdk/... -count=1` | 494 tests passed across the selected packages | Useful regression evidence for these areas; not a full release gate |
| `make check-clean-room` | Passed | Current clean-room scanner accepted the tree; not a legal provenance opinion |
| `php vendor/bin/phpunit` from `build/` | 128 tests, 286 assertions, 3 failures | The three child-process failures encountered a PHP launcher missing `libargon2.so.1`; 125 tests passed. Fix the environment and rerun before claiming a full PHP pass |
| Production CLI dependency enumeration with `go list -deps` | 1,855 packages, including AWS, GCP, OVH, Scaleway, and Pulumi packages | The shipped registration path is not a lean provider-independent binary |
| `GOWORK=off GOPROXY=off GOSUMDB=off GOFLAGS=-mod=readonly go list github.com/magelift/magelift/sdk` | Failed to resolve the SDK module | Workspace success does not establish consumer module resolution |
| `go run ./cmd/gencertdocs --check` | Failed: the first record in `gcp-operator-verbs-mldp8-20260915.sealed.jsonl` did not match its recorded digest | Evidence integrity is currently a release blocker |

No cloud resources were created, changed, or destroyed for this review. No full
`make verify`, full race run, penetration test, or current public installation
was completed. Source inspection is not a live reproduction. Older reports are
useful evidence of their recorded artifacts and scenarios, not proof of every
behavior in today's dirty working tree.

A source count found 775 Go files and about 176,639 lines under `internal`,
including tests. Production certification code accounted for about 12,039 lines
in 36 files; production CLI code about 7,389 lines in 36 files; the SDK about
6,156 lines in 21 files. These are size indicators, not quality scores. They
explain why expanding the matrix before stabilizing the boundaries is costly.

## Findings

### F01 — Release blocker: provider independence is not achieved

[`cmd/magelift/main.go`](../cmd/magelift/main.go) constructs the CLI through
`NewWithExtensionsAndHooks`. The default wiring in
[`internal/cli/cli.go`](../internal/cli/cli.go) and
[`internal/registry/registry.go`](../internal/registry/registry.go) still includes
provider implementations. Moving imports between packages has improved
organization, but it has not removed those implementations from the shipped
binary. The dependency enumeration confirms this.

Keep provider topology separate, as the existing ADRs require. Make the shipped
core responsible for configuration envelopes, command UX, plugin discovery,
trust, locking, and orchestration policy. Make the provider responsible for its
cloud implementation and its operating capabilities. Prove that separation with
a provider built outside the root module using the public SDK and a core built
without provider SDK dependencies.

Owner: [provider contract](provider-plugin-contract/intent.md), then
[autonomous GCP provider](gcp-autonomous-provider/intent.md).

### F02 — Release blocker: the subprocess path is a proof, not the product

[`internal/cli/subprocess.go`](../internal/cli/subprocess.go) selects the GCP
Autopilot subprocess path, looks for local plugin artifacts, and falls back to
an in-process backend on load or dial errors. There is no complete YAML-driven
provider download path. A rejected checksum or incompatible plugin can therefore
change the implementation selected for the operation. This is not evidence that
an unverified binary executes; it is evidence that the requested execution path
is not enforced.

Day-two operations also remain host-owned. A plugin that provisions a stack but
cannot independently support status, logs, execution, backup, restore, teardown,
and credential refresh is not the autonomous provider described in the brief.

Require explicit compatibility negotiation and capabilities. Fail closed when
the selected plugin fails integrity or compatibility checks. An intentional
migration mode can exist, but must not silently turn plugin errors into embedded
provider execution. Do not treat a process boundary as a security sandbox: the
provider runs with the user's cloud privileges.

Owner: [autonomous GCP provider](gcp-autonomous-provider/intent.md) and
[verified distribution](verified-provider-distribution/intent.md).

### F03 — Release blocker before API freeze: the SDK boundary is too porous

The separate [`sdk/go.mod`](../sdk/go.mod) without third-party dependencies is a
good foundation. However, [`sdk/modules.go`](../sdk/modules.go) exposes
`Program(ModulePlan) (any, error)`, while
[`internal/platform/public_module.go`](../internal/platform/public_module.go)
expects the returned value to be a `pulumi.RunFunc`. That leaves an important
runtime contract outside the public type system.

The public configuration includes raw maps, while core configuration retains
provider enums, provider-specific types, and rejection of unknown providers in
[`internal/config`](../internal/config). The wire protocol in
[`internal/providerhost/hostproto/ping.proto`](../internal/providerhost/hostproto/ping.proto)
uses JSON payloads for planning and execution; it does not establish a complete
independent operations API. Public CLI hooks also expose internal hook types.

Define a small versioned protocol around observable operations, errors,
cancellation, progress, capabilities, configuration validation, and opaque
provider state. Decide deliberately which policy belongs to the core and which
schema belongs to the provider. Keep a stable common YAML envelope; do not force
a lowest-common-denominator cloud resource model. Do not export every existing
internal interface just to make the current package graph compile.

The current ADRs describe a deliberately limited, single-version proof. The
independent release goal is a change to that decision, and needs an approved
specification plus corresponding ADR and human-documentation updates.

Owner: [provider contract](provider-plugin-contract/intent.md).

### F04 — Release blocker: GCP operations can depend on an expired saved token

[`internal/cloud/gcp/runtime/runtime.go`](../internal/cloud/gcp/runtime/runtime.go)
builds kubeconfig from `GetClientConfigOutput().AccessToken`.
[`internal/kube/client.go`](../internal/kube/client.go) reads stack-output
credentials unless a kubeconfig override is supplied. Its own commentary notes
the roughly one-hour token lifetime. GCP operator wiring uses that client path.

The resulting risk is a successful deployment followed by failed operations
when the saved token expires. A manual kubeconfig override does not satisfy the
supported YAML-only operating path. Obtain fresh credentials when operating,
using the provider's authentication mechanism, and test expiry, refresh failure,
restart, and CI identity explicitly. Do not store refreshed secrets in evidence.

Owner: [autonomous GCP provider](gcp-autonomous-provider/intent.md). Verify through
[reference-store acceptance](reference-store-acceptance/intent.md).

### F05 — Release blocker: deployment health can mean only scheduler health

AWS deployment stabilization and health in
[`internal/cloud/aws/deployment/steps.go`](../internal/cloud/aws/deployment/steps.go)
use the same service-health wait. Kubernetes stabilization and health in
[`internal/kube/steps.go`](../internal/kube/steps.go) similarly rely on replica
availability. The traced Kubernetes health check does not establish the intended
observed generation, updated replicas, and image digest. Some resource creation
uses `skipAwait`.

The nginx health endpoint is a static response; the AWS container definition
explicitly describes a health check without Magento. These checks are useful
for infrastructure liveness, but cannot prove a working Magento application.
A separate health command does not repair a deployment that already reported
success too early.

Success must establish the intended rollout and application readiness. Test
stale healthy replicas, PHP/bootstrap failure, failed database migration,
misconfigured search, and unreachable application dependencies. Keep probes
bounded and distinguish readiness from heavyweight functional acceptance.
[Kubernetes documents the deployment conditions and progression model](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/).

Owner: [Magento deployment safety](magento-deployment-safety/intent.md).

### F06 — Release blocker: two Magento lifecycle authorities can diverge

[`build/src/Magento/LifecyclePlan.php`](../build/src/Magento/LifecyclePlan.php)
contains the PHP lifecycle plan. [`internal/platform/workloads.go`](../internal/platform/workloads.go)
contains a separate migration shell sequence, including static-content work.
The Kubernetes candidate migration job in
[`internal/kube/candidate.go`](../internal/kube/candidate.go) does not mount a
shared static-content volume in the traced path. Static assets generated inside
that disposable job are therefore at risk of not reaching serving containers.
This is a source-level concern, not a demonstrated live missing-asset failure.

The traced migration sequence also needs an explicit policy for old web,
consumer, and cron writers while schema changes run. A digest rollback cannot
undo an incompatible database migration. The existing locks, production
approval, forward-only acknowledgement, and bounded cleanup are good work;
retain them.

Choose one Magento lifecycle authority, preferably the existing PHP package,
with Go orchestrating it. Build immutable application assets once, define which
runtime state is shared, and prove that serving containers receive the intended
assets. Start with a conservative maintenance/drain policy for incompatible
migrations. A zero-downtime promise is not required for alpha.
[Adobe's static-content deployment guidance](https://experienceleague.adobe.com/en/docs/commerce-operations/implementation-playbook/best-practices/development/static-content-deployment)
provides the relevant application context.

Owner: [Magento deployment safety](magento-deployment-safety/intent.md).

### F07 — Release blocker: workspace resolution masks distribution work

The SDK is a separate module and `go.work` joins it to the root module. The root
`go.mod` does not yet contain the consumer requirement needed outside that
workspace. The offline `GOWORK=off` check failed. The archived extraction report
acknowledges that publication step as deferred; it is not evidence of a working
external consumer today.

Generated CI in [`internal/cli/ci.go`](../internal/cli/ci.go) also uses a versioned
`go install`, which compiles the current broad toolchain in the customer job.
That undermines a simple binary-first experience and inherits source-module
resolution requirements.

Prove publication order and consumer resolution before any release tag. Test an
SDK consumer and provider from clean modules with `GOWORK=off`, then use verified
release binaries in the supported install and CI paths. Local test registries or
module proxies can prove the sequence before authorized public publication.
[Go's workspace tutorial](https://go.dev/doc/tutorial/workspaces) distinguishes
workspace development from published module requirements.

Owner: [verified distribution](verified-provider-distribution/intent.md).

### F08 — Release blocker: certification claims exceed individual evidence cells

The matrix and evidence must remain the authority for existing classification.
However, a certified cell is not certification of every production topology.
The narrow AWS predicate in
[`internal/cloud/aws/stack/certification_tier.go`](../internal/cloud/aws/stack/certification_tier.go)
includes a preview shape with search disabled. The GCP predicate in
[`internal/cloud/gcp/stack/certification_tier.go`](../internal/cloud/gcp/stack/certification_tier.go)
also does not itself express every dimension of the proved architecture.

There is meaningful broader evidence: the archived GCP search report records
reindexing, an HTTP product result, and successful querying after search recycle.
The AWS search report records infrastructure success but explicitly lacks that
full application-level sequence. Preserve this distinction. Do not claim there
is no functional GCP proof, and do not promote AWS from infrastructure proof.

The archived operator run exercised 13 verbs, but its deploy result included a
rejection guard rather than a successful complete store upgrade. Name exactly
what each test proves. Audit website and documentation language against those
limits; certification of a preview cell cannot support a blanket production
readiness claim.

Owner: [trust baseline](release-trust-baseline/intent.md) and
[reference-store acceptance](reference-store-acceptance/intent.md).

### F09 — Release blocker: the evidence integrity gate is red

`go run ./cmd/gencertdocs --check` rejected the first record in
`docs/evidence/runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl` because its
record digest did not match its contents. This prevents trusting the current
generated view without reconciliation.

Compare against the original artifact and establish why it changed. Restore the
valid original, or explicitly withdraw and replace evidence with provenance.
Do not merely recompute hashes to make an altered assertion look like its
original attestation. Regenerate derived files through supported generators.

Owner: [trust baseline](release-trust-baseline/intent.md).

### F10 — Release blocker for public promises: budgets are not enforced caps

Website copy in [`website/src/data/content.js`](../website/src/data/content.js)
uses budget-cap and spend-stopped language. The traced budget readers report
`Enforced: false`. GCP live pricing in
[`internal/cloud/gcp/cost/estimate.go`](../internal/cloud/gcp/cost/estimate.go)
is not wired to a complete live estimate. Validating a positive configured
budget is not spending enforcement.

Report estimates, missing prices, alerts, and limits honestly. A timeout in a CLI
process is not an autonomous environment-expiry service. Specify who runs the
expiry scheduler, how it authenticates, what happens if it fails, and which
backups, volumes, addresses, or other resources can continue to cost money after
teardown. Provider billing delay also prevents a simple exact hard-cap promise.

Owner: [trust baseline](release-trust-baseline/intent.md),
[reference onboarding](full-deployment-coverage/intent.md), and acceptance.

### F11 — Release blocker: first-install verification can be skipped

[`website/public/install.sh`](../website/public/install.sh) conditionally uses
Cosign and can skip signature verification when tooling or signature retrieval
is unavailable. A checksum fetched from the same untrusted distribution path
is not equivalent to verified publisher identity.

The updater's negative signature tests and signed release pipeline are useful,
but they do not prove the initial installer has the same policy. Define one
trust policy for installer, updater, core, and plugins. Verify identity and
content before execution; fail closed on invalid or missing required material.
Provide an explicit development path separately if needed. Include atomic
installation, cache integrity, compatibility, and recovery from interrupted
updates. Do not require the user to understand signing infrastructure.

Owner: [verified distribution](verified-provider-distribution/intent.md).

### F12 — Important: security documentation and runtime settings disagree

The architecture documentation describes read-only container filesystems; the
traced AWS container configuration uses a writable root filesystem and documents
a temporary reason pending writable storage design. Blindly enabling read-only
root would likely break Magento. Resolve the actual writable-path requirements,
then align configuration and claims.

A full security review remains outstanding. At minimum the release path needs
reviewed secret references, log redaction, least-privilege permissions, network
exposure, artifact trust, and recovery material. This audit does not assert that
those other areas are broken; it asserts that a broad secure-by-default claim
requires evidence for the supported recipe.

Owner: [trust baseline](release-trust-baseline/intent.md) for claims;
[Magento deployment safety](magento-deployment-safety/intent.md) and
[reference onboarding](full-deployment-coverage/intent.md) for the runtime.

### F13 — Important: the onboarding backlog confuses automation and ownership

The previous `full-deployment-coverage/audit.md` is useful inventory, but some
conclusions are wrong or outdated. A secret reference does not mean the user must
manually create the secret: MageLift can generate and store it. Domain ownership
and permission to change DNS do not mean every DNS operation must be manual.
Managed KMS configuration is not inherently incompatible with automation.
Conversely, billing activation, domain ownership, licenses, and some email
approvals cannot honestly be guaranteed to require no human action.

The existing AWS onboarding also exposes certificate, notification, and logging
prerequisites that need explicit treatment; some proposed follow-up slugs in
the old audit never became intents. Implementing three managed email vendors
first is a poor use of time. Validate a supported SMTP path for alpha. Put
provider-specific AWS gaps in the AWS parity intent. The partially implemented
SES files must be deliberately completed, isolated, or removed; do not let them
accidentally become the release contract.

Owner: [reference onboarding](full-deployment-coverage/intent.md),
[trust baseline](release-trust-baseline/intent.md), and
[AWS parity](aws-provider-parity/intent.md).

### F14 — Important: synthetic scenarios must not pretend to prove a store

The current mocked-scenario intent combines private-shop-shaped requirements,
multiple cloud recipes, migration compatibility, and mocked healthy output.
That scope makes it difficult to know what passing means. A mock can prove
command routing, contracts, deterministic failures, and configuration handling.
It cannot prove that Magento installed, served static assets, searched products,
or recovered its database.

Use original public synthetic fixtures, compatible with the clean-room policy.
Build three explicit layers: offline protocol/configuration tests, local Magento
application tests where useful, and bounded live acceptance of the published
recipe. Unsupported imported versions and services must be reported, not silently
substituted. Keep fixtures independent of private shop source and documents.

Owner: [mocked scenarios](mocked-shop-scenarios/intent.md) and acceptance.

### F15 — Important: the repository needs clearer ownership, not more repos

A monorepo fits coordinated changes to the SDK, providers, Magento package,
images, schemas, docs, and CLI. Independent releases are a build and dependency
contract; separate Git repositories are not a prerequisite. Splitting now would
multiply cross-repository changes before the boundaries are settled.

Keep `cmd/`, a lean core, public SDK, provider implementations, `build/`, images,
user skills, contributor tooling, docs, and website visibly separate. Use the
contract work to decide final provider module locations; avoid a directory-only
reorganization before those dependencies are understood. Use path ownership,
release manifests, component compatibility tests, and a short navigation map.

Stop treating every archived intent as a permanent roadmap row. Keep the active
roadmap small, link historical evidence where it matters, and distinguish
historical reports from current release gates. Do not create another parallel
backlog in agent notes or a second planning framework.

Owner: [provider contract](provider-plugin-contract/intent.md) for boundaries;
this planning rewrite for the active queue.

## Decisions: keep, change, defer

| Decision | Recommendation | Reason and tradeoff |
| --- | --- | --- |
| Monorepo | Keep | Coordinated development is valuable; independent artifact releases still need explicit contracts |
| Go CLI | Keep | Suitable distribution and orchestration model; avoid requiring Go in the supported customer install |
| Pulumi Automation API | Keep behind providers | Good infrastructure engine; its CLI/runtime dependencies must be provisioned and diagnosed automatically |
| YAML-only user path | Keep | Good fit for teams and CI; strong validation and migration tooling matter more than unrestricted configuration |
| Independent provider plugins | Finish before presenting the architecture as delivered | Higher initial contract/distribution work, lower long-term core coupling |
| Broad provider and service catalog | Freeze expansion | Each combination adds upgrade, recovery, compatibility, and support obligations |
| Custom Magento build package | Keep provisionally; consolidate lifecycle authority | Magento-specific deployment knowledge is valuable; duplicate orchestration is not |
| Cloud/quality patch integration | Keep supported integration, audit behavior and provenance | Do not copy upstream implementation or databases; integration is not proof of equivalent lifecycle behavior |
| Preview environments | Keep one bounded supported implementation | Requires autonomous expiry, data-sanitization policy, quotas, and honest residual-cost reporting |
| AI skills | Keep after the human workflow works | Skills should describe a trustworthy CLI, not teach workarounds for missing product behavior |
| Website and documentation in one repo | Keep | Simplifies matching docs to code; remove unsupported claims before marketing expands |
| Stable v1 immediately | Reject | Contract, installation, lifecycle, and evidence gaps make the freeze premature |

Pulumi Automation API still requires the Pulumi CLI/runtime; hiding that
complexity means managing it, not assuming it disappears. See the
[official Automation API guide](https://www.pulumi.com/docs/iac/guides/building-extending/automation-api/).

## Recommended first-release boundary

The proposed first public artifact is `v0.1.0-alpha.1`, not a stable v1 or an RC
that implies the current public contract is nearly frozen. Exact naming remains
subject to the release decision. No tag is authorized by this document.

Support one documented GCP Autopilot recipe, using the already demonstrated
functional search direction and explicitly selected compatible Magento/PHP/data
service versions. The recipe must serve a real shop with HTTPS, assets, search,
cron/consumers where required, outbound SMTP, and persistent media. Existing
integrations can remain available only with honest experimental labeling; they
must not drag embedded provider SDKs back into the lean shipped core.

Prove initial deployment, a subsequent application release, a failed release,
operation after credential expiry, backup and restore including Magento's
application encryption key and media, and autonomous preview expiry. Limit the
service combinations and scale expectations; do not promise arbitrary versions,
zero-downtime incompatible schema changes, cross-cloud disaster recovery, or a
hard spending cap.

Then validate with pilot teams and complete AWS ECS parity through the same
public protocol. Stable v1 should freeze only contracts supported by that
experience. If early interviews establish that the first pilot must be AWS,
change the provider order at intent acceptance while retaining the same gates;
do not attempt two simultaneous reference implementations to avoid deciding.

## Disposition of existing open work

| Existing intent | Disposition | Why |
| --- | --- | --- |
| `full-deployment-coverage` | Rewrite as draft reference onboarding | Three email vendors and an absolute zero-manual promise delay proof of the primary workflow |
| `mocked-shop-scenarios` | Rewrite as draft synthetic scenario foundation | Preserve useful test ideas; separate mocks from real application evidence and remove private-shop dependence |
| `v1-stable-cut` | Defer and rewrite for post-alpha stability | Its report is pending, the old ref is stale, and architecture is not ready to freeze |
| `eu-providers-experimental` | Defer with evidence retained | Useful experimental work, but neither provider is necessary to prove the first supported release |

None of these four has a completed passing verification record for its current
scope. None is archived as complete. Prior specifications, plans, and reports
are retained with explicit historical status so past effort is not erased or
mistaken for approval of the new scope. Existing archived directories remain
unchanged. New intents are drafts, with specification and implementation plans
still to be approved.

## Release sequence and completion evidence

[ROADMAP.md](ROADMAP.md) assigns every finding to a bounded intent. The sequence
first restores trust in gates, specifies the public boundary, adds focused test
fixtures, fixes Magento lifecycle semantics, proves an autonomous provider,
finishes onboarding, proves verified distribution, and runs the actual shipped
path before publishing an alpha. Tests for implementation belong with each
implementation; the final acceptance intent is not permission to defer testing.

Do not rerun every historic matrix merely because a roadmap changed. Reuse
valid evidence within its scope and rerun when code, packaging, or the public
execution path changes. Final release proof must reference the actual release
candidate and artifacts, not an unrelated old successful commit.

The stable decision also needs user evidence: can agency developers diagnose a
failed operation from CLI output, recover without private maintainer commands,
and understand the ongoing cost and responsibilities? Without that, a clean
architecture would still be an unvalidated product.

## Planning rewrite validation and deviations

This change edits planning documents only. Source code, generated schemas,
certification files, and existing archives are outside this rewrite. File-link,
intent-state, dependency, and diff checks are recorded by the editing session;
no product test result above is presented as a rerun after these prose edits.

The humanizer guidance was applied as an editorial review. The required
`remove-ai-marks` service health check failed to connect at its configured/default
local endpoint. Its skill prohibits fallback local cleaning, so that service
check remains unperformed. No claim of watermark removal is made. This does not
block writing the authorized audit, but must not be reported as a passed prose
pipeline.
