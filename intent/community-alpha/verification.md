# Community alpha implementation verification

Base: `3ac1b844a7117752f3a5045054587508c16dcf6c` on
`wip/all-local-work`; implementation branch: `reboot/community-alpha`.
The maintainer supplied `MageLift_Plan_Implementation_Codex.md` (plan v1.0,
2026-10-17). There was no `CODEX_IMPLEMENTATION_PLAN.md` in the checkout.
The local root `go.mod` and `go.work` toolchain changes were preserved.

## ML-00 — complete (local scope)

Existing implementation retained: provider process cleanup, Execute cleanup on
success/error/cancellation, and setup:upgrade before app:config:import.

Executed on 2026-10-17:

| Command | Result |
| --- | --- |
| `composer install --working-dir=build --no-interaction --prefer-dist` | PASS; installed the committed lock, no dependency resolution |
| `go test ./internal/providerhost/ ./internal/cli/ ./internal/platform/ -run 'TestCachedDialerCloseReapsPlugin\|TestExecuteClosesProviders\|Migration' -count=1` | PASS; Go 1.27.2, memory limit from scripts/go-memlimit.sh |
| `go test ./providers/gcp/operations/ -run TestRegisterCandidateUsesPlatformMigrationContract -count=1` | PASS; workspace mode, Go 1.27.2 |
| `bash tests/acceptance/development_loop_gate_test.sh` | PASS; synthetic distribution and dry-run harness only |
| `make check-clean-room` | PASS |
| Initial `make php-test` with container fallback | FAIL; test RuntimeEnvTemplateTest requires images/php-nginx/env.php outside the mounted build directory |

The PHP fallback mount failure was fixed and rerun. Final checks:

| Command | Result |
| --- | --- |
| `MAGELIFT_PHP_TEST_CONTAINER=1 make verify-community` | PASS; scoped core, SDK, full GCP, bounded race, PHP validation/audit/PHPStan/Psalm, 133 PHPUnit tests and 310 assertions, lifecycle golden, clean-room, core dependency boundary, helper regressions |
| `MAGELIFT_PHP_TEST_CONTAINER=1 make lifecycle-golden-check community-test-helpers` | PASS after the final shared PHP runner change |
| Two simultaneous `go test ./internal/dumpimport/ -run 'TestImportCreatesTablesFromTinySQL\|TestYesSchemaReplaceConverges' -count=1 -v` | PASS in both processes; two integration tests each |
| `make docs` | PASS; initial out-of-site roadmap link fixed |
| `make workflow-check` | PASS |
| `go test ./internal/lintcoverage/ -run TestGoWorkflowJobsCapMemory -count=1` | PASS after adding the existing memory action to both canary Go jobs |
| `shellcheck --severity=warning` on the six new shell helpers/tests | PASS |
| `git diff --check` | PASS |

The broad `make test-fast` initially failed on the two canary jobs missing the
memory action; a subsequent run passed. Simultaneous broad verification runs
then exposed a real test-fixture collision: Docker Compose project names used
only the sequential temp-directory leaf `001`. The test helper now hashes the
full unique work directory, with a naming regression and concurrent MySQL
verification. Product import validation was unchanged.

`make test-fast` (and alias `make test`) retains all three module suites.
`verify-community` excludes root AWS, OVH, Scaleway, external-vendor suites and
their acceptance commands. Full root race, provider mocks, images, Floci and live
acceptance remain separate. No tests were deleted. The full local `make verify`
and its root-wide race were NOT_RUN; hosted CI was NOT_RUN.

The PHP runner supports installation from the committed Composer lock when
vendor is missing, uses the same environment for the lifecycle golden, mounts
only build and image inputs, and keeps container analysis caches off the host.
Container verification used Composer 2.10.3 and PHP 8.5.10; Go tests used 1.27.2.
The root toolchain edits present at session start remain unchanged; nested
module pins were aligned to 1.27.2.

Delivered: AGENTS/roadmap/release policy reconciled, three-module test loop,
scoped community gate, bounded race, reproducible PHP runner, CI memory fixes,
and test isolation. Next lot: ML-01.
No cloud mutations, tags, publication, or protected destruction were performed.
AWS budget reserved/consumed by this work: zero. No new cloud resources were
created; existing project inventory has not been asserted clean. A read-only
Docker inventory after the concurrency runs found zero remaining MySQL fixture
containers, networks or volumes.

The remove-ai-marks service at its configured default loopback address was
unreachable (connection refused); that documentation pass is NOT_RUN. Humanizer
review applies to modified contributor prose. No knowledge notes were written.

## Acceptance target

The maintainer explicitly authorized `digital-lab-341608` or `n8n-dlab` for the
milestone A acceptance run, with the cheapest compatible region and a clearly
identifiable MageLift prefix. Read-only preflight found both projects ACTIVE.
The seven checked recipe APIs are already enabled in digital-lab-341608;
n8n-dlab has one of those seven enabled. Region, IAM permissions, owned prefix,
immutable artifact and cleanup inventory still need verification before mutation.


## ML-01 — local implementation verified; live product proof pending

The existing canary `e840adff5dd5ddfe5f165b2a0d44584a90d4c14d` is still draft.
`GH_REPO=magelift/magelift MAGELIFT_DEVLOOP_FIXTURE=0 bash scripts/development-loop/distribution-gate.sh --sha e840adff5dd5ddfe5f165b2a0d44584a90d4c14d --gate-file .magelift/development-loop/ml01-canary/distribution.jsonl`
passed on Linux/amd64: real archive download, Sigstore verification, installation,
CLI version and signed provider verification. This proves that old canary's
distribution; it does not prove distribution of the current uncommitted tree.
No release was created or changed.

Read-only acceptance preflight: ADC refresh passed and the effective identity
matched the active gcloud principal. The checked create/use IAM permissions
passed on both authorized projects. digital-lab-341608 has all nine harness
APIs plus Artifact Registry enabled; n8n-dlab lacks nine of these ten APIs.

