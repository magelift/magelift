# Roadmap: agency test alpha on GCP

Updated: 2026-10-18.

The active [implementation plan](../IMPLEMENTATION_PLAN.md) is v1.3. It
replaces plan v1.2 and the ML-xx priorities. It is execution authority; no
repeated spec/plan approval is needed for each subtask. Existing verified
work is retained. Historical reports remain historical evidence.
Tags/publication, history rewrite, protected destruction and spending outside
the envelope still require approval. Never destroy foreign resources.

Work stays on `reboot/community-alpha`, created from `wip/all-local-work`.
The base recorded by the maintainer was `3ac1b844a7117752f3a5045054587508c16dcf6c`.
The latest committed implementation before this revision was `7aa79fe`; its
uncommitted source-build, cache and CI changes were preserved during the update.

## Outcome

A Magento developer can install one product version, inspect an existing shop,
initialize a test environment, prepare the authorized GCP project and CI, then
build, publish and deploy through GitHub Actions. The guide does not require
them to design IAM, PKI, Pulumi state, networking or a cluster topology.

The CLI ships versioned recipes and PHP build code. The user's runner builds
the intermediates and publishes the deployable application to their registry;
there are no central MageLift Docker images. Artifact Registry is the GCP
reference default. Cache is optional, private where necessary, and separate
from deployable artifacts. A promotion reuses an existing digest and manifest.

This is an alpha for test environments. Production migration recommendations
require the pilot's restoration/migration proof. No HA, all-provider matrix,
console or mandatory AWS extraction blocks the test alpha.

## Active sequence

| Lot | Current state | Remaining outcome |
| --- | --- | --- |
| S0: usable correction loop | Community gates pass across root/SDK/GCP with bounded race and current PHP; original module/theme overlay built and checked in a real image | Preserve fast local work without a cloud account or release tag dependency |
| S1: build with distributed software | Active carryover: embedded source recipe, local/default builds, optional cache, image/manifest trace, verified provider acquisition and local OCI registry proofs | Real QPT, optional Cloud Patches, changed hotfix/local-cache and fresh extracted-archive/cold-BuildKit builds pass; remote signed CI/runtime acceptance remains with S2/S3 |
| S2: guided agency and CI preparation | Single-test init/CI, isolated federation, owned registry/state-key/node bootstrap, explicit CI grants and non-overwriting GitHub-variable setup pass local regression gates | Demonstrate repeated bootstrap and real authorized private-repository GCP CI, runtime pull and observed runner/resource use |
| S3: supportable alpha | Existing deploy/ops/doctor/JSON/skills retained; local-only doctor and voluntary metadata export in verification; Linux packaging smoke passes | Functional GCP scenario, second release and persistence, useful incident, final diagnostic/install/release evidence and independent cleanup |
| S4: agency pilot | Deferred until a real pilot; existing dump/import/media and local runtime stay available | Isolated shop trial and restoration proof before real commercial data recommendations; requested previews and second-runner recovery |
| S5: useful AWS | Experimental path retained; targeted tests within budget | One grouped ECR/ECS scenario and cleanup; extraction only if a measured problem justifies it |

S0-S3 lead to the alpha. S4/S5 follow according to pilots and do not become
retroactive publication gates. Former ML-06 through ML-09 obligations are not
an active backlog merely because they appeared in the older plan. Console,
MCP, universal plugin stability and extra providers have the triggers in plan
section 7, not unconditional implementation commitments.

The old [published-build-images intent](published-build-images/intent.md) is
superseded. Keep useful Docker sources/tests; do not execute central publishing.
The former alpha-loop/provider/build reports under
[community-alpha](community-alpha/verification.md) record what actually ran;
they do not prove the new guided agency path or a cloud run by themselves.

## Resource envelope

GCP acceptance is authorized in `digital-lab-341608` (the maintainer also
previously allowed `n8n-dlab` if rights required it). Prefer the selected
`us-central1` reference region, whose compared compute/SQL components tied
for the lowest inspected prices. Recheck the exact service/cost assumptions
before mutations; credits do not prove zero gross cost. The current
community-alpha campaign keeps its existing project, region, stack, state
and key until the final verified teardown in `IMPLEMENTATION_PLAN.md`.
Do not mint a fresh prefix or destroy that stack to rerun a probe. After
that teardown, new runs go back to a fresh `magelift-` prefix, an owner
record, a bounded lifetime and cleanup. No other unapproved retained debug
environment.

The configured default AWS account is authorized for targeted acceptance.
The announced credits are approximately USD150: at most USD100 planned test
spend after checking coverage/current consumption, keeping USD50 reserve.
One active disposable stack by default; session estimate/time limit,
inventory and cleanup precede new creation. No authorization to pay by card
when credits are exhausted. Current recorded AWS test spend/reservation: zero;
remaining account credits have not been verified.

## Verification and honesty

Use `make verify-community` for the community gate, narrow package tests for
iteration, and explicit heavier CI checks when relevant. Keep clean-room,
trust, locks, secrets and ownership protections. No unbounded root race run,
no local GoReleaser multi-target matrix, and no removed assertion to hide a bug.
A local Docker or unit success does not prove GitHub CI, runtime pull or GCP.
A human guide trial remains NOT_RUN until an outside person actually performs it.

Report delivered changes, executed tests, NOT_RUN, limits, remaining resources
and the next useful work. No new certification engine, state database or
parallel manifest is needed.
