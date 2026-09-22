---
status: planned
slug: alpha-development-loop
spec: spec.md
---

# Plan: alpha development loop

## Files that change

- `internal/providerhost/discover.go` — edit: `CachedDialer.Close` reaps the cached plugin and drops it so a later dial starts fresh.
- `internal/providerhost/client_test.go` — edit: live test that `Close` reaps the GCP provider process.
- `internal/platform/module.go` — edit: `RegisterCloser` and idempotent `Close`.
- `internal/registry/registry.go` — edit: the GCP shim dialer is registered as a module closer.
- `internal/registry/hooks.go` — edit: hook dialer is exposed as `Hooks.Close`.
- `internal/cli/root.go` — edit: `Hooks.Close` is registered on the module registry; the root command records that closer.
- `internal/cli/execute.go` — edit: `Execute` reaps recorded provider closers on success, error, and cancellation.
- `internal/cli/execute_test.go` — new: success, error, and cancelled-context cases call both closers.
- `.github/workflows/release.yml` — edit: `v0.0.0-dialproof` and `v0.0.0-canary.` share the slim lane; lock upload goes through the canary-safe script.
- `scripts/release-upload-assets.sh` — new: non-canary uploads still use `--clobber`; canary uploads reuse an identical file and reject a different digest.
- `tests/acceptance/release_canary_predicate_test.sh` — new: workflow predicate plus upload script cases.
- `scripts/development-loop/lib-canary-identity.sh` — new: `v0.0.0-canary.<40 lowercase hex>` only.
- `scripts/development-loop/lib-gates.sh` — new: independent `PASS` / `FAIL` / `SKIP` records.
- `tests/acceptance/development_loop_gate_test.sh` — new: identity and non-rewriting gate records.
- `Makefile` — edit: those two shell tests run inside `acceptance-harness-test`.
- `scripts/development-loop/` runners for distribution, artifact, GCP product, and rc.22 capture, classify, and cleanup. Not started.
- `.github/workflows/alpha-development-loop.yml` — new: `workflow_dispatch` runner. It does not create, push, undraft, or delete the canary ref. Not started.
- `docs/evidence/` — new proof pages and a sealed run, plus a README link. Not started. `docs/capability-matrix.md` stays unchanged.
- One regression test and its fix — only after rc.22 classification names a single cause. Omit both if the evidence stays unresolved.

`internal/cli/subprocess.go` stays as it is. `defaultDialProvider` already tracks those sessions. The leak was the two `CachedDialer`s, which never joined that list.

## Order of work

- [x] 1.1 Add `TestCachedDialerCloseReapsPlugin` — verify: `go test ./internal/providerhost/ -run TestCachedDialerCloseReapsPlugin -count=1` failed with `provider process still running after CachedDialer.Close` while `Close` was a no-op.
- [x] 1.2 Implement `CachedDialer.Close` — verify: the same test passed, along with `TestCachedDialerDialsOnce`, `TestDialNegotiatesLivePlugin`, and `TestCallCancelKillsAndFails`.
- [x] 1.3 Reap both dialers from `Execute` on success, error, and an already-cancelled context — verify: `go test ./internal/cli/ -run TestExecuteClosesProviders -count=1` passed (4 tests).
- [x] 1.4 Keep registry and platform tests green — verify: `go test ./internal/registry/ ./internal/platform/ -count=1` passed (104 tests).
- [x] 3.1 Share the dialproof lane with `v0.0.0-canary.` in `release.yml` — verify: `make workflow-check` passed.
- [x] 3.2 Canary lock upload does not use `--clobber`; an identical file is kept and a different digest fails — verify: `bash tests/acceptance/release_canary_predicate_test.sh` passed.
- [x] 3.3 Ordinary non-canary upload still uses `--clobber` — verify: the same script's `v0.0.0-dialproof.1` case passed.

