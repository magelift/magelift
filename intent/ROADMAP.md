# Roadmap: GCP alpha, AWS second, provider platform third

Updated: 2026-09-22.

## Goal

Ship a useful public alpha quickly without creating work that must be thrown
away when AWS follows:

1. Make one pinned Magento Open Source recipe repeatably deploy on GCP GKE
   Autopilot from the supported user path.
2. Publish that path as the first alpha.
3. Move AWS ECS behind the same autonomous provider contract.
4. Harden the contract only after GCP and AWS have proved what actually needs
   to be portable.

The product remains a Go CLI that consumes magelift.yaml and deploys into the
user's cloud account. MageLift is not a host.

The decision basis is [report.md](report.md). Current code, accepted ADRs,
capability evidence, and the repository instructions remain authoritative when
they conflict with an older planning record.

## Reset decision

The previous queue made one clean-machine run prove installation, provider
distribution, image construction, GCP infrastructure, Magento behavior, media
security, resolver precedence, generated CI, extended operations, and cleanup.
That coupled unrelated defects to public release candidates.

This roadmap replaces that queue. It does not reverse the useful architecture:

- keep Go, YAML, Pulumi Automation API, and the monorepo;
- keep the existing artifact manifest and immutable artifact identity;
- keep GCP as an autonomous signed provider process;
- keep provider-owned cloud topology;
- keep GKE Autopilot as the first GCP runtime;
- keep explicit account bootstrap;
- keep existing AWS code and evidence warm;
- keep experimental providers frozen; and
- keep capability claims bounded by the capability matrix and evidence pack.

The change is the order of proof and the supported surface.

## Working sequence

Work one intent at a time:

1. accept intent;
2. write and approve spec;
3. write and approve plan;
4. implement;
5. verify in isolation;
6. archive only with a passing verification report.

No implementation starts from a draft intent. Later draft intents may be
corrected using evidence from earlier work.

| Order | Intent | Purpose | Exit | Status |
| --- | --- | --- | --- | --- |
| 1 | [alpha-development-loop](alpha-development-loop/intent.md) | Stop debugging through public release candidates. Close the rc.22 experiment and establish commit-addressed GCP canaries with independent gate results. | The HTTP 500 has an evidence-backed cause and regression test; provider children terminate; retained resources are gone; a commit can run distribution, artifact, and GCP product gates without a semantic-version release. | in progress |
| 2 | [transparent-provider-delivery](transparent-provider-delivery/intent.md) | Preserve the signed provider boundary while removing manual provider installation from the normal online path. | A clean install automatically acquires and verifies the exact release-pinned GCP provider; project pins win; tampering fails closed; offline/prewarm installation remains explicit. | draft; after 1 |
| 3 | [published-build-images](published-build-images/intent.md) | Stop asking users to clone MageLift and build framework base images. | A release catalog publishes tested builder/runtime digests with compatibility and provenance; the reference shop builds from those assets and emits the existing artifact manifest. | draft; after 2 |
| 4 | [gcp-alpha-release](gcp-alpha-release/intent.md) | Prove and publish the smallest useful GCP product slice. | Two clean commit canaries pass the pinned GCP recipe; a candidate from the same identities passes public distribution qualification; onboarding is executed literally; the report ends in a go/no-go and ask-first tag command. | draft; after 3 |
| 5 | [aws-autonomous-provider](aws-autonomous-provider/intent.md) | Add AWS immediately after GCP without a second application lifecycle. | AWS ECS runs as a separate signed provider using the same artifact, operations, lifecycle, distribution, and error contracts; AWS topology and evidence remain provider-owned. | draft; after 4 |
| 6 | [provider-ecosystem-readiness](provider-ecosystem-readiness/intent.md) | Turn the two-provider experience into a durable extension contract before adding more clouds. | Core has no provider SDKs, provider modules have no root-internal imports, provider-owned configuration schemas and conformance tests exist, and compatibility rules support independent provider releases. | draft; after 5 |

## First-alpha gates

The old monolithic box 1.4 is replaced by independent gates. A failure keeps
only its own gate red.

| Gate | Owner | Proves | Blocks first alpha |
| --- | --- | --- | --- |
| A. Distribution | orders 1–2 | CLI installation, provider acquisition, signature/digest verification, negotiation, process cleanup | yes |
| B. Artifact | order 3 | published build inputs, database-free build, existing manifest, provenance, signature, secret absence | yes |
| C. GCP product | orders 1 and 4 | bootstrap, deploy task, serving rollout, Magento HTTPS/assets/search/cron/queue/media, second deploy, destroy | yes |
| D. Project provider pin | order 2 | project lock precedence, exact executed identity, incompatibility and tamper rejection | yes for the documented pin feature; does not require a second Magento deployment |
| E. Generated CI | later pilot work | generated workflow and workload federation from released artifacts | no; experimental until proved |
| F. Extended operations | capability-specific follow-ups | backup/restore, failed release recovery, credential expiry, preview expiry, HA, external integrations | no, unless the alpha claims that exact capability |

Security negatives for an endpoint exposed by the alpha stay in the relevant
blocking gate. Splitting gates is not permission to omit trust-boundary tests.

## First-alpha product boundary

The public alpha has one paved recipe:

| Concern | Alpha choice |
| --- | --- |
| Application | Magento Open Source 2.4.9, repository-evidenced patch and PHP tuple |
| Provider/runtime | GCP / GKE Autopilot |
| Database | Cloud SQL MySQL version in the evidenced compatibility intersection |
| Cache/session | Evidenced Memorystore Valkey version |
| Search | One-replica OpenSearch workload on Autopilot |
| Queue | Magento database queue |
| Media | Private GCS through the implemented Magento remote-storage contract |
| Secrets | Secret Manager references; no plaintext YAML |
| Deployment | One candidate GKE Job before web, cron, and consumer rollout |
| Edge | Existing GCP HTTPS load-balancing path |
| Observability | Native logs required for diagnosis; external telemetry is not an alpha gate |

