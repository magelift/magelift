---
status: specified
slug: alpha-development-loop
intent: intent.md
---

# Spec: alpha development loop

## Requirements

### Requirement: commit-addressed canary identity
The system SHALL bind every canary CLI, GCP provider, Magento artifact, gate record, and evidence record to the source commit that produced it.

#### Scenario: valid canary identity
- **WHEN** a maintainer starts a canary for a repository commit
- **THEN** `"commitSha MUST be the full 40-character lowercase hexadecimal Git commit SHA"`
- **AND** `"the canary Git ref name MUST be exactly v0.0.0-canary.<commitSha>"`
- **AND** the signature identity is the existing first-party prefix plus that ref name
- **AND** the CLI and provider version strings are the version the existing dialproof GoReleaser config stamps from that ref
- **AND** the Magento image is recorded by immutable OCI digest rather than by its mutable tag
- **AND** the evidence records the CLI archive checksum, provider binary digest and signature identity, Magento image digest, artifact-manifest digest, build-input identities, and commit SHA

#### Scenario: conflicting publication
- **WHEN** storage already contains a canary asset for the same commit and platform
- **THEN** an identical digest MAY be reused
- **AND** a different digest is rejected instead of replacing the existing asset
- **AND** `"canary asset upload MUST NOT use gh release upload --clobber"`

#### Scenario: no public semantic-version release
- **WHEN** a canary is packaged or executed
- **THEN** `"No canary operation creates, pushes, or undrafts a public release tag of the form v<major>.<minor>.<patch> or a release-candidate tag"`
- **AND** `"the only canary Git ref is v0.0.0-canary.<commitSha>, and it stays draft, prerelease, and not latest"`
- **AND** the release workflow does not create or push that ref

### Requirement: existing canary storage
The system SHALL store canary artifacts in the existing distribution stores appropriate to their artifact type.

#### Scenario: binary canary storage
- **WHEN** the canary CLI and GCP provider are packaged
- **THEN** the CLI archive, provider binary, checksums, Sigstore bundles, and platform provider lock are stored as draft GitHub Release assets for `v0.0.0-canary.<commitSha>`
- **AND** packaging uses `.github/workflows/release.yml` with `.goreleaser.dialproof.yaml`
- **AND** `"release.yml predicates that today match only the v0.0.0-dialproof prefix MUST be extended to a shared dialproof-or-canary predicate"` so module-proxy wait, attestation, published-module verification, undraft, Homebrew, and cask verification stay skipped for `v0.0.0-canary.`
- **AND** the draft is not promoted to a public release

#### Scenario: Magento canary storage
- **WHEN** the reference Magento application is built for the GCP product gate
- **THEN** the image is pushed to the dedicated acceptance project's existing Artifact Registry repository under a commit-addressed tag
- **AND** all later verification and deployment use the returned OCI digest

### Requirement: release-equivalent verification
The system SHALL verify every canary surface through the same verification implementation used for the corresponding release surface.

#### Scenario: CLI archive verification
- **WHEN** the distribution gate installs the canary CLI
- **THEN** the existing installer verifies the real `checksums.txt` Sigstore bundle against the tag-bound `release.yml` identity
- **AND** it verifies the archive SHA-256 against the verified checksums before extraction
- **AND** missing, invalid, or mismatched material fails closed

#### Scenario: provider verification
- **WHEN** the canary CLI resolves the staged GCP provider
- **THEN** the existing provider lock parser and local provider verifier validate the pinned first-party workflow identity, issuer, binary digest, and Sigstore bundle before process start
- **AND** the production resolver, loader, protocol negotiation, and separate subprocess boundary are used
- **AND** no embedded-provider fallback or verification bypass is available

#### Scenario: Magento image verification
- **WHEN** the canary Magento image is prepared for deployment
- **THEN** the existing build, digest-signing, promotion, and deployment verification path signs and verifies the immutable OCI digest
- **AND** the existing artifact manifest and immutable artifact contract identify the source, build inputs, image, manifest, provenance, and signature
- **AND** no second artifact manifest or verifier is introduced

### Requirement: production GCP boundary
The system SHALL execute the GCP product canary through the production CLI-to-provider boundary in the dedicated GCP acceptance project.

#### Scenario: product canary starts
- **WHEN** the GCP product gate begins
- **THEN** the canary CLI loads the verified canary GCP provider as a separate process
- **AND** the provider reports its commit-addressed version through protocol negotiation
- **AND** the existing GKE Autopilot preview topology and existing Magento artifact contract are used
- **AND** no other cloud account, runtime, provider, or topology is mutated