Region candidate: us-central1. On 2026-10-17, the public Cloud Billing Catalog
contained 44 regions with prices for all five compared components (Autopilot
CPU/RAM, MySQL Enterprise Plus N CPU/RAM and Valkey Shared Core Nano).
us-central1, us-west1 and us-west8 shared the minimum of each component.
The Cloud SQL db-perf-optimized-N-2 tier is available in us-central1 with
16 GiB RAM; GKE server configuration is available there. This comparison
excludes disks, IP/load balancer, registry, request charges and data transfer;
it is not a total run-price or credit-coverage guarantee. Source: Cloud Billing
Catalog API, plus [GKE pricing](https://cloud.google.com/kubernetes-engine/pricing)
and [Valkey pricing](https://cloud.google.com/memorystore/valkey/pricing).

Live product run: NOT_RUN while gate corrections and immutable synthetic-shop
inputs are being prepared. No resources created; no AWS reservation or spend.


ML-01 local delivery:

- Gates keep independent private result/log files. Crashes, malformed records,
  nonzero exits after PASS, and explicit FAIL all fail only that gate; remaining
  gates still run. The final aggregate is published atomically.
- Dry-run product checks and offline artifact contracts explicitly leave the
  real product/image result SKIP/NOT_RUN. PHPUnit exit 2 remains FAIL.
- Live product proof invokes `up`, uses a fresh short prefix, work directory and
  checkpoint, and requires the full ordered preview Autopilot application
  catalog, an immutable non-placeholder digest and current-run cleanup evidence.
  Retention, resume, infra-only and partial-catalog runs cannot pass this gate.
- Read-only project/API/ADC/create/delete permission checks precede mutation.
  Existing independent inventory rejects an occupied or unreadable prefix.
- Auto-named pre-existing buckets and encryption keys are not adopted. Explicit
  external references are preserved. A failed runtime cleanup preserves its
  encryption key, recovery state and passphrase; cleanup failure is nonzero.
  The zone choice is now included in the existing checkpoint fingerprint.

Checks on the integrated implementation:

| Command | Result |
| --- | --- |
| `MAGELIFT_PHP_TEST_CONTAINER=1 make verify-community` | PASS; all scoped modules, targeted race, PHP suite, boundaries and development-loop regressions |
| `make development-loop-test` | PASS; runner, artifact, product, preflight, ownership and existing independent-gate tests; doubles are not cloud proofs |
| `MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash tests/acceptance/gcp_harness_shape_test.sh` | PASS |
| `bash tests/acceptance/gcp_dry_run_no_account_test.sh` | PASS |
| Source `gcp-acceptance-preflight.sh` with the authorized project, us-central1 and the nine real harness APIs; call `gcp_acceptance_preflight` | PASS with actual ADC; read-only project/IAM/API calls only |

The pre-existing bucket adoption test failed before the guard and passed after
it. Ownership, occupied/unreadable prefix, runtime cleanup failure and auxiliary
cleanup failure have regression coverage. Local Docker MySQL fixtures were
removed by their owning tests. No live resources were created; AWS reserved and
consumed budget remains zero. The newest synthetic-shop image is not available
yet; the one real Magento run remains a milestone A blocker, not a local code
blocker. Do not treat the old draft canary distribution pass as proof of this
working tree. Next implementation lot: ML-02; final live proof stays scheduled
with the qualified build inputs from ML-03 and the release scenario in ML-05.

ML-02 in progress (not a completed lot):

- Cursor workers: Composer 2.5 for provider cache; Grok 4.7 high for lazy
  first-party acquisition; Grok 4.7 medium for existing tool installation.
  These model IDs use their non-fast variants.
- Pinned Cosign remains v3.1.3: GitHub's latest release query returned that
  version, published 2026-08-06. No dependency rotation is needed for it.
- Python 3.15.0 was installed with mise without changing global settings.
  Both the preflight unit suite and actual read-only acceptance preflight
  passed under that interpreter. The initial uv installation attempt failed
  because that interpreter was not available through its managed downloads.
- Provider cache installs atomically via a version-level staging directory
  renamed into `<goos>_<goarch>/`, so a crash cannot leave a resolver-visible
  binary without its bundle. Advisory `.install.lock` sits on the version
  parent; repair clears only known artifacts for the active platform and
  preserves the lock inode. Corrupt platform state does not fall back to
  legacy layout; legacy digest mismatch fails closed. Explicit
  `magelift providers install` uses `Repair: true`; lazy paths stay fail-closed.
  Targeted `internal/providerhost` download/cache tests PASS (including
  subprocess concurrency and canceled download); combined ML-02 lot still
  open for acquisition, toolchain, and release surfaces.

Historical canary asset sizes queried from GitHub (not current build sizes):
Linux amd64 CLI archive 62,858,957 bytes; provider binary 207,511,712 bytes;
provider signature bundle 11,271 bytes; platform lock 1,021 bytes. These values
exclude the extracted CLI, on-demand tools and Docker images. They must not
be reported as the total installation footprint.

Incremental commits requested by the maintainer:

- `1d2f27a` docs(alpha): reconcile maintainer plan and milestone A priorities
- `fd501df` fix(alpha): make community gates independent and acceptance cleanup fail closed
- `067e1fe` test(dumpimport): isolate concurrent Compose fixtures
- `ce6c3b6` chore(go): align workspace toolchains on Go 1.27.2
- `7592eaa` style(tests): remove trailing whitespace in ownership regression
- `a80a14f` fix(providerhost): install verified platform caches atomically under process locks
- `4c2b2ee` fix(providerhost): reject provider names that escape the cache
- `b0625dd` fix(cli): verify bundled providers before publishing bootstrap locks

Root `go.mod` and `go.work` already contained the maintainer's Go 1.27.2
selection before this task; that selection was preserved and committed with
the two nested module toolchain alignments. No history rewrite or remote push.

Additional ML-02 checks: the complete `internal/providerhost` suite PASS
(9.734s); targeted provider CLI tests PASS, including invalid signatures,
tampered binaries and failed bootstrap leaving no project lock. A project pin
created concurrently is preserved and causes bootstrap publication to fail.
These are local checks, not a release or cloud qualification.

Local resources at 2026-10-17 18:32 UTC: workspace filesystem 549 GiB
available, /tmp 11 GiB available, RAM approximately 16 GiB available. No
cloud resources created by this task so far; AWS budget reservation and
consumption remain zero. These are workstation measurements, not an estimate
of remaining GCP credits.

Further ML-02 execution results:

| Check | Result |
| --- | --- |
| `go test -race ./internal/providerhost -run 'TestDownloadInstallConcurrent\|TestEnsureVerifier\|TestVerifierLegacy' -count=1` | PASS (2.661s); bounded package/runs only |
| `go test ./internal/upgrade/ -count=1 -run TestInstall` | PASS; Linux fixtures plus Windows ZIP extraction on Linux, not Windows execution |
| `bash scripts/install-verify-harness.sh` | PASS (14 checks); local fixture server, not a public release install |
| `make docs check-clean-room workflow-check` | PASS |
| `mise exec goreleaser@2.18.3 -- goreleaser check` | PASS; configuration only, no release matrix |
| `go build -o /tmp/magelift-community-alpha/magelift-current ./cmd/magelift` | PASS |
| Built CLI `version`, `--help`, sample-shop `config validate` with an empty dedicated provider cache | PASS; cache remained empty |

GoReleaser v2.18.3 was confirmed against the upstream latest release
(2026-10-09) and installed with mise without changing global activation.
The local unstripped development CLI above is 439,728,419 bytes; it is not a
release archive or stripped installed release. Current tool cache and Docker
image totals remain unmeasured; `docker system df` did not return promptly.
No cache pruning was performed.

Read-only checks reached the project-owned `magelift/homebrew-tap` and
`magelift/scoop-bucket`. The configured repositories and archive formats are
not install evidence. Brew install, Scoop install and native Windows process
execution remain NOT_RUN. Existing remote manifests refer to an older release.

Preparation for ML-03: registry inspection of both
`ghcr.io/magelift/magelift-builder:8.5` and
`ghcr.io/magelift/magelift-nginx:8.5` returned `not found`. No new image was
published and no old application digest was adopted as the new acceptance
artifact.

ML-02 acquisition and offline proof:

- Both production GCP lazy dialing and the CLI backend loader use the existing
  resolver/downloader through `Acquire`. Project pins win over beside-CLI locks
  and the exact CLI release lock. Malformed locks, missing GCP entries and corrupt
  caches fail without selecting another version. Non-GCP names are refused by
  automatic acquisition. Release lock cache writes serialize across processes.
- Required signed-release and session-launcher tools use the existing package
  manager allowlist with interactive confirmation or `--yes`. Non-interactive
  missing-tool errors, Docker prerequisites and cloud authentication remain
  explicit. No Go toolchain is required by an installed release.
- A real proxy-blocked Cosign 3.1.3 check initially FAILED: the default verifier
  attempted a live TUF refresh despite its cache. Passing the authenticated
  Sigstore trust root succeeded. The CLI now carries that public snapshot,
  authenticated with `cosign initialize` on 2026-10-17, and validates its digest
  before writing or using it. Corruption is refused; explicit repair restores
  the embedded bytes. Publisher, certificate, transparency and digest checks
  remain enabled. Root rotation requires a reviewed CLI update.
- `MAGELIFT_SIGNED_PROVIDER_ACCEPTANCE=1 go test ./internal/providerhost -run
  '^TestAcquireSignedReleaseThenOffline$' -count=1 -v`: PASS (9.442s) using
  actual signed historical canary artifacts, authenticated staging via `gh`,
  real pinned Cosign verification, then HTTP-denied acquisition and proxy-blocked
  Cosign verification. The initial direct download trial was FAIL/HTTP 404
  because the historical canary is still a draft. This is not proof of public
  distribution of the current source or of current-provider cloud behavior.
- Complete `internal/providerhost` and `internal/cosign` suites after the root
  correction: PASS. The final combined community gate is running after these
  security changes; its result is not inferred from the earlier green run.

Local packaging smoke:

- GoReleaser 2.18.3 single Linux amd64 snapshot: PASS after correction.
  First attempt: FAIL because `dist` does not expand Go templates; its output
  was moved to private `/tmp/magelift-community-alpha/failed-smoke-dist`.
- The corrected script uses a concrete private temporary output path, includes
  LICENSE/NOTICE, checks provider presence and executes the extracted CLI.
  Pre-existing CLI/provider files in `dist/` retained their original SHA-256.
- Measured snapshot: archive 62,878,001 bytes; extracted stripped CLI
  307,945,632 bytes; provider 207,552,672 bytes. These exclude the on-demand
  verifier, Docker images and caches. This unsigned local snapshot is not a
  published release.
- Repeated real historical distribution gate with Cosign 3.1.3: PASS. No tag,
  release publication, tap/bucket write or cloud resource mutation occurred.

ML-02 final local result: PASS.

`MAGELIFT_PHP_TEST_CONTAINER=1 make verify-community docs workflow-check`
completed with exit 0 after the offline trust-root correction. It ran the root,
SDK and GCP suites independently, targeted race checks, PHP checks, golden
contract, clean-room and core boundary checks, gate regressions, strict docs
and workflow validation. Before this pass, module distribution tests failed
because the first smoke's 552 MiB artifacts were retained under `.magelift`;
they were moved outside the source tree, preserving the artifacts and the
module ZIP test. No exclusion or test bypass was added.

Implementation commit: `35a8e48` (`feat(cli): acquire verified GCP providers at
activation with offline cache support`). Packaging smoke correction:
`4d76bf4`. Native Brew/Scoop/Windows remain NOT_RUN and are not advertised as
qualified. Current-source public distribution and current-provider cloud
execution remain release-scenario checks, not inferred passes.

The initial Grok 4.7 medium/high implementation tasks produced no edits in
approximately ten minutes and were interrupted (exit 130). Composer 2.5 then
implemented those bounded slices; the parent integrated, corrected and verified
them. Grok 4.7 low completed the packaging inspection and executed its fixture
checks. All Cursor invocations used non-fast model IDs.

The shared Docker inventory eventually returned: images 100.6 GB, container
writable layers 1.694 GB, local volumes 80.42 GB and build cache 48.65 GB.
These are workstation-wide values, not this task's installation footprint or
removable task resources. Nothing was pruned. Latest workspace availability
was 540 GiB; /tmp had 11 GiB available. No cloud resource was created and no
AWS budget was reserved or spent. Actual remaining account credits were not
queried or inferred from these local measurements.

Next: ML-03. Magento 2.4.9 remains the latest stable line among the upstream
releases inspected; 2.4.10-alpha1 is an upstream alpha despite its GitHub
prerelease metadata. The current official PHP 8.5 FPM Trixie image resolves to
`sha256:adde18b67ea7a8cddc580f657e0d3f16e800758c6fa690b969c2925a303e8afc`
and an actual Linux amd64 run reports PHP 8.5.11. No image was published.

## ML-03 — local build verification; publication pending

Implementation commits: `b2ac1d2` (current image inputs and context exclusions),
`5e59e33` (registered themes), `5064ea3` (image resolution and trace),
`b08f962` (scoped publication workflow). No image, tag or branch was published.
Default published image pins remain empty deliberately: the default resolver
reports unavailability until real GHCR digests are published and anonymously
verified. Explicit contributor image overrides were used for the checks below.
ML-03 is therefore not complete.

Current recipe: Magento Open Source 2.4.9, PHP 8.5.11, Composer 2.10.3,
nginx 1.30.4, Linux amd64. Docker 29.9.1 / Buildx 0.38.0 / BuildKit 0.34.0
were used through the default builder; the user's older named builder was
not changed. Dockerfile frontend 1.28.0 and official base images are digest
pinned. No new dependency framework or manifest was introduced.

Actual local builds from a clean synthetic Magento Git source passed twice.
Images were `sha256:2704c810f175b744497e9dcea07640e762e2a0ed2ed35c64916cb3461ab93150`
and `sha256:0bd7deb48f8275b044ef2f70429bcd01cb931915b45f6817ea5cca25e2231a06`.
Source revision, build inputs, versions, 222 enabled modules, static matrix,
compatibility, capabilities and checksums matched. Image digests differ;
bitwise reproducibility is not claimed. These runs exposed a hardcoded CLI
version in the PHP manifest; the environment now carries the normalized CLI
version and OCI labels carry the builder/runtime references. The existing
external manifest and provider contract are unchanged.

Final existing-shop integration passed in 170.005 seconds using the corrected
builder. Image:
`sha256:d73c03a572df2bea0b957d6ba49b6d5ab7d0dfe9ab6cdd2b80244221e9b39f8a`.
Existing manifest SHA256:
`0999e94aa325d22f49562ff56bc2e7312cc9ae9b62900de7d7916918f44729f7`.
The source configuration and clean Git tree remained unchanged; the custom
MageLift_AlphaDisabled module remained disabled in the runtime image. A fake
Composer credential canary was absent from captured logs, the manifest, and
all 39 exported archive entries (1,125,084,796 uncompressed bytes inspected).
OCI version and base image labels matched. The test is opt-in and skips when
its explicit local fixture/image inputs are absent. Proof and raw manifests
are private under `/tmp/magelift-community-alpha/`, not committed artifacts.

Negative real builds: missing PHP extension imap exited 3 as expected.
An unregistered theme initially incorrectly succeeded (exit 0), demonstrating
a real defect. After registrar validation was added without database bootstrap,
the same Missing/does-not-exist theme exited 3 with the expected error. The
positive existing-shop integration above used this final corrected builder.
Runtime inspection confirmed application YAML and auth.json were absent.
Local bare image IDs and named digest FROM experiments failed because BuildKit
attempted remote resolution; advanced local tags retain captured-ID drift
checks instead. No registry workaround or extra registry was introduced.

Final `MAGELIFT_PHP_TEST_CONTAINER=1 make verify-community docs workflow-check
cli-docs-check`: PASS, exit 0. This includes root/SDK/GCP community suites,
targeted race checks, clean-room and architecture gates, PHP static analysis,
audit, lifecycle golden contract, strict docs and actionlint. Final PHP suite:
135 tests, 317 assertions, PASS. Generated CLI docs and shipped skill manifest
were regenerated with project targets. Workflow tests execute the actual
version/source-tag guard and check the two-cell PHP85 amd64 publication scope.
Historical evidence-link informational MkDocs messages remain; docs built.

Public GHCR default-path cold pulls/builds, image signing/attestation and
current-source release installation: NOT_RUN, awaiting explicit publication
approval. Native Windows/macOS/package-manager runs remain NOT_RUN.
The remove-ai-marks service was unavailable (connection refused): NOT_RUN;
prose was reviewed with humanizer. No certification claim follows these tests.

Resources remaining: 526 GiB workspace and 7.6 GiB /tmp available at this
checkpoint. Task images/build cache and synthetic source fixtures are retained
locally for review; shared Docker resources were not pruned. Engine-reported
sizes for an earlier trace builder/runtime/application were respectively
956,296,496 / 837,517,312 / 1,662,426,781 bytes; layers are shared, these are
not compressed download sizes or an additive installation footprint. No cloud
resource was created, no AWS budget was reserved or spent, and account credits
were not measured. Authorized GCP target remains digital-lab-341608,
us-central1, with a fresh magelift-prefixed acceptance run.

## Maintainer correction — user-owned images

This checkpoint supersedes the ML-03 publication-pending proposal above. The
maintainer clarified that all builder/runtime/application images are built in
the user's CI/CD and published to their chosen OCI repository. The earlier
GHCR publication approval request is cancelled; no permission was inferred.
No MageLift image publication, tag, branch push or cloud deployment occurred.

Commits: `8619ba3` updates the product plan/roadmap/intent; `a4b108e` removes
MageLift image publishing from CI; `149643f` ships the existing recipes in the
CLI and builds the images on the user's Docker engine. No Go installation or
framework checkout is needed at runtime. No new manifest, registry service,
plugin framework or provider contract was introduced. CLI-generated bases use
the same user repository as `--image`, with distinct fingerprinted tags;
application builds consume the returned base digests. Local builds use private
invocation tags and captured-ID drift checks, without requiring RepoDigests
from Docker's classic image store. Official upstream image inputs remain pinned.

Real default-path builds, with no builder/runtime override and run from outside
the MageLift repository:

- Existing synthetic shop: PASS, exit 0. Image
  `sha256:7e18ed9a4d67fa93b1c2c5330bf72e8108d81ab0e84d0cf703bc82bc09cbc42e`;
  existing manifest SHA256
  `0a0a1d8ce7b8f9d19abb35f3f49c71033627c5c272f8688dd79ce75686412fdc`.
- Fresh synthetic shop with final resolver refinement: PASS, exit 0. Image
  `sha256:dacddf60022ede3f1391c948984ae91ca332b84242ae8dfe853b5e0cc39289af`;
  existing manifest SHA256
  `067325162b5fc5e3da3a71af50dd6a141a1cac0923040719425f773989413059`.

Both images report pushed=false and manifest CLI version 0.0.0-dev. Runtime
inspection of the existing-shop image confirmed MageLift_AlphaDisabled remains
disabled and application YAML/auth.json are absent. Source Git status is clean;
config.php hash stayed
`4e4e5adde8a1234f317606cd9f8f1d4a2ecfb62acd6529d7dff7ccac790ffa3e`.
Raw existing manifests and logs are private under
`/tmp/magelift-community-alpha/user-owned-images-*`.

Checks: full `MAGELIFT_PHP_TEST_CONTAINER=1 make verify-community docs
workflow-check cli-docs-check` PASS, exit 0, including 135 PHP tests / 317
assertions. Focused root/toolchain/CLI/pipeline Go suites PASS; after the local
RepoDigests refinement, root/toolchain/CLI suites PASS again. Bounded
`go test -race . ./internal/toolchain -count=1` PASS. Final actionlint PASS.
Registry-prefix tests exercise GHCR, ECR, Artifact Registry and port-qualified
names; subprocess fixtures verify push/load flags, same-repository tags,
metadata digests and fail-closed errors. These are local tests, not evidence of
actual publication to those services. Generated shipped skill manifest was
regenerated using `make generate cli-docs`.

Actual user-registry push, remote cold pull, signatures/attestations verification
and current-source GCP application acceptance remain NOT_RUN. Native ARM,
Windows/macOS and package-manager qualification remain NOT_RUN. Humanizer was
applied to human docs; the previously unavailable remove-ai-marks service was
not rerun and remains NOT_RUN. The earlier real canary secret scan still applies
to the unchanged PHP packaging path; it was not repeated for these two CLI runs.

Remaining resources: 522 GiB workspace and 6.6 GiB /tmp available. Local images,
cache, proofs and synthetic sources are retained; no shared Docker resource was
pruned. No cloud resources created, no AWS budget reserved/spent, remaining
account credits unmeasured. Registry retention is user-owned: failed pushes
can leave base images, which MageLift does not delete automatically.

Cursor Composer 2.5 (non-fast) implemented the bounded embedded-context and
image-builder slice; the parent corrected repository/tag isolation, integrated
the CLI and reviewed the final source/diff. An optional Grok 4.7 medium
(non-fast) read-only review produced no report in five minutes and was
interrupted (exit 130); independent agent review is NOT_RUN, not a pass.


## Plan 1.2 / S1–S2 checkpoint — 2026-10-18

The maintainer's v1.2 replaces earlier requirements. The earlier report above
about publishing builder/runtime tags is historical: commit 499a08e keeps those
intermediates local and publishes only the application. S0–S3 lead to the test
alpha; pilot-triggered S4/S5 are not publication gates. No tag or publication
has been performed.

Delivered: embedded recipe export without Docker/cloud access; default
source-built local intermediates and composed runtime stage; optional private
registry cache; build-only Composer env reference; existing manifest enriched
with recipe hash, platform and three pinned inputs. Existing provider and PHP
protocol contracts remain. The Docker recipe matrix is retained as build checks
without central image publication. A detached private Git checkout without any
configured author identity passes the new source regression.

Real checks on the synthetic Magento 2.4.9 shop, Linux amd64:

- Default CLI build without image overrides: PASS, exit 0, PHP 8.5.11 and
  Composer 2.10.3. Image b2f62e76572b34c190ae82d1a91b01aeb285c8d85109ba29d1ebfd6a8628b906;
  external manifest SHA256 1273889129fa8a0975ab94e32d0974120940ccaebab40d28799e21ab12f6981f.
  OCI recipe label matches manifest buildRecipe.sha256
  121d78a260d4544d40e9b69d14690355d548844415b7bf133cde1f743cfa78e8.
- Export with PATH containing no tools: PASS. Existing destination collision
  tests reject occupied recipe paths.
- Owned authenticated loopback registry 3.1.2: first push/pull test PASS,
  174.339 seconds; initial registry cache empty, workstation BuildKit warm.
  Second push/pull with populated cache PASS, 180.650 seconds. Build log records
  registry cache import and cached steps. These runs do not demonstrate a cold
  runner or a cache speedup; the second run included the new manifest metadata.
- Second image digest sha256:1f8d1548bef1a7798b193463f42f1dae2eed5e9b54bccf8f74838398004435c6;
  manifest SHA256 9a4f8259142bc3bcf8800adcaa030d67bb16cc863f4ad35b7f6693ff1f76b55b.
  Full image scan: 40 archive entries / 1,131,415,267 uncompressed bytes;
  fake Composer credential absent, source unchanged, disabled module preserved.
- First registry transport checked actual index/child/blob digests plus SPDX
  and SLSA attestation predicate types. Anonymous API returned 401; actual
  unauthenticated Docker push failed, exit 1. No denied tag was published.
  Initial cache/blob scan: 86 files / 32 gzip blobs / 1,247,410,585 decoded bytes,
  no Composer or fake registry credential. These pipeline tests do not invoke
  CLI Cosign signing and do not prove the full signature chain.

S2 CI changes: generation preserves customized workflows and reports an exact
candidate path; explicit --replace adopts the template, --check-template checks
optional freshness. Default validation reports its configuration/presence/YAML
syntax scope. User CI builds source recipes; GCP Artifact Registry, user GHCR
and explicit generic-registry auth remain separate. Build/deploy identity
references are separate. The existing manifest/checksum/result survive between
jobs; production promotion requires an existing successful same-workflow,
same-repository non-PR run and does not invoke the builder. Actual generated
Bash handoff scripts accept valid input and reject corruption/digest mismatch;
run-metadata tests reject fork/PR, failed and different-workflow sources.
These changes do not complete guided bootstrap: identity/policy variables still
need automatic generation, and the minimal single-test starter remains pending.

Executed: targeted CLI suite PASS; source suite PASS; complete
MAGELIFT_PHP_TEST_CONTAINER=1 make verify-community docs workflow-check
cli-docs-check PASS, exit 0. Includes root/SDK/GCP suites, bounded race,
150 PHP tests / 346 assertions, PHP static analysis/audit, lifecycle golden,
clean-room, harness, strict docs and actionlint. make generate cli-docs PASS.
Cursor Composer 2.5 non-fast implemented CI write protection; parent reviewed,
integrated and fixed the concrete candidate error guidance. Grok 4.7 low
non-fast completed the bounded upstream patch CLI lookup. A Grok 4.7 high
non-fast review stalled and was interrupted (exit 130): review NOT_RUN.

NOT_RUN: changed-patch/cache scenario, truly cold distributed runner, current
GitHub/GCP CI, runtime pull after credential expiry, full signature chain for
these images, current cloud application acceptance, outside-person guide trial,
new native macOS/Windows/Brew/Scoop tests and the 12-cell recipe matrix.
Humanizer applied to human prose; remove-ai-marks service is unavailable
(connection refused), so its pass is NOT_RUN. No cloud resources were created;
AWS spend/reservation remains zero and remaining credits unverified.
Private proofs are under /tmp/magelift-community-alpha; local sources/images and
cache remain. Local registry/server cleanup is recorded separately below.


Registry follow-up: second transport proof PASS (actual index/child/blob hashes,
SPDX/SLSA predicates); only application tags cold/warm and the separate cache
reference existed, with no builder/runtime repositories. Final registry/cache
scan PASS: 113 files, 36 gzip blobs, 1,779,028,191 decoded bytes, no fake Composer
or registry credentials. The exact owned registry container was stopped/removed
after checking its ID and acceptance owner label. The exact owned HTTPS Git
server was stopped (session exit 143 after SIGTERM). Both services are gone;
their private data and proof files remain. No shared Docker resources pruned.
Remaining local storage after these tests: 512 GiB workspace, 5.5 GiB /tmp.


### S0 synthetic overlay and S1 hotfix follow-up

Added an original reusable module/theme overlay under
tests/fixtures/magento-shop-overlay. Actual CLI default build from the isolated
shop passed, exit 0, revision f6c158a72a922dbd60d83feade3d831e9f70a1fb.
Image sha256:6f43c559035b43ef4b7f2bf0523674521be7813980ebca7c0ec6ea16faed126d;
manifest SHA256 c5e9554b49241c6601466e6e303d8d663578dc6b3dce6fb1a8864fb0c9a72580.
Running PHP inside the resulting image confirmed the custom theme CSS contains
.magelift-alpha-fixture and MageLift_AlphaDisabled remains 0. Manifest matrix
contains MageLift/alpha/en_US; source Git status is clean and config.php hash
unchanged. This proves build content, not HTTP/store/cloud behavior.

Hotfix errors now identify category/path/exit and a next action without echoing
patch stderr/source text. Native patch tests cover a conflicting hunk, full
reapplication and a genuinely partial two-hunk patch. The partial fixture first
applies successfully to pristine input, proving it is valid; after resetting to
partial content, failure leaves the target unchanged with no .rej/.orig files.
The parent corrected the worker's overlapping-hunk fixture before acceptance.
MAGELIFT_PHP_TEST_CONTAINER=1 make php-test PASS after that correction:
152 tests / 359 assertions, PHP 8.5.11, static analysis/audit PASS.
make docs check-clean-room PASS. Cursor Composer 2.5 non-fast supplied the
bounded implementation/tests; parent reviewed the source and reran the checks.
No claim of patch-series atomicity or protection against a write failure after
successful dry-run is made.

Real QPT execution is still pending: ordinary installation in a separate shop
failed because the package is not on the configured public registry (exit 2).
The explicit upstream VCS attempt without credentials then failed GitHub auth
(exit 1). A retry uses the existing GitHub login via a private read-only Composer
auth file mounted into the disposable installer; that file is removed on exit.
No upstream package source/catalog is added to MageLift, and no Adobe credentials
are invented or requested. These install failures are not passing patch tests.


## S1 decisive local acceptance / S2 initial integration — 2026-10-18

S1 real hotfix build passed with local BuildKit cache: the compiled custom-theme
CSS changed to #228844 through m2-hotfixes while source stayed #1255a3 and Git
status stayed clean. Image sha256:e604fc12dd3a5ee7d335cc5ceb6e214007fdfa7cf7cdf21a12ea89a6b15627cb;
manifest SHA256 f6c66195078e69b3bd105c85e46b9f28ceeb94b0964792d8d83bd77322595b33.
This changed-patch run used the local cache, not a registry cache.

QPT installation initially failed on missing repository, anonymous GitHub auth,
then a locked Symfony dependency conflict. The isolated fixture now explicitly
locks current upstream QPT1.1.84 and Cloud Patches1.1.14. Composer resolved the
latest compatible dependencies after unlocking http-kernel; config, DI and YAML
moved from8.1 to7.4, http-kernel from8.1.9 to8.0.15. No dependency constraint or
security advisory block was disabled; audit reported no advisories. These are
synthetic shop lock changes, not MageLift dependency changes. Private Composer
auth mounts were removed on exit. No upstream code/catalog was added to MageLift.

Actual upstream status recognized Magento2.4.9 and an unknown ID was rejected
(exit1). A real MageLift build with declared QPT AC-17975 passed; running the
installed upstream status command inside the image confirmed Applied. Source
was unchanged. Image sha256:7370b1b24d2dd4b3723406b56d4b959c8f1be7dcbaad773c4717dddf74c67f2f;
manifest SHA256773e62a2ef5e0dedcea3b0e8701e4529090714ba2b626b36a11ca790d3f5ed43.
The first test invocation had a duplicated Composer YAML mapping created by the
harness; it failed decoding (exit2). The fixture was corrected before the real
successful run, without changing the application validator.

Optional build.cloudPatches now defaults false and delegates one locked upstream
ECE apply when explicitly enabled. Its single pass owns required cloud patches,
YAML-declared QPT and project hotfixes. The parent caught a missing apply command
in the worker's first wrapper; stronger executable tests failed twice, then
passed after fixing argv and retaining constructor compatibility. Full local
community/docs/actionlint/generated checks passed, exit0, with160 PHP tests /
384 assertions. The user-facing scope notice stays on stderr; no separate
patch database or lifecycle engine was introduced.

Real Cloud Patches opt-in build passed. The built image reports AC-17975 Applied,
contains the upstream-applied #55aa66 theme CSS and preserves the disabled module.
The invalid ID deliberately left in .magento.env.yaml (present in the image)
did not control selection. Source config/theme/Git remained unchanged and stdout
was valid JSON. Image sha256:b80fd4c4caed2741f98da274d072669d40c8965efa5910c80e79e531dd5b6e38;
manifest SHA2568e7fb55d905ae3787857fdd237325f0f63ea73509498940ef3cb07123f77f295.

Fresh host-only GoReleaser2.18.3 snapshot packaging of commit58b5286 passed
(sign/publish/SBOM explicitly skipped for this local archive). The extracted
archive built a detached synthetic shop with a new empty docker-container
BuildKit0.34.0 store, no cache import and a host PATH containing only Git/Docker.
BuildKit was capped at6GiB and4CPU equivalents; Magento preparation runs in a
separate container, so this is not a measurement of total peak RAM. Build passed
in320.688s; the temporary builder, its container and state volume are gone.
Theme/hotfix/module content, snapshot version and recipe hash were checked in
the result. No MageLift source checkout, Go, PHP or Node was used by that CLI.
Image sha256:9eb6c9269e340470a2df560ce6afa734c3f12f3c0e3854ae9076bfbb04f416b2;
manifest SHA2567aed2fceee520cc9819fe0c9e484ce3cc79d82b761984aed44c651b2d120d13f.
Archive62,991,601 bytes; installed CLI308,441,248; provider207,556,768. These
sizes exclude tools, Docker images/cache and the rest of the checkout.

A streamed scan of all three QPT/Cloud/cold images passed:68 archive entries,
52 compressed blobs,2,377,025,893 decoded bytes; the actual build credential and
its base64 value were absent. This checks those encodings, not every possible
encoding of arbitrary extension data. The earlier fake canary/cache scans remain
separate evidence. These local app images were not pushed/signed; current remote
signature chain, GitHub CI and GCP runtime-pull acceptance remain NOT_RUN.

S2 initial integration delivered: source-inspecting init, default GCP single
test environment, explicit AWS starter, real source-checkout fallback, Commerce
edition precedence, regular-file/JSON/compatibility validation before writing,
nonsecret JSON result and preserved import flows. A real isolated init produced
one valid test environment without changing the shop. The minimal workflow has
validate/build/test jobs, retained-manifest handoff, serialized test deployment,
no production inputs or preview schedule. GCP build/test identities have separate
GitHub environment references; bootstrap provisioning is still being completed.

Google auth writes federation files in the checkout. Generated GCP jobs now move
them outside the shop, export only path references, and retain the auth action's
post cleanup. Executed Bash tests cover paths with spaces, clean Git, unchanged
credential content,0600 even from0644 input, invalid/symlink/outside paths,
RUNNER_TEMP inside checkout and failed env publication cleanup. Native PHP also
rejects known root gha-creds files before preparation. PHP checks pass with161
tests /388 assertions. CLI/skills suites and strict docs/actionlint/CLI docs pass.
The first complete S2 gate failed on filename code spans interpreted as YAML
schema keys by the skills check; formatting was fixed without weakening the
assertion, and the skills suite passed. Final combined S2 gates will be rerun
after the remaining bootstrap integration; no full S2 completion is claimed.

Signing now preserves explicit overrides, otherwise uses ambient GitHub OIDC
before gcloud; missing job OIDC permission fails with guidance. The command
callback tests prove no deploy-SA impersonation/token materialization in this
path. The real GitHub signing call is NOT_RUN. Cursor Composer2.5 non-fast
implemented the bounded init, credential and signer slices; parent corrected
real source handling, strict input checks, credential-mode/failure cleanup and
ambient detection against upstream provider semantics. Grok4.7-high again
produced no report in its bounded signing attempt and was interrupted (exit130),
so that review is NOT_RUN.

Storage:8.8GiB of owned private proofs/synthetic checkouts were moved from tmpfs
to /home/alex/.cache/magelift-community-alpha-20261018; the original /tmp path is
a private alias. Files were preserved. Remaining:490GiB workspace,11GiB /tmp.
Local images/caches remain; no shared Docker prune. No cloud resources created,
no external push/tag/publication, AWS reservation/spend still0, credits unmeasured.
Humanizer applied to prose; remove-ai-marks service remains unavailable/NOT_RUN.

Next useful work is guided account bootstrap: API/registry/state preparation,
owned scoped build/deploy/pull identities, derived CI settings and real acceptance
on the authorized GCP target. S3 functional/release/cleanup acceptance and the
outside-person trial remain pending. S4/S5 are still conditional pilot work.

## S2 increment: shop init, CI identity and managed GCP prerequisites

The CLI inspects the existing shop and generates one GCP `test` environment.
Project names now fit GCP planning limits, including numeric and long Composer
names. Generated CI separates build and test identities, relocates Google
credentials outside the shop and gives GitHub ambient OIDC precedence for
signing. Bootstrap resolves immutable GitHub repository/owner IDs before cloud
creation, pins the normal workflow claims, creates distinct build/deploy service
accounts, and preserves unrelated IAM conditions, etags and audit configuration.

GCP bootstrap now ensures missing APIs, the owned state bucket, an immutable
Secret Manager state passphrase at version 1 and the user's Docker Artifact
Registry repository. Key initialization reuses the deployment lock and existing
GCS adapter, checks archived object generations, refuses prior encrypted state
without its key and releases locks with an independent bounded context. Pulumi
receives backend URL and passphrase together in workspace-scoped environment
variables. Explicit passphrase/file overrides remain available for recovery.
No secret bytes enter bootstrap output or the provider RPC contract.

Executed locally with Go 1.27.2 and containerized PHP 8.5.11:

- `MAGELIFT_PHP_TEST_CONTAINER=1 make verify-community docs workflow-check cli-docs-check generate-check`: PASS (exit 0); root, SDK and GCP modules independently, bounded race gates, clean-room, lifecycle and development-loop harnesses, docs and generated references. PHP: 161 tests, 388 assertions.
- Additional GCP `go test -race ./bootstrap ./state ./plugin -count=1 -timeout=120s`: PASS.
- Targeted automation, GitHub client, CLI/skills and GCP bootstrap/state/ops/plugin/secrets suites: PASS.
- Native GCS HTTP fixture: PASS; request includes `versions=true` and a deleted checkpoint generation prevents treating state as absent. Review replaced the duplicate storage adapter and removed a test-global mutable Pulumi factory.

S2 remains open: CI project/resource role grants and GitHub variable setup are
not yet integrated. Real GCP IAM/CEL evaluation, remote-key decryption, GitHub
workflow execution and a new signed release/install remain NOT_RUN. No cloud
resource was created by this increment, no image/tag/release was published, and
no protected resource was destroyed. Cloud spend/reservation remains 0; AWS
credit coverage remains unverified. Proofs are retained in the private local
acceptance directory; its existing /tmp path resolves to disk storage. The
remove-ai-marks service remains unavailable (NOT_RUN).

## S2 increment: explicit CI grants and repository setup

Bootstrap now returns a permission plan before applying CI resource/project
roles. `--apply-ci-permissions` explicitly applies the reviewed plan;
`--configure-github` additionally creates allowlisted nonsecret repository
variables without overwriting existing values. Immutable repository/owner IDs
are checked again before variable writes. Build, deployment and node-pull
identities remain distinct. Project-level deployment roles cover the whole GCP
project; they are not restricted to resource prefixes. Prefer a dedicated test
project and review that scope before applying it.

Native IAM clients preserve unrelated bindings, conditions, audit configuration
and etags, reject unowned resources and a foreign project number, and reread
policies after a conflict. The node impersonation grant targets the exact
Compute Engine service account. Integration tests exercise the complete
bootstrap path: default planning never constructs the grants client, explicit
application ensures/closes it once, and an apply failure never returns success.

Executed checks:

- `MAGELIFT_PHP_TEST_CONTAINER=1 make verify-community docs workflow-check cli-docs-check generate-check`: PASS (exit 0), including independent root/SDK/GCP suites, bounded race checks, clean-room, acceptance harnesses, strict docs, actionlint and generated references. PHP: 161 tests, 388 assertions.
- Targeted CLI/platform/GitHub and GCP bootstrap/ops/plugin suites: PASS.
- CLI/GitHub/automation race suite: PASS.
- GCP bootstrap/ops/plugin race suite: PASS after fixing an unsynchronized counter in the existing concurrent-lock test fixture. The initial race run failed; its failure and the corrected run are retained separately.
- Additional complete ops integration race suite: PASS.
- Actual CLI init and config validation against clean Magento Open Source 2.4.9: PASS. Current compatible Composer dependencies were resolved with PHP 8.5.11 and Composer 2.10.3; the resolver reported no security advisories.

The maintainer authorized a private user acceptance repository. A clean official
Magento 2.4.9 source and its generated single-test configuration are committed
there. No upstream workflows were imported, and no Actions run has started.
The configured acceptance project is active with billing enabled; the dedicated
acceptance DNS name is absent and available. These are read checks, not proof
of cloud permissions, federation, TLS or a working deployment.

Real CI grants, GitHub variable writes, remote-state decryption, runtime image
pull and end-to-end workflow execution remain NOT_RUN. No GCP resource or DNS
record was created by this increment, no application image or MageLift release
was published, and no protected destruction occurred. The private source
repository remains for the authorized acceptance. AWS spend/reservation remains
0 and its credit coverage is unverified. Local proof storage has 479 GiB free;
/tmp has 11 GiB free. S2 acceptance and S3 functional/release/cleanup proof stay
open; S4/S5 do not block the alpha. Humanizer was applied to prose; the local
remove-ai-marks service remains unavailable (NOT_RUN).

## S3 increment: local doctor and voluntary diagnostic export

`doctor --local` now works without cloud credentials, including before a valid
configuration exists. It reports local tool checks and explicitly skips cloud
credential probes. `doctor --export NEW_FILE` writes a voluntary local JSON
metadata file with mode 0600 and refuses existing files and symlinks. It does
not upload anything. The default doctor behavior and invalid-input exit code
remain unchanged; local readiness failures exit 4.

The export uses a separate typed field allowlist. It includes known version,
OS/architecture, built-in target categories, check categories/statuses/counts
and known required/optional classifications. It never copies the rich console
report. Environment suffixes become a generic category; unknown targets, check
identifiers and arbitrary version/status strings are omitted or categorized.
Free-text diagnostics, config values, names, account IDs, paths, tokens, URLs,
raw logs and dumps are excluded. Raw extension logs remain a separate manual
review task; this export makes no claim to redact arbitrary log contents.

Executed checks:

- CLI and skills regression/race suites: PASS after review tightened categorical allowlists and fixed generation ordering. An intermediate strict-shape failure after adding the requirement enum and a stale skill digest failure were corrected; failed and corrected logs are retained separately.
- Actual stripped CLI build: PASS. With PATH unavailable, an empty HOME and nonexistent ADC, the missing-config local run exports metadata and exits 4; the valid GCP shop local run exits 0, reports five missing optional tools and one skipped credential check. Both exports are mode 0600 and omit the configured project, domain and path.
- Strict docs, generated references/manifest checks and actionlint: PASS.
- Linux/amd64 GoReleaser packaging smoke: PASS with the current CLI/provider source. Archive 63,022,545 bytes, installed CLI 308,506,784 bytes, provider 208,781,472 bytes, excluding tools and Docker images. This smoke skips signing, SBOM and publication; it is not a signed-install proof.
- Current upstream GoReleaser release is v2.18.3, verified through the official release API; CI and release workflows now use it consistently with local smoke.
- Bootstrap/ops race suite after switching newly added fixtures to a fictional project: PASS.

Cursor Composer 2.5 non-fast implemented the bounded doctor slice; parent review
added stricter categorical privacy checks and actual binary smoke. No framework,
provider protocol or additional manifest was introduced. Humanizer was applied
to prose; remove-ai-marks remains unavailable (NOT_RUN).

The new source is not yet distributed as a signed release. Actual GCP CI,
remote state, two functional deployments with persistence, runtime incident,
TLS/DNS and independent cleanup remain NOT_RUN. A new first-party signed
candidate is required to exercise this source through the normal fail-closed
provider installer; a local binary or custom signing identity cannot substitute.
Public tags/publication still require the maintainer's explicit approval. No
MageLift tag was created or pushed; no application image or cloud runtime was
published. The authorized private source repository and local proof/image caches
remain. No new GCP resources or DNS records exist from these increments. AWS
spend/reservation remains 0; credits remain unverified. macOS, Windows and the
outside-person trial are NOT_RUN. S4/S5 remain outside the alpha's critical path.

## Signed canary and first live bootstrap increment

The maintainer explicitly approved pushing the community-alpha branch and the
immutable canary tag for commit ce1b74196039f60171ad3049b95d005d181b54f2. Its
existing release workflow completed successfully (run 39083042658). The
Linux/amd64 candidate was published as a prerelease with latest disabled after
independent verification of both artifact SHA-256 values, the checksum and
provider Sigstore bundles against the exact tag workflow identity/issuer, and
the extracted CLI version. No central application/builder/runtime image was
published. Release archive: 63,025,777 bytes; installed CLI: 308,510,880 bytes;
provider: 208,777,376 bytes.

The installer fetched from that immutable public tag passed without host Go,
PHP, Node or Cosign (4.653 seconds). Provider installation from a fresh cache
passed without those tools (5.424 seconds, cached=false, bundled=false), including
pinned Cosign 3.1.3 acquisition. The hosted website installer is still the older
implementation; its publication is pending and the release notes identify the
versioned installer. Generated CI verifies the release assets directly.

The authorized private acceptance fixture now carries official Magento Open
Source 2.4.9 CE isolated security fixes for July through October 2026 plus the
separate CVE-2026-75650 fix. A second clean source copy passed all five dry-runs
and applications with fuzz=0 and no ignored hunks. All 74 runtime source files
match that strict result; PHP 8.5.11 syntax checks passed for 58 files. A
reapplication-generated duplicate nginx rule was removed by comparing the clean
strict result. The upstream CVT matrix is NOT_RUN for the GitHub-source lock
layout; no CVT protection claim is made. Official download hashes/proofs stay
private. No upstream patch catalog or engine was copied into MageLift.

Two real bootstrap commands succeeded on the authorized GCP acceptance target.
They returned the same backend, immutable state-key version, CI variables and
24-binding permission plan. CI permissions were not applied. The second run
took 14.840 seconds. A harness used backendUrl instead of backendURL after the
first command had already succeeded; it was corrected and the saved native
results were compared. Initial invocation of local build with a push-only image
flag was correctly refused; the documented local build is running separately.
No runtime or DNS record has been created. The acceptance state bucket, state
secret, empty user registry and owned federation identities now remain for the
ongoing run; billing amounts are not yet measured. AWS remains unused.

A read-only project IAM preflight found an actual client defect: Resource
Manager v1 expects a bare project ID and the client added a duplicate projects/
prefix, producing HTTP 400. Exact native-URL tests reproduce failures for both
get and set before the fix. They pass with bootstrap/ops/plugin race suites
after the fix, and a real read-only policy request succeeds. No project IAM
write was performed by this check. The published canary still contains the
old client; the fix requires a new immutable signed candidate before CI grant
application can be tested. Functional deployments, persistence, runtime pull,
incident handling and destruction remain NOT_RUN.

### S2 increment: owned GKE node identity

Read-only acceptance IAM inspection found broad preexisting privileges on the
shared default Compute account. The implementation now derives an owned node
account, prepares repository reader and the minimum GKE node project role, and
selects it explicitly for new clusters. Existing immutable node identity fields
are ignored on updates to avoid implicit replacement. Default-account policies
are untouched. The CI grant plan validates the owned node before applying grants.

Targeted bootstrap, naming, ops, runtime, stack and plugin tests pass, including
bounded race runs. One initial test fixture lacked the new injected factory and
attempted a read against a fictional project (403); the fixture was corrected
and all six packages pass. No cloud mutation resulted from that fixture. Live
node creation, pull identity, legacy migration and CI grants remain NOT_RUN.
The existing acceptance prerequisites remain; no node account, runtime or DNS
record has yet been created by this increment. Billing remains unmeasured.

The signed installed CLI also built the patched private Magento fixture locally
in 312.366 seconds without host Go, PHP or Node. The image was not pushed.
Existing BuildKit cache was available, so this is not a cold-build measurement.

Independent Cursor Composer 2.5 review (non-fast, read-only) found no concrete
regression in the bounded node-identity diff. Host-only Linux packaging passed
(31 seconds): archive 63,024,864 bytes, installed CLI 308,506,784 bytes, provider
208,801,952 bytes; signing/publication were deliberately skipped locally.
Human prose was reviewed with humanizer. remove-ai-marks is NOT_RUN because its
local HTTP service is unavailable. The maintainer approved the exact 24-binding
acceptance IAM plan on the original GCP target; application is still pending.
Cleanup of all owned test resources and only this run's added IAM bindings is
required after acceptance, with state/key retention until teardown completes.

The complete MAGELIFT_PHP_TEST_CONTAINER=1 make verify-community docs
workflow-check cli-docs-check generate-check invocation passed after integration:
all three Go modules, bounded race checks, PHP, lifecycle/clean-room/helper
gates, strict MkDocs, actionlint and generated-file consistency. The read-only
24-binding IAM baseline found no preexisting matching unconditional or
conditional member bindings; the private receipt is retained for scoped cleanup.
The maintainer prefers an organization-owned acceptance repository. Its final
visibility and federation update are pending; the prior personal-repository
permission approval is not silently extended to a different repository.

### S2 increment: organization acceptance and test-only dispatch

The maintainer authorized a public synthetic fixture in the MageLift organization.
The source snapshot preserves upstream licenses and public patch provenance;
41,603 files were checked against actual credential values and private target
identifiers with no matches. No private history or vendor cache was exported.
The public configuration uses placeholders. Actual target YAML is supplied as
a nonsecret repository variable by an acceptance-only workflow customization.
Build/test environments exist with main-only deployment branch policies. The
organization's Free plan does not support private-repository environments.
The revised 24-binding plan for the organization repository is approved.

Actionlint on the real single-test generated workflow found an undefined
inputs.environment reference in its build condition. The generator now accepts
push or manual dispatch without inputs for that mode, while multi-environment
production selection is unchanged. The strengthened regression rejects any
inputs reference in test-only output. Targeted workflow tests and the complete
CLI package pass; the corrected actual workflow passes actionlint. This fix is
newer than the second approved canary, whose fixture workflow is customized
accordingly; no tag is moved. Cloud CI/runtime acceptance remains NOT_RUN.

The organization IAM baseline attempt encountered a 403 when requesting policy
on an absent registry. This is not recorded as a successful policy read or as
proof of missing project permission. The baseline must be completed after
owned prerequisite creation and before CI grants, accounting separately for
the newly added node baseline grants. Cleanup remains required for both the
unused personal-repository prerequisites and the active organization run.

### S2 increment: signed candidate and real organization bootstrap

The second approved immutable canary completed its release workflow successfully.
Independent verification passed both SHA-256 checks, both Sigstore bundles with
the exact tag workflow identity/issuer, and the extracted CLI version before
publication as a non-latest prerelease. Archive: 63,021,155 bytes; CLI:
308,510,880 bytes; provider: 208,801,952 bytes. Installation from the immutable
tag passed in 5.134 seconds and fresh provider installation in 5.139 seconds,
without host Go/PHP/Node/Cosign. The public fixture now pins the verified lock.

Real signed-CLI base bootstrap passed in 53.738 seconds. The complete 24-binding
policy baseline then passed: only the two node bindings added by this base
preparation were present; no conditional matches. Applying CI permissions and
configuring GitHub variables passed in 37.005 seconds. Repeating that command
passed in 30.743 seconds, with identical backend, key reference, permission plan
and CI variables. The earlier absent-registry 403 is superseded by this real
policy read, not suppressed. Cleanup records distinguish the two base-added
bindings from foreign preexisting policy.

Using the installed binary also reproduced a CI version normalization defect:
GoReleaser's installed version lacks v while generation required it. Explicit
--magelift-version with the exact canary passes on the distributed binary. The
source now normalizes either spelling to the immutable tag; full CLI tests
pass, including stable/prerelease/canary equivalence regressions. The second
canary remains immutable and requires that explicit flag. The customized user
workflow passes actionlint and ci validate and has started its real GitHub run.
No runtime or DNS record exists yet; functional deployment remains NOT_RUN.
The maintainer has granted autonomous GCP decisions and reiterated complete
owned-resource cleanup after confirmed tests. Existing shared resources and
preexisting IAM remain outside cleanup scope.

### S2 increment: real CI cleanliness failure

The first organization Actions run passed validation and the actual build
identity's Google federation/registry authentication, then stopped before image
creation: the acceptance customization rewrote tracked YAML, and the generated
build step redirected stdout to an untracked checkout file before the CLI's
source cleanliness check. No runtime was deployed. Both failures are retained
in the private run log; the clean-source protection is unchanged.

Acceptance YAML now lives in an ignored root configuration file; the tracked
placeholder stays intact. Local validation of the actual ignored configuration
passed with an empty git status. Generated build results now live directly
under RUNNER_TEMP before invoking the CLI. A Cursor Composer 2.5 worker
(non-fast) added a shell-executed clean-Git regression with a fake builder,
checksum verification and handoff checks. Targeted tests and full CLI/source
suites pass. Renderer fixtures were refreshed through their update test;
workflow lint and generated consistency pass. The two older example workflows
received the same output-location correction. The second real Actions run is
in progress. Functional GCP, image publication and runtime pull remain NOT_RUN
until that run supplies evidence. Bootstrap resources and CI grants remain for
the active tests and must be removed afterward.

### S2 increment: repository image tagging

The second real CI run passed the clean-source check and then refused the
bootstrap image destination because it had no explicit tag. The generator now
appends the validated GitHub source SHA to an untagged repository and preserves
an explicit tag. Shell-executed tests cover both forms, registry ports, missing
SHA and invalid SHA, with invalid inputs stopped before builder invocation.
The fake builder checks the actual image argument and emits valid JSON. The
Cursor worker stopped with a looping error without edits; root completed the
bounded change and its regression locally. Targeted tests pass. The third real
Actions run has reached the actual build step. The published canary's generator
still needs these reviewed workflow corrections; its build command protections
are retained. No runtime or DNS has yet been created.
The full CLI package, refreshed renderer fixtures, workflow lint, generated
consistency and strict docs also pass for the image-tag correction.

### S2/S3 increment: signed application image and canonical handoff

The third organization run built, pushed and signed the actual application
image in 250 seconds (build step start/end timestamps). Its manifest artifact
was downloaded independently: checksum, image digest and source revision match.
Cosign independently verified the image against the exact organization workflow
main identity and GitHub issuer, using temporary registry credentials removed
after verification. This proves user-registry publication, not runtime pull.

The deploy job then refused tag@digest at provider-plan validation, before
runtime creation. The existing GCP artifact contract requires repository@digest.
The generator and its handoff verification now remove only a trailing image
tag; registry ports are preserved and the image digest is unchanged. Shell
regressions cover canonical output and the existing corruption/different-digest
refusals. The fourth Actions run uses that correction. No provider contract,
manifest or source cleanliness protection was changed.

The complete community/docs/workflow/generated gate passed on the preceding
image-tag correction, including 161 PHP tests and 388 assertions. Canonical
handoff has its own targeted/full CLI and workflow consistency checks. Current
resources include the owned bootstrap prerequisites, added CI grants and an
actual acceptance image; runtime, DNS, persistence and destruction remain
NOT_RUN. All owned cloud resources still require final cleanup.
