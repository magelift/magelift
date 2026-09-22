---
status: adopted
date: 2026-09-22
scope: product architecture, GCP alpha recovery, and multi-provider roadmap
---

# MageLift architecture and alpha recovery report

## Executive judgment

MageLift does not need a greenfield rewrite. It also does not need to become an
SST or OpenNext clone.

The best path to a first public alpha is to preserve the architecture that is
already expensive to rediscover and narrow the release path around it:

1. Keep the Go CLI, YAML contract, Pulumi Automation API, external artifact
   manifest, typed provider protocol, GCP provider, and existing AWS code.
2. Keep GKE Autopilot as the first GCP runtime. The current evidence does not
   justify replacing it with Cloud Run, Compute Engine, or Terraform.
3. Stop treating the current box 1.4 as one indivisible release gate. It mixes
   product acceptance, distribution, resolver precedence, media security, and
   generated CI. Split those concerns and define a smaller alpha gate.
4. Diagnose the rc.22 Magento HTTP 500 on the exact deployed artifact before
   changing topology. An HTTP 500 is an application integration failure, not
   evidence that the provider architecture is wrong.
5. Reuse the existing artifact contracts. Do not introduce a new
   magelift.bundle.json or a second application manifest.
6. Keep providers as separate signed binaries. Do not bundle them into the CLI.
   Make first-party provider installation automatic and invisible on the normal
   deploy path, while retaining the existing lockfile, cache, verification, and
   explicit offline-install command.
7. Keep alpha versions lockstep as already decided, but keep the physical and
   protocol boundaries that allow GCP and AWS to release independently after
   alpha.
8. Keep account bootstrap explicit. It is a one-time administrative trust
   boundary, not an implementation detail that deploy should silently cross.
9. Publish tested builder and runtime base images. A user should build the shop
   image, not rebuild MageLift's framework images from MageLift source.
10. Use commit-addressed canaries for product debugging. Cut a release
    candidate only from a commit that has already completed the reduced GCP
    product loop.

This is a sequencing correction and a small number of boundary completions,
not an architectural restart.

## The central diagnosis

The project is not blocked because it lacks a credible multi-provider
architecture. It is blocked because the first release candidate has become the
integration environment for too many independently failing systems.

The superseded alpha-review-corrections box 1.4 plan, retained in Git history,
required a fresh VM to prove, in one run:

- published CLI installation;
- provider download and verification;
- base-image and shop-image construction;
- GCP bootstrap and infrastructure creation;
- Magento deployment and serving health;
- extensive media behavior and access-control negatives;
- project-pinned provider precedence;
- generated CI; and
- complete cleanup.

That is not one acceptance test. It is a release train containing at least five
test products. Every ordinary Magento defect forces another provider release,
module-proxy wait, signed candidate, clean VM, cloud deployment, and cleanup
cycle.

The result is a feedback loop measured in release candidates instead of a
feedback loop measured in commits.

The evidence also shows that the core GCP topology is substantially farther
along than the current failure suggests. The
[capability matrix](../docs/capability-matrix.md) records live GKE Autopilot
Magento, deploy-candidate, search, runtime operations, and cleanup evidence.
The rc.22 run passed installation, provider resolution, bootstrap, image
construction, infrastructure creation, and workload startup before Magento
returned HTTP 500. The immediate unknown is therefore inside the serving path
or its runtime inputs. It is not whether MageLift can create GCP resources.

## What SST and OpenNext do and do not imply

SST and OpenNext are useful examples because they demonstrate a good user
experience over complicated cloud machinery:

- the application developer works at a framework level;
- build output is a stable handoff to infrastructure;
- cloud details are hidden until the user needs an escape hatch; and
- one tool composes build, provisioning, and deployment without pretending
  those phases are the same thing.

They are not the target architecture for MageLift.