#### Scenario: product identity is reconstructable
- **WHEN** the product gate records its result
- **THEN** the record includes the commit, CLI checksum, executed provider digest and version, Magento image and manifest digests, effective configuration fingerprint, stack identity, state identity, and acceptance run identity

### Requirement: independent gate records
The system SHALL preserve independent outcomes for the distribution, artifact, and GCP product gates.

#### Scenario: gate result schema
- **WHEN** any gate completes or cannot start
- **THEN** `"gate MUST be one of distribution, artifact, or gcp-product"`
- **AND** `"status MUST be one of PASS, FAIL, or SKIP"`
- **AND** a `SKIP` record names the unavailable prerequisite
- **AND** a `FAIL` record names the failed assertion and its sanitized evidence

#### Scenario: one gate fails
- **WHEN** one gate records `FAIL`
- **THEN** every other gate whose own prerequisites remain available continues to completion
- **AND** no aggregate workflow result rewrites another gate's independent status

#### Scenario: shared artifact production fails
- **WHEN** a gate cannot run because its required canary artifact was not produced
- **THEN** that gate records `SKIP` with the missing artifact identity
- **AND** it does not copy the producing gate's `FAIL` status

### Requirement: provider process ownership
The system SHALL terminate every provider subprocess owned by a CLI invocation before that invocation returns.

#### Scenario: successful command
- **WHEN** a CLI command starts a provider process and completes successfully
- **THEN** the provider process is closed and reaped before the CLI exits
- **AND** no child with that invocation's provider PID remains

#### Scenario: failed command
- **WHEN** provider negotiation, transport, provider execution, or command execution returns an error after process start
- **THEN** the provider process is closed and reaped before the error is returned
- **AND** the original operation error remains observable

#### Scenario: cancelled command
- **WHEN** the invocation context is cancelled by SIGINT, SIGTERM, or context cancellation
- **THEN** the in-flight provider operation is cancelled
- **AND** the provider process is terminated and reaped before the CLI exits
- **AND** the command reports cancellation rather than success

### Requirement: evidence-backed HTTP 500 diagnosis
The system SHALL assign the rc.22 HTTP 500 a root-cause classification only when runtime evidence distinguishes that classification.

#### Scenario: diagnostic evidence is collected
- **WHEN** the retained rc.22 environment is inspected
- **THEN** evidence includes the exact deployed identities, deploy-job result, web-server and PHP-FPM logs, Magento exception and system logs, effective redacted runtime bindings, dependency health, an in-cluster service request, and the corresponding ingress request
- **AND** evidence capture occurs before resource destruction

#### Scenario: cause is classified
- **WHEN** the collected evidence identifies the failing boundary
- **THEN** `"cause MUST be exactly one of image, configuration, migration, service dependency, or ingress"`
- **AND** the evidence record states the observations that exclude the other classifications
- **AND** the record distinguishes the root cause from secondary symptoms

#### Scenario: evidence is inconclusive
- **WHEN** the collected evidence does not distinguish one allowed classification
- **THEN** the cause remains unresolved
- **AND** no guessed cause or speculative fix is recorded as the root cause
- **AND** the GCP product gate remains non-passing

### Requirement: root-cause regression
The system SHALL add a deterministic regression test that reproduces the evidence-backed rc.22 cause at the narrowest responsible boundary.

#### Scenario: regression demonstrates the defect
- **WHEN** the regression is run against the pre-fix behavior
- **THEN** it fails for the same condition identified in the rc.22 evidence
- **AND** it does not merely assert a generic HTTP 500

#### Scenario: regression protects the fix
- **WHEN** the regression is run against the corrected behavior
- **THEN** it passes
- **AND** changing the corrected input or behavior back to the diagnosed failure makes the test fail
- **AND** the evidence record names the regression command and test

### Requirement: destroy-on-exit
The system SHALL destroy canary-owned live resources on every exit unless a retained debug cell has explicit authorization.

#### Scenario: normal live run exits
- **WHEN** a live canary succeeds, fails, or is cancelled after mutation begins
- **THEN** the existing destroy, bounded orphan cleanup, and residual inventory sequence runs
- **AND** cleanup produces its own gate evidence

#### Scenario: retained debug cell
- **WHEN** a maintainer requests KEEP
- **THEN** `"KEEP authorization MUST record the approving owner, retention reason, cleanup owner, and expiry"`
- **AND** an unrecorded KEEP request is rejected
- **AND** the cleanup result remains `SKIP` and the GCP product gate cannot pass until teardown and inventory complete