- [x] 2.1 Add `scripts/development-loop/lib-canary-identity.sh` and `lib-gates.sh` — verify: `bash -n` on both passed, and `bash tests/acceptance/development_loop_gate_test.sh` accepted only `v0.0.0-canary.` plus 40 lowercase hex characters.
- [x] 2.2 One gate `FAIL` does not rewrite another gate's status; a missing artifact is `SKIP`, not a copied `FAIL` — verify: the same test passed. Both tests are wired into `make acceptance-harness-test`.
- [x] 4.1 Distribution gate stages draft assets and runs `website/public/install.sh` with `MAGELIFT_RELEASE_BASE`, then the existing provider verifier — verify: `bash -n` and a fixture run that does not download from the public release URL.
- [x] 4.2 Installer fail-closed paths still pass — verify: `make installer-harness-test`.
- [x] 5.1 Artifact gate runs the existing certification path and the manifest tests. Host `php` 8.5.4 lacks `dom`, `mbstring`, and `xmlwriter`, and sudo cannot install them, so `composer test --working-dir=build` cannot run on this host. The same PHPUnit file passed in `php:8.5-fpm-trixie`: `docker run --rm -v "$PWD/build:/app" -w /app --entrypoint php php:8.5-fpm-trixie vendor/bin/phpunit tests/Artifact/ArtifactManifestTest.php` (13 tests). `go test ./internal/certification/ -count=1` passed in this session (282 tests). No second manifest was added. `artifact-gate.sh` runs that PHPUnit file in `php:8.5-fpm-trixie` when the host is missing the extensions; a fixture run recorded `artifact` `PASS`.
- [x] 6.1 GCP product gate delegates to `providers/gcp/scripts/gcp-acceptance-local.sh` with a unique ownership prefix and the immutable digest — verify: `MAGELIFT_ACCEPTANCE_DRY_RUN=1` on that wrapper and `bash tests/acceptance/gcp_harness_shape_test.sh`.
- [x] 6.2 `run-gates.sh` runs the three gates without fail-fast and writes an unsealed candidate through `scripts/acceptance/lib-evidence.sh` — verify: `bash tests/acceptance/development_loop_gate_test.sh` passed after the Docker PHPUnit path (`development_loop_gate_test OK`, installer verified the fixture canary tag) and `bash tests/acceptance/shared_evidence_test.sh` passed earlier.
- [x] 7.1 `rc22-capture.sh` collects the spec's log and request set and does not change topology — verify: `bash -n`. Live capture runs only against the retained rc.22 resources in the acceptance project.
- [x] 7.2 Run that capture — verify: `.magelift/development-loop/rc22-diagnosis.json` records the retained proof identities and a redacted SQLSTATE excerpt, with no secret values, before any destroy.
- [x] 7.3 Classify that record — verify: the file's cause is `migration`. Import failed because `magento.flag` does not exist; the image started, Cloud SQL was RUNNABLE, and both in-cluster and ingress HTTP were 500, so the other classes are not the distinguishing boundary. The resources are labeled `rc19-proof` / `alpha-review-corrections-1-4` on the proof VM the 2026-09-22 report left running. No rc.22 version string was in cloud metadata.
- [x] 8.1 Add the regression that setup:upgrade precedes app:config:import — verify: `testDeployCreatesSchemaBeforeConfigImport` failed with `Failed asserting that 1 is less than 0` before the plan changed.
- [x] 8.2 Put setup:upgrade before app:config:import and regenerate the Go golden — verify: the same PHPUnit file passed (13 tests) in `php:8.5-fpm-trixie`, `php build/bin/magelift-lifecycle-export | diff -u internal/platform/testdata/lifecycle-deploy.json -` was empty, and `go test ./internal/platform/ -run 'TestMagentoMigrationShell|TestMagentoProbe' -count=1` passed. The migration-shell test now fails if `app:config:import` precedes `setup:upgrade`. `go test ./providers/gcp/operations/ -run TestRegisterCandidateUsesPlatformMigrationContract -count=1` passed against that shell. A workspace build of `dist/magelift-provider-gcp` contains `setup:upgrade` before `app:config:import`.
- [x] 9.1 After the diagnosis record is durable, destroy the retained proof cell — verify: `gcloud compute networks peerings delete servicenetworking-googleapis-com` removed the peering that `gcloud services vpc-peerings delete` refused. The reserved range and `rc19-proof-preview-net` then deleted. A sweep of clusters, instances, SQL, Valkey, networks, addresses, and service accounts named `rc19-proof` returned empty. `mldp7-*` resources and the unlabeled bucket `magelift-digital-lab-341608-europe-wes-fb33d83727-preview-state` were left. Recorded in `docs/evidence/gcp-rc19-proof-http500-20260922.md`.
- [x] 9.2 Unowned resources in the acceptance project are not deleted and are not counted as rc.22 residuals — verify: `.magelift/development-loop/rc22-cleanup.txt` lists `mldp7-*` separately and those resources were not deleted. The diagnosis is also in `docs/evidence/gcp-rc19-proof-http500-20260922.md`.
- [x] 10.1 Add the dispatch workflow that fetches the draft canary assets and runs the three gates independently — verify: `make workflow-check`.
- [x] 10.2 The workflow and runner headers state that pushing and deleting `v0.0.0-canary.<sha>` are operator actions — verify: `bash tests/acceptance/release_canary_predicate_test.sh` still passes and the workflow file contains no tag push.
- [x] 11.1 Push `v0.0.0-canary.e840adff5dd5ddfe5f165b2a0d44584a90d4c14d` — verify: GitHub Actions run 35760954723 succeeded. `gh release view` shows `isDraft: true`, `isPrerelease: true`. The slim goreleaser lane ran; module wait, full matrix, attestation, module publish, undraft, and cask were skipped. Provider bundles and checksums verified in the workflow. A local distribution gate against those assets recorded `distribution` `PASS` after the installer printed `Sigstore bundle verified (release.yml @ v0.0.0-canary.e840adff5dd5ddfe5f165b2a0d44584a90d4c14d)` and `version: 0.0.0-canary.e840adff5dd5ddfe5f165b2a0d44584a90d4c14d`.
- [ ] 11.2 Run the gates against that draft and the acceptance-project image digest — verify: three independent gate records.
- [ ] 12.1 Live GCP product gate in the dedicated acceptance project, destroy on exit, no KEEP unless the spec's authorization record exists — verify: the acceptance harness exits clean.
- [ ] 12.2 Seal the candidate — verify: `magelift certification seal` writes `docs/evidence/runs/`, `magelift certification verify` accepts it, and `make check-clean-room` passes.
- [ ] 13.1 Write the development-loop proof and link it from `docs/evidence/README.md` without editing `docs/capability-matrix.md` — verify: `make docs`.
- [ ] 13.2 Full local suite — verify: `make verify`. Partial, not this box: `make generate-check`, `make cli-docs-check`, `make lint` (0 issues), `make license-check`, `make docs`, and `make check-clean-room` passed. Build PHPUnit (133 tests), PHPStan, and Psalm passed in `php:8.5-fpm-trixie` with the repo mounted. Host `make php-test` still cannot run. `make test` is `go test -race ./...`, which the repo instructions forbid as an unbounded race.

