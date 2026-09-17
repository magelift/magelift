# Review of alpha-review-corrections

Reviewed 2026-09-17. Baseline: `90e0dc120dee9f042d1bc70374c4207e6c789cbe`.
Reviewed working-tree HEAD: `f384522ebfd553d3a1089e594da87c9b12d9392d`.
Release candidate `v0.1.0-alpha.1-rc.2`, SDK tag, and GCP provider tag all point to
`a6d4d882a74fceb441fd850d760738a6794a2fb0`, before the require-bump commit.

**Verdict: keep this intent open. The direction improves the project, but the claim that only 1.4 and 5.1 remain is incorrect.**

There are independently reproducible failures before cloud acceptance: release-version
provider bootstrap, the application image build guard, and published provider module
resolution. The media IAM binding also contradicts Google's documented API constraints.
Fix these before spending another live run discovering them. Do not archive this intent
or treat an rc.2 CLI release as evidence that R01-R12 are resolved.

This review changes only this report. It does not alter source, tags, workflows, cloud
resources, the plan, or the roadmap. Local regression probes used Go overlays outside
the repository. References below are repository-relative, with reviewed line numbers.

**Findings**

1. **P1: the released CLI cannot bootstrap providers using its default version.**

   `internal/cli/providers.go:55-60` passes `Version` straight to `ValidReleaseTag`.
   `.goreleaser.yaml:20` embeds `{{ .Version }}`, which omits the leading `v`.
   `internal/providerhost/fetch.go:27` requires that prefix. On a clean installation,
   the documented `magelift providers install` therefore rejects
   `0.1.0-alpha.1-rc.2` before attempting to fetch its lockfile.

   Reproduced with the real CLI command constructor, a release-shaped `Version`,
   an empty project cache, and a fetch spy: the fetch is never reached. Existing
   positive bootstrap coverage supplies `--version v9.9.9`, hiding the mismatch.
   Normalize embedded versions into release tags at the boundary and test the
   default command with the exact GoReleaser version format. Reopen 1.2.
   [GoReleaser documents the distinction](https://goreleaser.com/customization/general/templates/).

2. **P1: the media template makes application image builds fail.**

   `images/php-nginx/env.php:119-136` introduces `MAGELIFT_MEDIA_*` placeholders.
   The application Dockerfile restores this exact runtime template, then requires
   `! grep -Eiq 'MAGELIFT' app/etc/env.php`
   (`internal/build/pipeline/assets/application.Dockerfile:22-29`). The pipeline
   writes that embedded Dockerfile unchanged (`internal/build/pipeline/pipeline.go:174`).

   Running the same guard against the new template returns exit 1. A shop build
   using the corrected runtime image cannot complete. Using an older image would
   omit the media correction. Reconcile the template contract and validation;
   retain meaningful secret/unresolved-template checks, and exercise the actual
   generated application Dockerfile against the supported runtime template.
   Reopen 3.1 and include this check before 1.4.

3. **P1: the new public-media IAM binding is invalid.**

   `providers/gcp/storage/storage.go:107-118` grants `roles/storage.objectViewer`
   to `allUsers` with a prefix condition. Google explicitly disallows conditions
   on bindings to `allUsers` and `allAuthenticatedUsers`. Pulumi resource mocks
   accept this shape, but the real IAM API does not.

   Choose a supported delivery mechanism and reconcile it with the spec's private
   bucket requirement. Simply removing the condition exposes the entire bucket
   and is not an equivalent fix. Keep private media and public assets distinct
   where the application requires it. Reopen 3.1; unit registration tests cannot
   close this issue.
   [Google IAM Conditions documentation](https://docs.cloud.google.com/iam/docs/conditions-overview).

4. **P1: the published rc.2 provider module still lacks its required MageLift modules.**

   `6a64de4` adds root/SDK requirements after all rc.2 tags were created.
   `providers/gcp/go.mod:21-22` at HEAD therefore describes a different module
   manifest from the published rc.2 manifest. Testing HEAD with rc.2 dependencies
   does not prove that consumers can build the rc.2 provider itself.

   I downloaded `github.com/magelift/magelift/providers/gcp@v0.1.0-alpha.1-rc.2`
   through the Go module tooling, then ran `GOWORK=off go list -mod=readonly -deps`
   on its real command package. It fails to resolve root-internal imports and
   `github.com/magelift/magelift/sdk`, and reports missing transitive sums.
   Its downloaded origin is `a6d4d88`, confirming the tag/HEAD distinction.

   The synthetic distribution suite passes, but it edits requirements and runs
   tidy on staged copies (`tests/distribution/distribution_test.go:254-299`).
   That proves the repaired shape can compile, not that this tag publishes it.
   Reopen 2.5. Publish a new immutable candidate containing the corrected
   manifests and validate the actual downloaded provider at that version without
   editing its manifest. Do not move rc.2 tags.

5. **P1: media transfer can read or overwrite files outside the selected directory.**

   `providers/gcp/plugin/media.go:192-204` walks source entries and uploads every
   non-directory, including symlinks. The production uploader uses `os.Open`,
   following the link. A symlink under the selected media tree can upload an
   unrelated local file. Export similarly joins paths beneath a directory and
   opens them with truncation, following existing file or directory symlinks
   (`media.go:78-81,168-180`).

   Two regression probes reproduced both directions using private fixture files:
   import uploaded a symlink target outside the source; export overwrote a file
   outside its destination. No real credentials or cloud access were involved.
   Enforce containment at file-open time, reject unsupported non-regular entries,
   and fail explicitly on unsafe names. Test parent-directory links as well as
   file links. Also replace the blanket `strings.Contains(relative, "..")` skip:
   it silently omits legitimate names such as `photo..jpg` while reporting success.

6. **P2: provider installation overwrites invalid project locks instead of failing closed.**

   `internal/cli/providers.go:49-79` treats every auto-discovery error as a missing
   lock. An existing project lock with malformed JSON or a rejected publisher
   goes through bootstrap and is overwritten with the running CLI's release lock.
   Only an explicitly supplied `--lockfile` preserves the error.

   This silently changes project pins and contradicts the resolver's own
   fail-closed contract. Distinguish an absent lock from read, parse, and trust
   validation errors; bootstrap only on absence. Add command-level tests asserting
   that invalid existing locks are neither replaced nor bypassed. Reopen 1.2.

7. **P2: custom provider caches remain disconnected between installation and execution.**

   Installation calls `EffectiveCacheDir`, honoring `MAGELIFT_PROVIDER_CACHE_DIR`
   (`internal/cli/providers.go:83`), but `Resolve` falls straight back to
   `DefaultCacheDir` (`internal/providerhost/resolve.go:66-73`). Registry execution
   supplies no cache override. The verifier lookup does honor the environment,
   so even the binary and verifier searches disagree.

   A regression test installed a fixture in the environment-selected cache and
   called the production resolver with default options. It returned a cache miss.
   Also, discovery of a beside-CLI lock unconditionally requires a beside-CLI
   binary, even after `providers install` downloads that lock's artifact to cache.
   Use one effective cache policy and implement the specified cache fallback.
   Reopen 1.2; test install followed by load, not only the two helpers separately.

8. **P2: nested-provider CI still misses changes to code the provider imports.**

   `.github/workflows/ci.yml:71-79` triggers provider verification for provider,
   SDK, and workspace files, but excludes root `internal/**`, root `go.mod` and
   `go.sum`, and the workflow itself. The provider still imports substantial root
   implementation, including the newly reused kube/platform helpers.

   A change to those helpers can alter provider behavior while `provider-verify`
   is skipped. The root `go test ./...` does not run nested-module tests, and the
   release gate accepts skipped checks (`scripts/release-checks-gate.sh:100`).
   Until independence is achieved, propagate the real dependency graph into the
   filters or run the provider job whenever root Go changes. Require actual
   successful verification for a release candidate. Reopen 4.1.

9. **P2: the checked provider-boundary plan contradicts both the code and its new ADR.**

   Plan 2.1 is checked and promises provider-local helpers and a clean dependency
   graph in both directions. The spec's R05 section requires no root-internal
   imports and a default core without deferred-provider packages/SDKs.
   `providers/gcp/plugin/deploy.go:8-9` still imports root kube/platform code;
   other provider packages retain root implementation imports too.

   `internal/registry/registry.go:8-11` imports AWS, EKS, OVH, and Scaleway.
   `NewDefault` registers AWS ECS, and the experimental switch is a runtime
   selection: it does not remove those dependencies from the binary.

   ADR 0013 now honestly defers full independence until AWS parity. That can be
   an explicit alpha tradeoff, but it does not fulfill the accepted spec. Update
   the spec, plan, roadmap, and deferred intent consistently, or complete the
   agreed boundary now. My recommendation for the shortest credible alpha is to
   retain the provider-owned execution improvement, disclose the coupling, and
   make its removal a hard prerequisite to adding a second autonomous provider.
   Do not describe the current core as SDK-free or this provider as SDK-only.

10. **P2: onboarding still cannot be executed literally from a clean installation.**

    `docs/onboarding.md:120-130` runs Docker builds against
    `images/php-nginx/Dockerfile` in the current directory. The preceding steps
    install a release CLI and initialize shop configuration; none obtains a
    pinned MageLift source tree. The release archive does not include that
    Dockerfile or the `build/` sources it copies. The following step then expects
    a separate clean Magento source tree without an explicit directory transition.

    Either publish the recipe's base images or document the pinned source
    acquisition, prerequisites, registry creation/authentication, working
    directories, and transition to the Magento tree. A maintainer silently
    cloning the repository on the acceptance VM would not prove this page.
    Reopen 4.2 and let 1.4 test the corrected page literally.

**Release gate observations**

At inspection, release run `35255044081` was still in progress at `a6d4d88`.
The check-runs API for that exact commit returned the running release and a
skipped image job; none of `lint`, `go-verify`, `sdk-verify`, `provider-verify`,
`floci-gcp`, or `php` was present. The new publication gate requires those names.
If the check set stays unchanged, publication will be blocked after packaging.
CI checks on the later HEAD do not satisfy a gate on the tag's commit.

The gate also has a regression outside its current fixtures: given an older
successful run followed by a newer in-progress run of the same name, it chooses
the completed success and exits 0. I reproduced this through `--checks-json`.
Selection by completion time is not selection of the latest attempt; use run
identity/start ordering and block pending attempts. The workflow should declare
`checks: read` explicitly. I did not test its restricted GITHUB_TOKEN and do not
claim an observed permissions failure in this public repository.

**What improved**

The deploy phases now execute through a typed provider operation, moving actual
GCP behavior out of the host. Production provider construction installs the ADC
client factory; saved kubeconfig bearer tokens are cleared and normal Kubernetes
requests use refreshing credentials. The exec path also obtains a fresh token.
This is materially better, while expiry and restart behavior still need the
planned live evidence.

Mutating operations now receive non-retryable timeout policy. The updater copies
its backup before replacing the executable, closing the missing-executable crash
window. The PHP-generated lifecycle sequence plus drift gate is a reasonable
single source of truth. The HTTP serving probe and effective search configuration
checks are stronger than scheduler-only success. These changes should be kept.

The new ADR and operational limitations are more candid. Incompatible production
migrations remain operator-controlled; a maintenance acknowledgement does not
prove traffic or writers were drained. That limitation must stay visible in the
alpha claim and acceptance scope.

**Verification performed**

Commands were run serially with `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB`.

- Root batch: providerhost, upgrade, platform, cloud/kube, distribution:
  **298 tests passed across 5 packages**, including synthetic outside-workspace
  provider compilation.
- Second batch: CLI, registry, SDK, GCP auth/plugin/storage/stack/provider main:
  **569 tests passed across 8 packages**.
- Existing `tests/acceptance/release_checks_gate_test.sh`: passed.
- Four added temporary overlay regression probes: **all four failed**, reproducing
  release-version bootstrap, custom-cache lookup, import symlink disclosure, and
  export symlink overwrite. These are defect evidence, not successful tests.
- Exact application image template guard: exit 1 against the updated `env.php`.
- Actual downloaded rc.2 provider dependency resolution, `GOWORK=off` and
  `-mod=readonly`: failed with missing MageLift module requirements and sums.
- Release-gate duplicate-attempt fixture: incorrectly passed an in-progress rerun.
- Read-only Git tag, module-proxy, release-run, and check-run inspection completed.
- PHP tests could not run: `php` is not installed on this devbox. No live cloud,
  Docker build, full local-gates run, or post-expiry acceptance was performed.

**Recommended sequence**

1. Reopen the affected boxes. Fix version bootstrap, the build guard, IAM delivery,
   filesystem containment, resolver behavior, and the CI dependency filters.
2. Reconcile R05 explicitly; do not spend the clean-machine proof attempting to
   demonstrate an architecture the ADR now defers. Correct the onboarding page.
3. Prepare a new candidate with publishable module manifests already committed.
   Run checks on its exact commit. Verify the real published modules, then the
   packaged default install/provider flow. Keep rc.2 immutable.
4. Run 1.4 against that candidate and its pinned image inputs. Record each
   workstation prerequisite. Verify a real Magento media upload, storefront
   retrieval, replacement pod, and export/import round trip; object registration
   and file counts alone do not establish media persistence.
5. Close 5.1 only with an accurate report that separates proven fixes, explicit
   deferrals, and evidence still owned by Order 8. Then rerun Order 8 against the
   same candidate and artifacts before a release-readiness verdict.

The remaining live checks are substantial: real credential expiry, PHP/upstream
failure detection, intended rollout behavior, public HTTPS/DNS, Magento media
semantics, encryption-key continuity, recovery, preview expiry, and generated CI.
Their pending status is expected. The deterministic failures above are reasons
to repair the candidate before those checks, rather than wait for them to fail.