#### Scenario: residual resources remain
- **WHEN** destroy or orphan cleanup finishes with an owned resource still present
- **THEN** cleanup records `FAIL`
- **AND** the remaining resource identities are recorded without secret values
- **AND** the product gate cannot pass

### Requirement: rc.22 experiment closure
The system SHALL close the retained rc.22 experiment after diagnostic evidence has been captured.

#### Scenario: retained resources are removed
- **WHEN** the rc.22 diagnostic capture is complete
- **THEN** the retained preview stack, proof VM, state resources, temporary identities, and other rc.22-owned resources are destroyed through their owning cleanup paths
- **AND** `"remaining rc.22-owned resource count MUST equal 0"`
- **AND** the final inventory result and cleanup time are recorded

### Requirement: secret-safe logs and evidence
The system SHALL prevent secret values from entering canary logs, gate records, or committed evidence.

#### Scenario: credentials are required
- **WHEN** Composer, cloud, database, signing, or application credentials are used
- **THEN** only secret references or redacted metadata are recorded
- **AND** values travel through the existing file, standard-input, environment, or secret-manager boundaries without being printed on command lines

#### Scenario: evidence is published
- **WHEN** gate or diagnostic evidence is sealed or committed
- **THEN** `"secret value occurrences MUST equal 0"`
- **AND** the evidence passes the existing secret-safe validation and clean-room checks
- **AND** raw logs that cannot be proven safe remain access-controlled and uncommitted

### Requirement: capability claim stability
The system SHALL leave public capability status unchanged unless a separate capability change supplies the required matrix and evidence updates.

#### Scenario: canary passes
- **WHEN** all development-loop gates pass
- **THEN** the result proves the development loop for the recorded identities
- **AND** it does not by itself upgrade, broaden, or newly certify a capability-matrix row

#### Scenario: canary fails
- **WHEN** a gate fails or is skipped
- **THEN** no existing capability is retroactively called passing for that canary
- **AND** prior evidence remains bounded to its recorded identities

## Design

### Canary workflow and storage

The binary canary store is the repository's existing GitHub Release asset mechanism in `.github/workflows/release.yml`. That workflow already builds the CLI and autonomous GCP provider, signs `checksums.txt` and each provider binary, generates platform locks, verifies bundles, and keeps a non-dialproof release draft until checks pass. Canaries reuse the existing dialproof lane inside that workflow. They do not add a publisher or a second verifier.

A ref named `canary-<sha>` cannot be the canary identity. `release.yml` triggers only on tags matching `v*`, and `internal/providerhost/lock.go` accepts only identities under `https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/`. A non-`v*` ref would not run the workflow, so it could not mint the tag-bound certificate the installer and provider verifier already require.

- An operator pushes `v0.0.0-canary.<full-commit-sha>` at the commit under test. That ref matches the existing `v*` trigger and is not a public `vX.Y.Z` release. Pushing it is ask-first. The workflow does not create or push the ref.
- `release.yml` currently tests only the `v0.0.0-dialproof` prefix. Those predicates are extended to one shared dialproof-or-canary check. A canary then takes the slim lane: no module-proxy wait, `.goreleaser.dialproof.yaml` (draft, prerelease, `make_latest: false`), no attestation, no published-module verification, no undraft, no Homebrew, and no cask job. Ordinary `v*` releases keep their current path. Canary uploads compare an existing asset or lock and reject a different digest. They do not use `gh release upload --clobber`.
- The slim Linux/amd64 packaging shape produces the CLI archive and GCP provider binary. Version stamping stays the dialproof GoReleaser stamp for that ref. Archive, checksum, and signature layout stay the same.
- `cmd/genproviders/main.go` and `internal/providerhost/generate.go` produce the existing `magelift.providers.lock` schema for the canary assets. The generator, parser, digest verification, publisher pin, and protocol marker remain shared with releases.
- The assets remain in a draft GitHub Release. The public-candidate publication step, module-proxy wait, Homebrew, Scoop, release attestation, and cask verification do not run for canaries.
- Internal runners retrieve draft assets with authenticated GitHub access. The distribution gate passes those real assets through `website/public/install.sh`; its release-base override only supplies the authenticated local staging endpoint, while the real checksum bundle and tag-bound workflow identity are still verified.
- The GCP product runner stages the platform lock and provider binary plus bundle in the existing provider cache layout. Execution still resolves through `internal/providerhost/resolve.go`, verifies through `internal/providerhost/host.go` and `internal/providerhost/verify.go`, and dials through `internal/providerhost/client.go`. This intent does not implement automatic end-user provider acquisition.
- After gate evidence is durable, the draft release and ephemeral canary tag may be deleted. The evidence retains the commit and immutable artifact digests, matching the consumed-and-deleted dialproof precedent recorded in `docs/evidence/gcp-gke-autopilot-magento-live-mldp3-20260914.md` and `docs/evidence/gcp-gke-autopilot-magento-search-live-mldp6-20260914.md`.