[OpenNext's architecture](https://opennext.js.org/aws/inner_workings/architecture)
turns Next.js output into a deployable package and intentionally does not
create infrastructure.
[SST](https://sst.dev/docs) supplies high-level infrastructure components and
uses Pulumi underneath. Magento has a materially different lifecycle:
Composer authentication, PHP extension compatibility, dependency injection
compilation, static content, database migrations, runtime env.php, cron,
queue consumers, media, search, and rollback constraints.

The lesson to retain is the separation of concerns and the paved user path.
The implementation should remain Magento-specific.

## Sources and confidence

### Repository authority

The recommendations are grounded first in current code and accepted project
decisions:

- [architecture charter](../docs/architecture.md);
- [ADR 0001: YAML-only CLI](../docs/adr/0001-yaml-only-cli.md);
- [ADR 0003: portable contracts versus topology](../docs/adr/0003-portable-contracts-vs-topology.md);
- [ADR 0005: external artifact manifest](../docs/adr/0005-external-artifact-manifest.md);
- [ADR 0013: provider plugin contract](../docs/adr/0013-provider-plugin-contract.md);
- [ADR 0014: distribution trust](../docs/adr/0014-distribution-trust.md);
- [current roadmap](ROADMAP.md);
- the superseded alpha-review-corrections intent and plan retained in Git
  history; and
- [current capability claims](../docs/capability-matrix.md).

The old Codex CLI and Cursor Agent conversations were reviewed as historical
context. They explain why the project accumulated its current safeguards and
why several release gates were joined together. They are not treated as
authority where the current code or accepted ADRs differ.

### Private production field reference

At the user's request, the sibling private Magento production repository was
inspected. No private source, identifier, documentation, secret, or
organization name is reproduced here. Its useful findings were already
distilled into clean-room repository notes:

- [Magento image builds must not see a runtime env.php](../.agents/knowledge/lessons/Magento%20image%20builds%20must%20not%20see%20a%20runtime%20env.php.md);
- [Magento deploy writes env.php before traffic moves](../.agents/knowledge/lessons/Magento%20deploy%20writes%20env.php%20before%20traffic%20moves.md); and
- [AWS Magento wiring follows a prior shop](../.agents/knowledge/lessons/AWS%20Magento%20wiring%20follows%20a%20prior%20shop.md).

That project is evidence for lifecycle shape, not a source to copy and not
MageLift certification. It supports four conclusions:

1. Build and runtime images have different responsibilities.
2. The build can compile code and static content without database access or a
   runtime env.php.
3. A one-shot deploy task must materialize runtime configuration and complete
   database work before serving workloads roll.
4. The identity used by CI to deploy must not be able to rewrite the trust
   policy through which it obtained access.

### External primary sources

The report uses current primary documentation where behavior can change:

- [Pulumi Automation API](https://www.pulumi.com/docs/iac/concepts/automation-api/)
  for embedding preview, update, and destroy inside a higher-level CLI;
- [OpenTofu provider installation](https://opentofu.org/docs/cli/plugins/) and
  [dependency locks](https://opentofu.org/docs/language/files/dependency-lock/)
  as a proven model for automatic, cached, version-pinned provider delivery;
- [HashiCorp go-plugin internals](https://github.com/hashicorp/go-plugin/blob/main/docs/internals.md)
  for the subprocess host/server model;
- [Cloud Native Buildpacks lifecycle](https://buildpacks.io/docs/for-platform-operators/concepts/lifecycle/)
  as a useful precedent for keeping build phases and OCI output explicit;
- [GKE Autopilot overview](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/autopilot-overview)
  and [GKE mode guidance](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/choose-cluster-mode)
  for the current GCP runtime decision;
- [Cloud Run resource model](https://docs.cloud.google.com/run/docs/overview/what-is-cloud-run)
  for the comparison with services, jobs, worker pools, and instances;
- [Magento deployment overview](https://experienceleague.adobe.com/en/docs/commerce-operations/configuration-guide/deployment/overview)
  and [technical details](https://experienceleague.adobe.com/en/docs/commerce-operations/configuration-guide/deployment/technical-details)
  for build/deploy configuration separation; and
- [SLSA provenance](https://slsa.dev/spec/v1.2/provenance) for binding an
  artifact to how and from what it was built.

## Dated current-state snapshot

The following facts were observed on 2026-09-22:

- v0.1.0-alpha.1-rc.22 is a published prerelease.
- The clean-machine proof installed the release and progressed through provider
  resolution, bootstrap, base images, shop image, infrastructure, and runtime
  startup.
- The serving-path gate failed because Magento returned HTTP 500.
- No deploy.done marker was created.
- Earlier runs encountered IAM and service-networking failures, but the rc.22
  run progressed beyond those boundaries.
- Multiple old provider subprocesses remained on the proof VM.
- The proof VM was still running when inspected.
- Plan boxes 1.4 and 5.1 remained open.
- The roadmap still described box 1.4 as waiting on a candidate after rc.15,
  although rc.22 existed.
- The rc.22 GCP provider binaries were approximately 199–213 MB per platform.
- The rc.22 CLI archives were approximately 57–65 MB per platform.

The HTTP 500 has not been assigned a root cause in this report. Logs from the
web, PHP-FPM, deploy job, and Magento exception paths are required before
classifying it as image, runtime configuration, database, cache, search,
static-content, base-URL, or ingress behavior.

## Facts, inferences, and decisions

| Kind | Statement |
| --- | --- |
| Fact | GCP Autopilot has prior live Magento evidence in this repository. |
| Fact | rc.22 reached the application serving path and received HTTP 500. |
| Fact | MageLift already creates a canonical external artifact manifest. |
| Fact | MageLift already has a signed provider lockfile, cache, downloader, verifier, resolver, and subprocess protocol. |
| Fact | The GCP provider is several times larger than the compressed CLI archive. |
| Fact | The accepted provider ADR requires separate provider modules and lockstep alpha tags, then permits independent cadence later. |
| Inference | The fastest alpha is a corrected vertical slice through existing boundaries, not a new runtime or IaC engine. |
| Inference | Automatic provider acquisition gives the desired user experience without undoing the provider architecture. |
| Inference | Box 1.4's breadth, rather than a single impossible capability, is the main cause of slow closure. |
| Decision proposed | Keep GKE Autopilot and Pulumi for the alpha. |
| Decision proposed | Split box 1.4 and make only the product-critical subset release-blocking. |
| Decision proposed | Retain separate providers and automatically install the pinned first-party provider. |
| Decision proposed | Reuse existing artifact types; add no new manifest. |

## Product definition

MageLift should promise:

> A Magento engineering team can describe an environment in magelift.yaml and
> use one CLI to build, deploy, inspect, and remove a supported Magento
> topology in its own cloud account, without operating Pulumi or Kubernetes
> directly.

It should not promise to:

- be a hosted PaaS;
- eliminate the need for a cloud account owner;
- make all clouds topologically identical;
- support arbitrary service/version combinations;
- make database schema rollback safe;
- hide cost, DNS ownership, incident response, or data-governance decisions;
  or
- certify a capability that only passed a mock or an unrelated provider cell.

The first public alpha should have one paved road:

> Magento Open Source 2.4.9, the repository's evidenced PHP and service
> versions, preview class, GCP GKE Autopilot, Cloud SQL, Memorystore, GCS media,
> database queue, and the one-replica GKE OpenSearch shape already exercised by
> the repository.

The alpha can expose advanced fields already implemented, but they must not
expand its support claim.

## Recommended system architecture

### Four layers with one owner each

~~~text
Magento repository + magelift.yaml
                |
                v
  +-----------------------------+
  | MageLift core               |
  | YAML, compatibility, UX,    |
  | orchestration, trust, locks |
  +--------------+--------------+
                 |
       build contract and
       typed provider operations
                 |
        +--------+---------+
        |                  |
        v                  v
  +-----------+      +-----------+
  | GCP       |      | AWS       |
  | provider  |      | provider  |
  | owns GCP  |      | owns AWS  |
  | topology  |      | topology  |
  +-----+-----+      +-----+-----+
        |                  |
        v                  v
  Pulumi Automation   Pulumi Automation
  in provider process in provider process
~~~

There are four architectural layers:

1. **User contract:** magelift.yaml, presets, compatibility rules, and CLI
   output.
2. **Application artifact:** the immutable OCI image, canonical artifact
   manifest, provenance, and signature identity.
3. **Provider implementation:** provider-specific planning, Pulumi program,
   stack execution, runtime operations, and topology.
4. **Evidence:** contract tests, live cells, capability claims, and release
   qualification.

The current design already approximates this split. The priority is to make
the supported path traverse it cleanly, not replace it.

### Core ownership

The core should own:

- YAML loading, overlays, environment selection, and common validation;
- Magento/service compatibility policy;
- build orchestration and artifact identity;
- provider discovery, download, verification, negotiation, and cache;
- deployment ordering and safety policy;
- environment locks and release journal;
- consistent command UX and typed error presentation;
- evidence identities; and
- dispatch to the selected provider.

The core should not import a provider's cloud SDK or Pulumi provider package.
It should not construct provider resource graphs or normalize unlike cloud
resources into a universal infrastructure model.

### Provider ownership

Each provider should own:

- validation of its provider-specific target block;
- network, identity, database, cache, search, queue, storage, runtime, edge,
  and observability topology;
- its Pulumi program and Automation API workspace;
- provider capability admission;
- stack lifecycle;
- status, logs, exec, backup, restore, credential refresh, and teardown;
- residual-resource discovery; and
- provider-specific failure classification.

The accepted
[provider plugin contract](../docs/adr/0013-provider-plugin-contract.md)
already states this. It should remain the long-term direction.

### No shared cloud graph

Application lifecycle and capability vocabulary should be portable. Cloud
resource graphs should not be.

AWS and GCP can share:

- application metadata;
- build and deploy phase semantics;
- artifact identity;
- capability names;
- health expectations;
- error categories;
- operation request/response types; and
- acceptance scenarios.

They should not share a Pulumi component containing provider switches for
networks, databases, search, queues, or runtimes. The current ADR 0003 rule is
correct and prevents a lowest-common-denominator topology.

## Artifact architecture: reuse what exists

The previous draft proposed a new magelift.bundle.json. That recommendation is
withdrawn.

MageLift already has three complementary artifact views:

1. [ArtifactManifest](../build/src/Artifact/ArtifactManifest.php) records source
   revision, image digest, MageLift/build versions, Magento and PHP versions,
   extensions, modules, build inputs, checksums, static-content matrix,
   required runtime capabilities, and compatibility status.
2. [ImmutableArtifactContract](../sdk/artifact.go) binds the image digest,
   manifest digest, input fingerprint, provenance reference, and signature
   reference for reuse and certification.
3. [BuildArtifact](../sdk/types.go) is the deliberately small deploy transport
   containing image and manifest digests.

The [build pipeline](../internal/build/pipeline/pipeline.go) already finalizes
the manifest after BuildKit returns the OCI digest, which avoids a digest
cycle and matches ADR 0005.

The correct work is consolidation, not invention:

- document these three views as manifest content, immutable evidence identity,
  and deploy transport;
- verify that every deploy path carries both image and manifest digests;
- validate required runtime capabilities before provider mutation;
- keep secrets and runtime env.php out of all three;
- attach or reference provenance using the existing immutable contract; and
- add fields only when two provider consumers demonstrate a need.

Do not put GCP projects, AWS account IDs, subnets, Cloud SQL tiers, ECS task
shapes, or other topology into the artifact manifest.

### Build and deploy remain separate internally

The system must retain distinct stages:

~~~text
source
  -> prepare and compile without runtime secrets or database
  -> immutable OCI image plus external manifest
  -> sign and record provenance
  -> provision or update provider infrastructure
  -> one-shot deploy job creates runtime config and migrates
  -> roll web, cron, and consumers
  -> serving-path health
~~~

The CLI may compose these stages for convenience. It must not collapse their
contracts.

For alpha:

- magelift build --push remains a valid explicit CI primitive;
- magelift deploy --digest remains the deterministic deploy primitive;
- the interactive deploy command may later build when no digest is supplied,
  but that convenience must call the same build pipeline;
- no second image builder or provider-specific application image is allowed.

## Provider distribution: automatic, separate, pinned

Bundling first-party provider binaries into the CLI archive would simplify one
installation step but harm the architecture that AWS needs:

- the currently published raw GCP provider asset is roughly 199–213 MB while
  the compressed CLI archive is roughly 57–65 MB, so embedding providers would
  materially enlarge every installation even before AWS is added;
- adding AWS would make every user download every provider;
- a GCP provider security fix would require republishing the full CLI archive;
- bundling weakens the value of the existing cache and project lock; and
- the project has already implemented the hard verification machinery.

The better user experience is transparent installation:

~~~text
magelift deploy
  -> read provider selected by magelift.yaml
  -> read project lock when present, otherwise release lock
  -> use verified cache hit when available
  -> otherwise download the exact signed provider
  -> verify publisher, signature bundle, and digest
  -> atomically install in the cache
  -> negotiate protocol and required operations
  -> execute
~~~

This follows the useful part of the OpenTofu provider model: providers are
separate and independently cacheable, while initialization normally downloads
the pinned versions without manual assembly.

### Required distribution behavior

- Normal online users do not need to run magelift providers install.
- Mutation commands may automatically install a missing first-party provider
  after clearly reporting what is being downloaded and verified.
- An existing but invalid lockfile, signature, digest, or cached binary fails
  closed. It is never silently replaced.
- A project lock overrides the release default and remains reviewable in
  version control.
- The explicit providers install command remains for air-gapped preparation,
  cache prewarming, repair, and diagnostics.
- Provider binaries are cached by provider, version, operating system, and
  architecture.
- One CLI invocation owns and closes the provider process it starts.
- Tests must assert that success, failure, and cancellation leave no child
  provider process.

The last requirement is not theoretical: rc.22 left multiple provider
subprocesses resident on the proof VM.

### Versioning

Keep the existing alpha decision:

- core, SDK, and first-party providers publish lockstep alpha tags;
- the release lock pins exact tested provider artifacts;
- the binaries remain separate;
- protocol negotiation remains explicit; and
- independent provider cadence activates only after alpha when compatibility
  ranges and two supported providers have real maintenance history.

Lockstep release metadata is not the same as a fat binary. It provides one
tested distribution without coupling future updates.

## Bootstrap is an explicit security boundary

The earlier draft suggested hiding bootstrap inside deploy. That is wrong.

The production field reference and MageLift's own ADRs support a two-identity
model:

1. A human or account-bootstrap identity creates state storage, workload
   federation, service accounts or roles, and the bounded permissions used by
   CI.
2. The deploy identity uses those permissions but cannot rewrite the trust
   relationship through which it gained access.

Therefore the paved path remains:

~~~sh
magelift doctor
magelift bootstrap --env preview
magelift deploy --env preview --yes
magelift health --mode runtime
~~~

Bootstrap should be idempotent and needed once per account/project boundary.
Deploy should detect a missing bootstrap and print the exact next MageLift
command. It should not silently escalate an ordinary deploy into identity and
trust creation.

This retains the UX principle in
[Magelift commands hide cloud-devops from merchants](../.agents/knowledge/lessons/Magelift%20commands%20hide%20cloud-devops%20from%20merchants.md)
without erasing separation of duties.

## Base images and shop images

The primary onboarding path should not clone MageLift and build MageLift's
builder and runtime base images.

Each MageLift release should publish a small tested catalog:

- builder image digest;
- runtime image digest;
- supported architecture;
- Magento/PHP/Composer compatibility tuple;
- build package version;
- SBOM and provenance references; and
- deprecation or security status.

The shop build consumes those digests and produces the existing external
artifact manifest. A project may override base images only through an explicit
advanced path that invalidates or narrows certification claims.

This is analogous to Cloud Native Buildpacks only at the lifecycle level:
tested build inputs produce an OCI application image. MageLift should not adopt
the Buildpacks API or rewrite its Magento-specific builder merely for
similarity.

## Magento deployment lifecycle

The private production reference, MageLift knowledge, and public Magento
deployment guidance agree on the important sequence:

1. Composer dependencies, DI compile, and static content are built without the
   runtime database, secrets, or env.php.
2. Infrastructure and secrets references exist before application deployment.
3. A one-shot candidate deploy runs with the new image.
4. It materializes runtime configuration, proves the database connection,
   imports configuration where applicable, and runs setup:upgrade.
5. Serving, cron, and consumer workloads update only if the deploy task exits
   successfully.
6. Public serving health verifies application behavior, not merely pod or load
   balancer health.

The existing [deploy orchestrator](../internal/deploy/orchestrator.go) already
models candidate registration, migrations, service update, stabilization, and
health. That should remain the one lifecycle authority. Provider
implementations translate the phases to GKE Jobs, Kubernetes workloads, ECS
tasks, or services; they must not invent a separate ordering.

## GCP runtime decision

### Recommendation

Keep GKE Autopilot for the first alpha.

This is not a claim that GKE is universally superior. It is the best choice
for this project now because:

- it already has live MageLift evidence;
- the GCP provider and deploy phases are implemented;
- it accommodates web, deploy jobs, cron, queue consumers, and the current
  search workload in one runtime model;
- Autopilot manages nodes, scaling, upgrades, and baseline hardening;
- Kubernetes concepts map reasonably to later EKS or other Kubernetes
  providers without sharing cloud topology; and
- changing runtime would invalidate more evidence than it would simplify.

### Alternatives

| Runtime | Advantages | Cost now | Decision |
| --- | --- | --- | --- |
| GKE Autopilot | Existing code and evidence; web, jobs, cron, consumers, sidecars, stateful workload support; managed nodes | Kubernetes complexity; OpenSearch HA constraints; cluster startup time | Keep for alpha |
| Cloud Run | Managed services, jobs, worker pools, and instances; no cluster | Rebuild ingress, rollout, networking, ops, search, and provider evidence; worker pools require separate scaling policy | Revisit as a later GCP runtime |
| Compute Engine managed instance groups | Familiar VM model; easy to mirror a traditional Magento host | More patching, scaling, rollout, and process supervision; highly GCP-specific | Reject for the alpha |
| GKE Standard | More control, including OpenSearch HA sysctls | Node ownership and larger operational surface | Keep experimental |

Cloud Run is more capable now than older comparisons imply: current
documentation includes services, jobs, worker pools, and long-lived instances.
It may become a good low-operations GCP runtime. It is still a new provider
runtime with new networking, scaling, search, deployment, logs, exec, and
recovery behavior. It cannot shorten the current route to alpha.

### Alpha GCP topology

The first alpha should pin one recipe rather than generalize further:

| Magento concern | Alpha choice |
| --- | --- |
| Runtime | GKE Autopilot |
| Database | Cloud SQL MySQL, evidenced Magento-compatible version |
| Cache/session | Memorystore version already exercised by the current cell |
| Search | One-replica OpenSearch workload already exercised on Autopilot |
| Queue | Magento database queue |
| Media | Private GCS bucket through the implemented remote-storage contract |
| Secrets | Secret Manager references only |
| Deployment | One GKE candidate Job before workload rollout |
| Edge | Existing GCP load-balancing/TLS path required for the alpha URL |
| Observability | Native logs sufficient for alpha diagnosis; no third-party integration gate |

High availability, GKE Standard, RabbitMQ, external observability, alternate
edges, and broader version combinations stay available only under their
current honest status. They do not block the first alpha.

## Why Terraform is not the corrective action

The sibling production project shows that a focused Terraform and Docker stack
can deploy Magento successfully. It does not show that Terraform would solve
MageLift's current release problem.

That private stack has:

- one organization and a known account layout;
- one application;
- one cloud topology;
- predetermined CI and IAM;
- fewer compatibility and extension promises; and
- no public provider distribution contract.

MageLift has to solve a different problem: safe user-owned accounts,
configuration compatibility, provider selection, distribution, day-two
operations, evidence, and future AWS support.

[Pulumi Automation API](https://www.pulumi.com/docs/iac/concepts/automation-api/)
is explicitly intended for custom CLIs, multi-stage deployments, application
deployments with migrations, and programmatic preview/update/destroy. The
recent failures do not demonstrate systemic Pulumi state corruption or an
inability to express the topology.

Reconsider the IaC engine only if evidence shows a repeatable engine-specific
failure that cannot be fixed behind the provider boundary, such as:

- unrecoverable state corruption;
- non-idempotent updates attributable to the engine rather than provider APIs;
- inability to import or destroy required resources;
- unacceptable distribution/runtime overhead after measurement; or
- a provider implementation that is materially simpler and safer as a
  standalone replacement proven by a bounded prototype.

If that threshold is met later, only that provider should be prototyped with a
different engine. The core protocol should not care whether a provider uses
Pulumi, Terraform, direct SDKs, or another implementation.

## AWS path without reimplementation

AWS should not be rebuilt after GCP. It should consume the same contracts.

### During the GCP alpha

- Keep the AWS ECS implementation compiling.
- Keep shared SDK and lifecycle contract tests running against AWS.
- Do not add AWS feature breadth.
- Do not require an AWS live cell for GCP-only changes.
- Preserve existing AWS evidence and capability claims; do not imply GCP
  evidence certifies AWS.
- Any change to artifact identity, deploy phases, provider negotiation, or
  common configuration must update the AWS adapter or explicitly record the
  deferred migration.

### Immediately after the GCP alpha

1. Move AWS into an autonomous providers/aws module using the GCP provider as
   the protocol reference, not as a topology template.
2. Remove the deferred GCP imports of root-internal implementation helpers
   before copying the boundary.
3. Make AWS own its Pulumi execution and all required day-two operations.
4. Make the release catalog include a signed AWS provider artifact.
5. Run the existing ECS Fargate evidence scenarios through the autonomous
   provider.
6. Keep GCP and AWS alpha tags lockstep until both have passed the same
   provider-contract and distribution gates.

What AWS reuses:

- YAML envelope;
- application model;
- artifact manifest and immutable identity;
- build pipeline;
- deploy state machine;
- provider download/verification/cache;
- typed operations and errors;
- release journal;
- health semantics; and
- acceptance scenario definitions.

What AWS owns:

- VPC and IAM;
- ECS services and tasks;
- database, cache, search, queue, storage, edge, and observability choices;
- AWS deployment task mechanics;
- AWS logs, exec, backup, restore, refresh, and cleanup; and
- AWS cost and failure behavior.

This is the correct meaning of multi-provider: reuse product contracts and
workflow, not cloud graphs.

## User experience for the alpha

The alpha should have one documented online path:

~~~sh
magelift init --provider gcp
magelift doctor
magelift bootstrap --env preview
magelift build --push --image REGION-docker.pkg.dev/PROJECT/shop/shop:preview
magelift deploy --env preview --digest REGION-docker.pkg.dev/PROJECT/shop/shop@sha256:DIGEST --yes
magelift health --mode runtime
magelift destroy --env preview --yes
~~~

Doctor should report the provider that will be selected without mutating the
machine. The first mutating command should download it automatically when it is
not already present. The user should not:

- clone MageLift itself;
- compile Go code;
- build MageLift base images;
- invoke Pulumi;
- invoke kubectl;
- install Cosign;
- manually download the GCP provider; or
- supply plaintext secret values.

After the first alpha, a composed magelift deploy path may build when no digest
is present. That is a UX improvement over the same build and deploy contracts,
not a new architecture.

## Replace box 1.4 with independent gates

The current box should be superseded, not weakened silently.

### Gate A: release installation

Account-free and release-blocking:

- install the CLI archive on a clean supported machine;
- verify version and self-check;
- resolve the release's GCP provider;
- automatically download and verify it;
- negotiate the protocol and required operation set;
- prove tampered metadata and binary rejection; and
- prove the provider process exits on success, failure, and cancellation.

### Gate B: artifact build

Account-free except for registry push, release-blocking:

- consume published builder/runtime digests;
- build the reference Magento shop with no runtime env.php or database;
- emit the existing canonical artifact manifest;
- verify image, manifest, source, input, provenance, and signature identities;
- scan the produced image; and
- prove no secret value appears in the image, manifest, logs, or evidence.

### Gate C: GCP product vertical slice

Live GCP and release-blocking:

- explicit bootstrap succeeds;
- first preview deploy succeeds;
- deploy job exits successfully before workload rollout;
- public HTTPS and Magento application health return success;
- static assets load;
- a known product is reachable and search works;
- cron and the selected database queue path work;
- media upload, fresh-pod retrieval, and access-control negatives pass;
- a second deployment is idempotent; and
- destroy plus residual inventory report zero owned resources.

### Gate D: project provider pin

Account-free or minimal cloud smoke, release-blocking only for the provider
distribution feature:

- project lock overrides the release default;
- the selected and executed provider identities are recorded;
- incompatible or tampered project locks fail closed; and
- a conflicting beside-CLI binary cannot override the project decision.

This gate should not rebuild and redeploy Magento unless the provider execution
semantics changed.

### Gate E: generated CI

Separate feature gate:

- generated workflow uses released CLI and provider artifacts;
- identity federation succeeds;
- the workflow executes a bounded preview or deploy;
- permissions remain scoped; and
- cleanup completes.

Generated CI may be marked experimental in the first alpha if hosted execution
cannot be proved. It must not hold the core laptop deployment hostage.

### Gate F: extended operations and certification

Post-alpha or capability-specific:

- backup and restore;
- credential expiry and refresh;
- failed release and forward-only rollback handling;
- autonomous preview expiry;
- external edge and observability;
- HA and disruption scenarios;
- evidence sealing and reuse-boundary identity; and
- broader service/version combinations.

These gates remain valuable. Moving them after the first product loop is not
deleting quality; it is aligning claims with the maturity of an alpha.

## Development and release workflow

### Product loop

Use an unpublished commit-addressed canary:

~~~text
commit SHA
  -> build CLI and GCP provider
  -> publish SHA-addressed base images when changed
  -> build reference shop image
  -> deploy to the dedicated acceptance project
  -> inspect application logs and behavior
  -> fix
  -> destroy
~~~

The canary must use the same code paths, signatures or test trust root, and
provider process as a release. It does not need public semantic-version tags or
module-proxy publication for every defect.

### Release loop

Only after the product loop is green:

~~~text
known-green commit
  -> publish SDK/provider module tags in required order
  -> publish signed provider artifacts and release lock
  -> publish signed CLI candidate
  -> run Gates A, B, and a bounded confirmation of C
  -> publish prerelease
~~~

The release candidate proves distribution equivalence. It is not the place to
discover whether Magento can serve its home page.

### Evidence reuse

Evidence can be reused only when the identities relevant to that gate remain
unchanged:

- distribution evidence keys on CLI archive, provider binary, lock, signature,
  OS, and architecture;
- artifact evidence keys on source, build inputs, builder/runtime digests,
  image digest, manifest digest, provenance, and signature;
- GCP topology evidence keys on provider, configuration, state/schema
  fingerprint, and deployed artifact;
- Magento behavior evidence keys on application artifact and runtime contract;
  and
- cleanup evidence keys on ownership ledger, stack, provider, and inventory
  result.

The existing lesson
[Acceptance JSONL PASS needs reuse-boundary identities](../.agents/knowledge/lessons/Acceptance%20JSONL%20PASS%20needs%20reuse-boundary%20identities.md)
should remain the policy.

## Immediate handling of rc.22

The next action should be diagnostic, not architectural:

1. Preserve the rc.22 command log and exact artifact identities.
2. Collect logs through MageLift from the deploy job, web server, PHP-FPM,
   Magento exception/system logs, and ingress health request.
3. Compare the effective runtime bindings with the artifact's required
   capabilities.
4. Determine whether the HTTP 500 reproduces by calling the service directly
   inside the cluster and through the public load balancer.
5. Classify the failure as image, configuration, migration, service
   dependency, or ingress only after that evidence.
6. Add the smallest local or provider-level regression test that reproduces
   the cause.
7. Fix provider-client ownership so every CLI path waits for or terminates its
   child process.
8. Destroy the retained proof resources after evidence capture and verify
   residual inventory.
9. Re-run as a commit canary.
10. Do not cut rc.23 until the canary passes the reduced product vertical
    slice.

If the exact rc.22 application artifact can be repaired only by changing
runtime configuration, record whether the repair changed the provider plan,
deployment state, or only operator data. Do not retroactively call the failed
run a pass.

## Roadmap reset

### Phase 0: close the current experiment

Deliverables:

- rc.22 failure report with evidence-backed root cause;
- child-process lifecycle regression test;
- destroyed VM and GCP preview resources;
- zero-owned-resource inventory; and
- current roadmap status corrected.

Exit criterion: no retained resource or unexplained application failure from
rc.22.

### Phase 1: supersede the monolithic gate

Create one accepted intent that:

- replaces box 1.4 with Gates A through F;
- declares A, B, and C as first-alpha blockers;
- decides whether D is a blocker or a separately supported distribution
  feature;
- marks E experimental unless live CI evidence exists;
- moves F to capability-specific follow-ups;
- preserves all existing findings and evidence; and
- narrows the first-alpha capability claim.

Exit criterion: one approved plan with no release step requiring an unrelated
live redeploy.

### Phase 2: finish the paved distribution path

Implement only missing connections:

- automatic install of the release-pinned first-party provider;
- explicit project-lock precedence;
- provider subprocess cleanup;
- published builder and runtime image catalog;
- onboarding that never builds MageLift base images; and
- errors that name the next MageLift command.

Do not rewrite providerhost. The current
[lock](../internal/providerhost/lock.go),
[download](../internal/providerhost/download.go),
[fetch](../internal/providerhost/fetch.go), and
[resolver](../internal/providerhost/resolve.go) packages already contain most
of the required behavior.

Exit criterion: Gate A and Gate B pass on a clean machine.

### Phase 3: make the GCP slice repeatable

Use the pinned alpha topology and run Gate C from a commit canary until it
passes twice from clean state.

Two passes are recommended because one success after 22 release candidates
does not establish repeatability. The second pass may reuse immutable base
images but must use fresh stack state and a fresh owned environment.

Exit criterion: two green create/deploy/behavior/destroy loops with no owned
residuals.

### Phase 4: publish the first alpha

- cut a candidate from the proven commit;
- publish module tags and signed artifacts in the existing required order;
- run Gate A against public assets;
- compare public artifact identities with the canary;
- run one bounded GCP confirmation;
- publish the prerelease; and
- update the capability matrix and release notes to the exact proven scope.

Exit criterion: a public alpha whose documented primary path has been executed
literally from its published artifacts.

### Phase 5: autonomous AWS parity

- complete provider boundary decoupling;
- extract AWS to providers/aws;
- reuse the artifact, lifecycle, distribution, and evidence contracts;
- preserve AWS-owned topology;
- run the ECS Fargate product gate;
- add AWS to the release lock; and
- keep capability differences explicit.

Exit criterion: adding AWS requires provider work and evidence, not a second
application build/deploy framework.

### Phase 6: product depth

Prioritize from user demand:

- generated CI;
- backup and restore;
- release promotion and constrained rollback;
- production/HA GCP profile;
- Cloud Run as an optional GCP runtime prototype;
- edge and observability integrations;
- additional providers; and
- eventual independent provider release cadence.

## Capability disposition

| Area | Action before GCP alpha | Reason |
| --- | --- | --- |
| Go CLI and YAML | Keep | Correct product surface |
| Pulumi Automation API | Keep | Appropriate embedded engine; not implicated by current failure |
| Existing artifact manifest | Keep and document | Already provides the needed application boundary |
| ImmutableArtifactContract | Keep | Correct evidence/reuse identity |
| GCP provider subprocess | Keep and fix lifecycle | Correct future-provider boundary; leaked processes are an implementation defect |
| Signed provider lock/cache | Keep and auto-use | Avoids manual install without a fat binary |
| Provider bundling | Reject | Large artifacts and poor AWS scaling |
| GKE Autopilot | Keep | Fastest evidenced path |
| Cloud Run runtime | Defer prototype | Promising, but a separate runtime and evidence program |
| Terraform rewrite | Reject now | No engine-specific evidence justifies it |
| Explicit bootstrap | Keep | Administrative trust boundary |
| MageLift base-image source builds in onboarding | Remove | Release should provide tested digests |
| AWS ECS code | Keep warm | Second provider and contract consumer |
| AWS live gate on GCP changes | Do not require | Unrelated cost and latency |
| GKE Standard and EKS | Freeze experimental | No alpha value |
| OVH and Scaleway | Freeze | No alpha value |
| Generated CI | Separate gate | Valuable but not the core laptop path |
| Certification/evidence engine | Keep off critical product loop | Protects claims without slowing every debug cycle |
| External edge/observability | Freeze | Origin must be reliable first |
| Local Compose | Keep | Fast application lifecycle feedback, not cloud certification |

## Alpha exit criteria

The first alpha is ready when all are true:

- a clean supported machine installs the public CLI;
- the selected signed GCP provider is acquired automatically and verified;
- no provider subprocess survives the CLI invocation;
- the reference shop builds from published base-image digests;
- the build occurs without a runtime env.php, database, or plaintext secret;
- the existing artifact manifest and immutable identity validate;
- explicit bootstrap creates the bounded GCP prerequisites;
- the pinned GKE Autopilot preview deploy completes;
- the deploy job succeeds before serving workloads roll;
- the public URL returns healthy Magento content and static assets;
- product/search, cron/queue, and basic media behavior pass;
- unauthenticated media/import/export paths remain denied where the contract
  requires denial;
- a second deployment is idempotent;
- destroy removes all owned resources and records residual inventory;
- onboarding has been followed literally from public artifacts;
- capability documentation claims no more than this tuple; and
- the release candidate came from the same commit and artifact identities as a
  successful canary.

Generated CI, HA, full disaster recovery, every day-two operation, and every
integration are not required to call this an alpha. They must remain clearly
experimental or unavailable until their own evidence closes.

## Risks and controls

### Risk: splitting gates lowers quality

Control: every existing proof remains owned by a named gate. Only A, B, and C
block the first alpha. Security negatives that protect an exposed alpha path
remain in C.

### Risk: automatic provider installation hides mutation

Control: print provider ID, version, source, signature identity, and cache
destination before download. Fail closed on any existing invalid material.
Offer explicit offline/prewarm installation.

### Risk: separate providers create version drift

Control: the release lock pins exact alpha provider versions. Core and provider
alpha tags stay lockstep. Protocol negotiation and project locks prevent silent
substitution.

### Risk: keeping GKE preserves complexity

Control: pin one recipe and freeze GKE Standard and HA expansion. Reconsider
Cloud Run only through a standalone provider-runtime prototype after alpha.

### Risk: AWS silently rots

Control: shared contract changes compile and test AWS. Path-scoped AWS live
cells run when AWS or a shared contract changes, not for GCP implementation
changes.

### Risk: published base images become a maintenance burden

Control: keep a small compatibility catalog, immutable digests, SBOMs,
provenance, vulnerability policy, and explicit end-of-support status. Do not
publish an open matrix of untested combinations.

### Risk: artifact contracts proliferate

Control: designate the existing ArtifactManifest, ImmutableArtifactContract,
and BuildArtifact as the only three views. Any new artifact type requires an
ADR explaining why one of those cannot be extended.

### Risk: alpha claims production readiness

Control: use alpha language, keep the capability matrix tuple-specific, and
separate prior live evidence from the exact published release path.

## Changes that should not be made

- Do not start a new repository.
- Do not rewrite GCP in Terraform to debug an HTTP 500.
- Do not replace GKE before identifying the application failure.
- Do not create another artifact manifest.
- Do not merge GCP and AWS Pulumi graphs.
- Do not bundle every first-party provider into every CLI archive.
- Do not remove provider verification to simplify onboarding.
- Do not make ordinary deploy credentials able to edit their own trust setup.
- Do not delete acceptance, certification, recovery, or experimental-provider
  work merely because it moves off the first-alpha critical path.
- Do not cut further public release candidates as disposable debug builds.

## Decisions requested

Adopt the following as the architecture baseline:

1. Existing repository, Go CLI, YAML, and Pulumi remain.
2. Existing artifact contracts remain; no new bundle format is introduced.
3. GKE Autopilot remains the first GCP alpha runtime.
4. The provider subprocess architecture remains.
5. First-party providers remain separate signed artifacts and are
   automatically installed from a pinned release lock.
6. Alpha versions remain lockstep without physically bundling binaries.
7. Bootstrap remains an explicit one-time command.
8. MageLift publishes tested builder/runtime images.
9. Box 1.4 is superseded by independent Gates A through F.
10. Gates A, B, and C block the first alpha; generated CI and extended
    operations receive honest separate status.
11. Commit canaries replace public release candidates as the development
    feedback loop.
12. AWS remains a contract consumer during GCP work and becomes the second
    autonomous provider immediately after the GCP alpha.

## Required documentation changes if accepted

This report is analysis, not implementation authorization. An accepted reset
should update:

- [ROADMAP](ROADMAP.md) with the new gate sequence;
- the active correction intent or a superseding intent;
- [ADR 0001](../docs/adr/0001-yaml-only-cli.md) only if the primary command
  sequence changes;
- [ADR 0014](../docs/adr/0014-distribution-trust.md) to specify transparent
  first-party provider installation;
- [architecture](../docs/architecture.md) to make the four-layer ownership
  model explicit;
- [getting started](../docs/getting-started.md) and GCP onboarding so users no
  longer build MageLift base images or manually install the first-party
  provider;
- [capability matrix](../docs/capability-matrix.md) to state the exact first
  alpha tuple; and
- release and evidence documentation to distinguish commit canaries from
  public candidate qualification.

No capability status should be upgraded merely because this report is
accepted.

## Reference appendix

### Internal implementation and decisions

- [Artifact manifest implementation](../build/src/Artifact/ArtifactManifest.php)
- [Build pipeline](../internal/build/pipeline/pipeline.go)
- [Immutable artifact identity](../sdk/artifact.go)
- [Deploy transport types](../sdk/types.go)
- [Deploy orchestrator](../internal/deploy/orchestrator.go)
- [Public module contract](../sdk/modules.go)
- [Provider registry](../internal/registry/registry.go)
- [Provider lock format](../internal/providerhost/lock.go)
- [Provider resolver](../internal/providerhost/resolve.go)
- [Provider downloader](../internal/providerhost/download.go)
- [Provider metadata fetch](../internal/providerhost/fetch.go)
- [GCP capability and evidence matrix](../docs/capability-matrix.md)
- [GCP topology notes](../docs/gcp-experimental.md)
- [Provider plugin ADR](../docs/adr/0013-provider-plugin-contract.md)
- [Distribution trust ADR](../docs/adr/0014-distribution-trust.md)

### Clean-room field lessons

- [Magento image builds must not see a runtime env.php](../.agents/knowledge/lessons/Magento%20image%20builds%20must%20not%20see%20a%20runtime%20env.php.md)
- [Magento deploy writes env.php before traffic moves](../.agents/knowledge/lessons/Magento%20deploy%20writes%20env.php%20before%20traffic%20moves.md)
- [AWS Magento wiring follows a prior shop](../.agents/knowledge/lessons/AWS%20Magento%20wiring%20follows%20a%20prior%20shop.md)
- [MageLift commands hide cloud DevOps](../.agents/knowledge/lessons/Magelift%20commands%20hide%20cloud-devops%20from%20merchants.md)
- [Acceptance reuse identities](../.agents/knowledge/lessons/Acceptance%20JSONL%20PASS%20needs%20reuse-boundary%20identities.md)
- [OCI republish does not copy signatures](../.agents/knowledge/lessons/OCI%20republish%20does%20not%20copy%20Sigstore%20signatures.md)

### External references

- [SST documentation](https://sst.dev/docs)
- [SST Next.js component](https://sst.dev/docs/component/aws/nextjs)
- [OpenNext architecture](https://opennext.js.org/aws/inner_workings/architecture)
- [Pulumi Automation API](https://www.pulumi.com/docs/iac/concepts/automation-api/)
- [OpenTofu provider installation](https://opentofu.org/docs/cli/plugins/)
- [OpenTofu dependency lock](https://opentofu.org/docs/language/files/dependency-lock/)
- [HashiCorp go-plugin internals](https://github.com/hashicorp/go-plugin/blob/main/docs/internals.md)
- [Cloud Native Buildpacks lifecycle](https://buildpacks.io/docs/for-platform-operators/concepts/lifecycle/)
- [GKE Autopilot overview](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/autopilot-overview)
- [GKE mode selection](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/choose-cluster-mode)
- [Cloud Run overview](https://docs.cloud.google.com/run/docs/overview/what-is-cloud-run)
- [Magento deployment overview](https://experienceleague.adobe.com/en/docs/commerce-operations/configuration-guide/deployment/overview)
- [Magento deployment technical details](https://experienceleague.adobe.com/en/docs/commerce-operations/configuration-guide/deployment/technical-details)
- [SLSA provenance](https://slsa.dev/spec/v1.2/provenance)

## Final recommendation

Continue MageLift. Do not restart it.

The project's architectural direction is broadly correct: portable Magento
contracts, immutable artifacts, provider-owned topology, embedded
infrastructure automation, and evidence-gated claims. The failure is that the
release process has made all of those concerns mature simultaneously before
allowing one small public alpha.

Make the first alpha a narrow, repeatable GCP product slice. Preserve the
provider and artifact boundaries that AWS needs. Turn provider installation
into transparent plumbing. Keep bootstrap explicit. Publish the base images.
Debug Magento from commits, qualify releases from known-green commits, and let
each additional provider implement the same lifecycle without sharing its
cloud graph.

That path reaches a usable alpha faster and leaves less to reimplement when AWS
becomes the next priority.
