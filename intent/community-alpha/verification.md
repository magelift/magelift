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