The Magento artifact remains an OCI image in the dedicated acceptance project's Artifact Registry. `internal/cli/build.go` and `internal/build/pipeline/pipeline.go` already require immutable builder/runtime inputs for pushed builds, produce the external artifact manifest, return the image digest, and sign that digest. The product gate supplies the digest to `providers/gcp/scripts/gcp-acceptance-local.sh`, whose current preflight already verifies that a `.pkg.dev` digest exists and satisfies the Magento runtime contract.

This storage choice avoids a second trust implementation:

- GitHub Actions caches in `.github/workflows/ci.yml` are evictable build accelerators, not distribution storage or immutable evidence.
- Generic Actions run artifacts would require a new provider acquisition URL format and authenticated downloader path that the release lock and provider downloader do not implement.
- GHCR in `.github/workflows/images.yml` currently stores signed OCI base images; wrapping CLI and provider binaries as OCI artifacts would require a new downloader and verification path.
- `internal/certification/artifact_registry.go` stores immutable identity and build-claim records on one host; it is not blob storage for CLI archives or provider binaries.
- Pulumi state buckets and recovery buckets serve state and backup contracts, not software distribution. Reusing them would add an unrelated trust root.

### Gate execution and records

The canary workflow exposes independent distribution, artifact, and GCP product checks. Packaging may be a shared prerequisite, but the checks do not depend on each other's conclusion and do not use fail-fast aggregation.

The distribution check proves the canary CLI archive, signed checksums, provider lock, provider digest and bundle verification, protocol negotiation, and provider-process lifecycle. It reuses `website/public/install.sh`, `internal/providerhost`, and the process regression tests.

The artifact check runs the existing `magelift build --push` path for the reference shop. `internal/cli/build.go` signs the pushed digest. It does not verify that signature or construct `sdk.ImmutableArtifactContract`. The gate then runs the existing certification path in `internal/certification/artifact_build.go`, which constructs that contract, and verifies the signed digest with the existing verifier. The gate also validates `build/src/Artifact/ArtifactManifest.php` output and checks secret absence. It does not publish a builder/runtime catalog.

The GCP product check uses `providers/gcp/scripts/gcp-acceptance-local.sh` with a unique ownership prefix, the commit-addressed CLI and provider, and the Magento OCI digest. The harness continues to own preview, deploy, health, destroy, orphan cleanup, and inventory. The canary does not add a runtime or provider.

A current sanitized proof is added under `docs/evidence/` and linked from `docs/evidence/README.md`. It contains an independent row for each gate, links to the CI run and sealed live evidence, and records the rc.22 diagnosis, regression command, and cleanup outcome. `scripts/acceptance/lib-evidence.sh` writes an unsealed candidate under `.magelift`. A separate `magelift certification seal` step seals that candidate and places it under `docs/evidence/runs/`. Raw command and runtime logs stay in the existing `.magelift/gcp-matrix/` working area until redacted; they are not committed as the proof.

The existing artifact views remain authoritative:

- `build/src/Artifact/ArtifactManifest.php` is the Magento artifact content record.
- `sdk/artifact.go` supplies the immutable artifact and reuse identity.
- `sdk/types.go` supplies the small image-and-manifest deploy transport.

No canary manifest is added.

### Provider process lifecycle

`internal/providerhost/client.go` already kills a process after failed start, failed dispense, failed negotiation, and operation cancellation. It also exposes `Close`, but the registry-owned cached sessions in `internal/registry/registry.go` and `internal/registry/hooks.go` are not closed by the per-backend tracking in `internal/cli/subprocess.go`.

The process fix binds every process created by `providerhost.Dial` to the invocation context supplied by `internal/cli/execute.go`. Completion of `ExecuteContext`, command failure, SIGINT, or SIGTERM cancels that context and causes an idempotent close-and-reap operation. Existing explicit closes remain valid and use the same idempotent termination path. The CLI must not return to `cmd/magelift/main.go` until owned provider children have exited.

The regression suite uses a real helper provider process and observes its PID across successful execution, typed operation failure, transport failure, and cancellation. Unit tests in `internal/providerhost` cover client ownership; a CLI-level test covers root-context shutdown so registry and hook paths cannot evade the assertion.