## Risks

- Pushing `v0.0.0-canary.<sha>` is ask-first. The workflow will not create that ref.
- Draft assets are not on the public download URL. The distribution gate has to stage them and still verify the tag-bound signature.
- rc.22 may stay unresolved. The spec forbids a guessed cause, a guessed regression, and a passing product gate in that case.
- Live cleanup failure is a product-gate failure. Destroy only the retained rc.22 cell, and only after the diagnosis record exists.
- Raw logs under `.magelift/` stay uncommitted.

## Proof

- Provider processes started by either GCP `CachedDialer` are closed when `Execute` returns, including errors and cancellation: `TestCachedDialerCloseReapsPlugin` and `TestExecuteClosesProviders`.
- A canary ref packages on the existing slim release lane and cannot replace a lockfile with a different digest: `make workflow-check` and `bash tests/acceptance/release_canary_predicate_test.sh`.
- The rest of the proof is the unticked boxes: three independent gate records, an evidence-backed rc.22 classification or an explicit unresolved result, empty owned inventory, and a sealed evidence file that does not change the capability matrix.

## Deviations

- Codex CLI quota ran out while drafting this plan (`try again at 8:37 PM`). The file list and later boxes were drafted by a separate read-only agent, then corrected against `release.yml`, `CachedDialer`, and the certification package before implementation.
- Process ownership is `CachedDialer.Close` plus `Execute`, not a second session list inside `subprocess.go`.
- Box 5.1: `go test ./internal/certification/ -count=1` passed; `composer test --working-dir=build -- tests/Artifact/ArtifactManifestTest.php` did not run because PHPUnit extensions (`dom`, `mbstring`, `xmlwriter`, and others) are absent on this host. `artifact-gate.sh` records SKIP in that case.