The exact immutable digests are selected by the release intent, not frozen in
this roadmap.

The alpha does not promise:

- arbitrary Magento, PHP, database, cache, search, or queue combinations;
- GKE Standard or high availability;
- zero-downtime incompatible schema changes;
- cross-region or cross-cloud disaster recovery;
- generated CI;
- every existing day-two operation;
- a hard cloud-spend cap;
- external edge or observability integrations; or
- a stable provider API.

Existing broader capabilities keep the status recorded in
[the capability matrix](../docs/capability-matrix.md). This roadmap does not
downgrade or upgrade certification by itself.

## Development and release policy

### Product debugging

Product debugging uses commit-addressed canaries in the dedicated acceptance
account. Canaries use the production provider boundary and immutable artifacts,
but do not require a public semantic-version tag.

Every live run:

- loads the certification skill;
- records the exact commit, CLI, provider, build inputs, image, manifest, and
  state identities;
- declares owned resources before mutation;
- destroys on exit unless a retained debug cell is explicitly authorized;
- inventories residual resources and spend; and
- records each gate independently.

### Public release qualification

A release candidate may be cut only from a commit that already passed the
blocking product gates. Candidate qualification proves that public module
publication, archives, signatures, locks, downloads, and runtime identities
match the known-green canary.

Release tags remain ask-first. No intent may hide a release tag inside an
implementation or verification step.

## Provider expansion rule

The reusable product boundary is:

- common YAML envelope and Magento application model;
- existing artifact manifest and immutable artifact contract;
- deploy phase semantics;
- typed provider operations, progress, and errors;
- provider discovery and trust;
- health and evidence semantics; and
- acceptance scenario definitions.

Providers do not share network, database, cache, search, queue, runtime, edge,
or observability Pulumi graphs. AWS follows GCP through the same contract but
keeps AWS topology. Additional providers wait until order 6 demonstrates that
the contract is not shaped around only GCP and AWS implementation accidents.

## Preserved but unscheduled work

The following work remains valuable but has no active intent until the stated
trigger:

| Work | Trigger |
| --- | --- |
| Generated GitHub CI | The published laptop path is green and a pilot needs team automation |
| Backup/restore and failed-release recovery | First alpha is installed by pilots; prioritize from operational risk and feedback |
| Production/HA GCP profile | A pilot needs production scale and accepts the cost/evidence program |
| Cloud Run runtime | After GCP alpha; bounded standalone prototype must beat Autopilot on measured user or operating cost |
| Independent provider cadence | GCP and AWS supported; one provider needs a release without a core change |
| Stable contract/version | GCP and AWS pilots plus order 6 demonstrate the freeze surface |
| OVH and Scaleway | After AWS unless a funded pilot makes one the next provider |
| EKS and GKE Standard | A concrete workload cannot fit the certified runtimes |
| External edge/observability | Stable origin first; add by user demand |

Do not pre-create implementation intents for these. Open an intent when its
trigger is real and evidence can define the outcome.

## Disposition of the previous live queue

The 2026-09-16/17 live planning queue was superseded by
[the architecture report](report.md). Its unfinished folders were not moved
into the completed archive because they lacked an isolated passing report.
Their tracked contents remain recoverable from Git history.

| Previous record | Disposition |
| --- | --- |
| alpha-review-corrections | Superseded by orders 1–4; findings remain evidence, not one release gate |
| reference-store-acceptance | Replaced by the narrower GCP alpha intent and independent Gates A–F |
| aws-provider-parity | Rewritten as aws-autonomous-provider and moved immediately after GCP alpha |
| eu-providers-experimental | Removed from the active queue; capability docs and evidence remain; resume only on trigger |
| v1-stable-cut | Removed; stability is a decision after two providers and pilots |
| loose alpha review and audit | Superseded by report.md and this roadmap |

Existing directories under intent/archive remain immutable decision history.

## Budget and resource discipline

| Scope | Rule |
| --- | --- |
| GCP canaries and alpha | Dedicated acceptance project only; smallest evidenced recipe; destroy on exit; no KEEP by default; exact residual inventory |
| AWS parity | Reconfirm credits and set a per-session budget before live work; minimum shape; destroy and assert clean |
| Other providers | No live spend until their trigger creates an accepted intent |
| Third-party vendors | No new paid integration gate before the origin path is stable |

Budgets alert and inform; they are not an enforcement claim unless code and
evidence prove enforcement.

## Success measures

The roadmap is working when:

- Magento defects can be reproduced and fixed without cutting a public RC;
- clean install to GCP preview has one documented MageLift-only path;
- no user builds MageLift framework images or manually installs a first-party
  provider;
- the same application artifact and lifecycle reach AWS;
- a provider-specific change does not require live execution on another cloud;
- release qualification is shorter than product acceptance;
- each capability claim points to exact evidence; and
- new providers require an adapter and evidence, not edits throughout core.

## Working agreement

- One active intent at a time.
- Smallest correct change; no unrelated feature breadth.
- No code before an approved plan.
- Reuse existing contracts before adding abstractions.
- Current code and ADRs outrank old intent prose.
- Mocks prove contracts, not live Magento.
- Human docs and public claims change with their ADR/capability source.
- Ask first for release tags, protected or production destroy, and history
  rewrite.