### rc.22 diagnosis and regression placement

The rc.22 investigation starts from the retained identities and does not alter topology before evidence collection. Diagnostics use existing MageLift logs and exec operations plus the acceptance runner's Kubernetes access to capture:

- deploy Job status and logs;
- web-server and PHP-FPM output;
- Magento `exception.log` and `system.log`;
- redacted effective runtime bindings;
- database, cache, search, object-storage, and queue reachability;
- an HTTP request made directly to the service inside the cluster; and
- the corresponding request through the external ingress.

Classification follows the observed boundary:

- `image`: the immutable image lacks or contains incorrect application/runtime content independently of cloud bindings.
- `configuration`: required content exists, but an effective runtime binding or generated configuration is absent or incorrect.
- `migration`: the deploy Job, schema transition, or required deployment marker failed before serving rollout.
- `service dependency`: configuration is correct, but a required database, cache, search, queue, or storage dependency is unavailable or rejects the workload.
- `ingress`: the in-cluster service request succeeds while the equivalent ingress request fails.

The regression location is selected only after classification. Image or manifest defects belong in the existing `build/tests/` or `internal/build/pipeline/` suites. GCP configuration, migration, service, or ingress defects belong beside the responsible code in `providers/gcp/runtime/`, `providers/gcp/operations/`, `providers/gcp/plugin/`, or the acceptance contract tests under `tests/acceptance/`. The test records the exact observed failure input and must fail independently of a live cloud account unless the cause is inherently a live ingress behavior.

### Cleanup and residual inventory

The existing GCP acceptance trap in `providers/gcp/scripts/gcp-acceptance-local.sh` is armed before paid mutation. It runs MageLift destroy, bounded provider-owned orphan cleanup, WIF and temporary-secret cleanup, state-bucket removal, and `assert_clean`. Its inventory covers run-owned networks, network endpoint groups, GKE clusters, Cloud SQL instances and backups, Memorystore, service-connection policies, secrets, addresses, Cloud Armor policies, and GCS buckets.

Closing rc.22 reuses those owning cleanup paths for its retained stack and separately destroys the retained proof VM by its recorded identity. The final rc.22 evidence includes the exact inventory commands and an empty result. Pre-existing unowned resources in the acceptance project are neither deleted nor counted as rc.22 residuals.

KEEP remains exceptional. A retained run records authorization and cleanup ownership in its evidence, remains non-passing, and is later resumed only to capture the needed diagnostics and complete teardown. There is no leftover KEEP ledger in committed evidence.

## Gotchas / policy flags

- Canary signing must retain the exact tag-bound GitHub Actions identity required by ADR 0014. A branch identity, unsigned checksum, checksum-only provider, or alternate test verifier is not equivalent.
- `v0.0.0-canary.<sha>` is remote tag state. Creating or pushing it is ask-first, the same class as a release tag. The workflow never creates that ref and never undrafts it. Deleting the draft release and the ref after evidence is also an explicit operator action.
- The provider process has the user's cloud privileges and is not a sandbox, per ADR 0013.
- GCP live execution is limited to the dedicated acceptance project, uses a unique ownership prefix and immutable Magento digest, previews first, and destroys on exit.
- The rc.22 proof VM and stack are destroyed only after required diagnostic evidence is captured. Any protected or production target would require separate approval.
- Cleanup failure is a product-gate failure, not a warning. An unresolved inventory prevents passing evidence.
- Secret references may be recorded; secret values, identity tokens, Composer credentials, Pulumi passphrases, database passwords, and application encryption keys may not.
- Clean-room rules remain enforced by `make check-clean-room`; no private sibling source, employer material, identifiers, or credentials enter tests or evidence.
- A development-loop pass does not change `docs/capability-matrix.md`. Certification claims still require agreement between that matrix and `docs/evidence/README.md`.
- The design does not implement automatic provider installation, publish a builder/runtime catalog, create a public alpha, extract AWS, add a runtime or vendor, bundle the provider into the CLI, or change IaC engines.

## Open questions carried forward

None. CLI and provider canaries use draft GitHub Release assets on `v0.0.0-canary.<commitSha>`, packaged by the existing `v0.0.0-dialproof` lane of `release.yml`. The Magento image uses the dedicated acceptance project's Artifact Registry. Both surfaces use the existing verifiers. A `canary-<sha>` ref was rejected because `release.yml` only runs on `v*` tags. The rc.22 classification remains an evidence-gathering result required by this spec, not an unresolved design choice. Pushing or deleting the canary ref stays ask-first and is not a workflow step.