# Findings (task-scoped)

## .10 CI (2026-09-14 ~12:30Z)
- Run 34841348135 `in_progress` (goreleaser step+). Tag refs exist for .8/.9/.10; releases list .9/.8/.6 only.
- Earlier Release Artifacts runs show FAILs (34832651101, 34806936710, ...) — dialproof iteration history.
- "Release Container Images" runs all `[pending]` — not investigated (shop image comes from AR, not GHCR).

## Blocker root cause + fix (day2:exec via subprocess) — FIXED, uncommitted
- `cmd/magelift-provider-gcp/execute.go` served `ExecuteOutputs` with `RedactedOutputs` — kubeconfig arrived as `{"secret": true}`, failing `RequireStringOutput` for every in-process day-2 consumer (exec, health, logs, deploy Steps, tunnel).
- "Day-2 ports stay in-process" (ADR 0011) = port CODE runs in host; its INPUT (outputs map) crosses the RPC. Fix: plumb decrypted outputs through Execute RPC over local mTLS (matches in-process `Outputs` semantics).
- Fix: `ExecuteOutputs` → `backend.Outputs` (decrypted); new `ExecuteRedactedOutputs` ("redacted-outputs") → `backend.RedactedOutputs`; `SubprocessBackend.RedactedOutputs` added (outputsCommand type-asserts it — without it display would leak secrets post-fix).
- Pins: `TestExecuteOutputsRoutesDecryptedToDay2` (provider op→method), `TestSubprocessBackendRedactedOutputs`, `TestOutputsDisplayRedactsThroughSubprocessBackend` (CLI display), Dial mock round-trip incl. redacted shape, valid-op case.
- Green: providerhost 38 pass; cli + cmd/magelift-provider-gcp 288 pass.
- Docs: ADR 0011 + adding-a-provider.md updated (public contract). No humanizer skill on this box — prose kept minimal/factual; note for maintainer.

## DEVIATION for plan box 1.3: .11 retag required
- The fix spans host (CLI) + provider (subprocess binary). Session CLI builds from tree (gets fix), but the .8/.9/.10 provider bundles predate it: .10 bundle serves redacted from outputs op and rejects redacted-outputs as unknown op.
- So .10 can still prove "workflow green / lockfile step", but the Dial proof must consume a .11 bundle cut from the fix commit. After .11 green: download .11 triple, delete tags .8/.9/.10/.11, tick 1.3.
- Commit needed for tag. Ephemeral dialproof tags authorized by plan (throwaway, deleted after use).

## Seed dump — REGENERATED (not copied)
- Certified runs used `magento-249-sanitized-definer-free.sql.gz` (gcap28 fixture sha c57c2c…; AWS evidence: `.magelift/seed/…`). Live `up` gate REQUIRES installed-schema tables (flag, setup_module, core_config_data) — tiny.sql fails it.
- Regenerated via setup:install of the acceptance image (2.4.9/PHP8.5) against scratch mysql:8.4.11 + opensearch 2.19.6, admin@sanitized.invalid, DEFINERs stripped with the project's own sed, gzip.
- Gotchas hit: image bakes app/etc/env.php with static db host (must rm before install); key must be exactly 32 chars; scratch mysql needs --log-bin-trust-function-creators=1 (same 1419 as gcp-acceptance doc).
- New: /tmp/magento-249-sanitized-definer-free.sql(.gz), sha256 a2cd333cbd540f65f6401e4d0103b552fad4429db440bfc3b9aa12b1cde58cc1, harness validator PASS, zero DEFINERs, sole email admin@sanitized.invalid.
- Recipe images cached in docker (AR image + mysql:8.4 + opensearch:2); scratch containers/network removed.

## Digest + signing (for preview up)
- Digest: `europe-west1-docker.pkg.dev/digital-lab-341608/magelift-acceptance-rc1-20260813/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13fc390bc2b733ec3d03a0501ecc82a6ebad7fcca7758c933977fdb2be4` (tag `rc1`). The 09-13 `aa2fc2ec` entry is WSL's cosign SIGNATURE (.sig tag), not an image.
- GHCR magelift/magento is private (403 anon; gh token lacks read:packages) — irrelevant, image lives in AR.
- Signing: harness runs `magelift sign` (fresh sign, our identity) + `promote` in create-once. Need cosign (INSTALLED v3.1.3 pinned, ~/.local/bin) + identity token argv. Plan: `gcloud auth print-identity-token --audiences=sigstore --include-email` as active user; identity = user email; issuer https://accounts.google.com. Untested — first attempt happens at preview create-once.
- Crypt key: harness auto-creates `${NAME}-${PROFILE}-magento-crypt-key` (openssl rand -hex 32) — nothing to recover.

## GCP env on this box
- gcloud active: alexandre.courtiol@groupechantelle.com; AR + SA reads work against digital-lab-341608 (no extra login needed so far).
- Default gcloud project is iron-century-… — harness takes MAGELIFT_GCP_PROJECT explicitly; never rely on default.
- Project has pre-existing infra (cloud-run-source-deploy, gcf-artifacts repos, many SAs incl. platform-*) — NOT empty in general; handover's "verified empty" covered magelift-acceptance resource kinds only. assert_clean is prefix-scoped (mldp3+), so safe.

## Devbox build notes (user override of MacBook serial rule)
- 8 CPU / 15G RAM. `GOMAXPROCS=4 GOFLAGS=-p=4 GOMEMLIMIT=6GiB` works BUT parallel test linking overflows the 7.7G /tmp tmpfs ("mapping output file failed: disk quota exceeded", also seen spuriously as eval write failures). Fix: `TMPDIR=/home/dev/.tmp-gotest` (on 54G-free sda1). pulumi-mock-test green with that (69 ok).
- local-gates FULLY GREEN 2026-09-14: pulumi-mock 69 ok; floci-aws+gcp exit 0; harness exit 0 (needed user-local php8.5 deb extract + wrapper; cosign 3.1.3 installed to ~/.local/bin).

## Preview re-run runbook (mldp3, Dial subprocess) — run after .11 green

## 0. Preconditions (all verified 2026-09-14)
- [x] Fix committed (e423031) + pushed; unit green; local-gates green
- [x] Session CLI pre-built: /tmp/magelift-gcp-mldp3/magelift (dev, 208M)
- [x] Seed: /tmp/magento-249-sanitized-definer-free.sql.gz (a2cd333c…), validator PASS
- [x] Digest signed: cosign verify OK (identity devops@ SA, issuer accounts.google.com)
- [x] Tools: cosign 3.1.3, kubectl, pulumi, gke-auth-plugin, php 8.5 (harness only), jq/yq/go/docker
- [ ] .10 green (lockfile step proof) — IN PROGRESS
- [ ] .11 tag cut from e423031 + green (consumable triple with fix)

## 1. After .10 green
- gh release download v0.0.0-dialproof.10 --pattern 'magelift-provider-gcp_0.0.0-dialproof.10_linux_amd64*' (NO — .10 bundle is STALE, do not install)
- Actually: download NOTHING from .10. Record workflow-green evidence (run id + conclusion) for box 1.3.

## 2. Cut .11
- git tag v0.0.0-dialproof.11 e423031 && git push origin v0.0.0-dialproof.11
- Watch: gh run list --branch v0.0.0-dialproof.11 (expect ~1-1.5h like .10)
- On green: download linux_amd64 triple from .11 release:
  - magelift-provider-gcp_0.0.0-dialproof.11_linux_amd64 → /tmp/magelift-gcp-mldp3/magelift-provider-gcp (chmod +x)
  - magelift-provider-gcp_0.0.0-dialproof.11_linux_amd64.sigstore.json → same dir, KEEP release filename (lock names it)
  - magelift.providers.lock.linux_amd64 → /tmp/magelift-gcp-mldp3/magelift.providers.lock
- Sanity: CLI reports subprocess mode: /tmp/magelift-gcp-mldp3/magelift extensions list (providers entry mode=subprocess)
- THEN delete tags .8/.9/.10/.11 (releases + tags) and tick box 1.3.

## 3. Preview up (NEVER interrupt mid-create/destroy)
DIR=/tmp/magelift-gcp-mldp3 (has prebuilt BIN; harness reuses)
NAME=mldp3
Env:
  MAGELIFT_GCP_ACCEPTANCE=1
  MAGELIFT_GCP_PROJECT=digital-lab-341608
  MAGELIFT_GCP_REGION=europe-west1
  MAGELIFT_GCP_ACCEPTANCE_NAME=mldp3
  MAGELIFT_GCP_ACCEPTANCE_DIR=/tmp/magelift-gcp-mldp3
  MAGELIFT_GCP_ACCEPTANCE_PROFILE=preview
  MAGELIFT_GCP_ACCEPTANCE_DIGEST=europe-west1-docker.pkg.dev/digital-lab-341608/magelift-acceptance-rc1-20260813/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13fc390bc2b733ec3d03a0501ecc82a6ebad7fcca7758c933977fdb2be4
  MAGELIFT_CERTIFICATE_IDENTITY=devops@digital-lab-341608.iam.gserviceaccount.com
  MAGELIFT_CERTIFICATE_OIDC_ISSUER=https://accounts.google.com
  MAGELIFT_GCP_ACCEPTANCE_SEED_DUMP=/tmp/magento-249-sanitized-definer-free.sql.gz
  MAGELIFT_DUMPIMPORT_RUNNER=kube
  TMPDIR=/home/dev/.tmp-gotest (avoid /tmp tmpfs link overflow)
  (no MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV — promote verifies existing sig; no fresh sign)
  (no KEEP — destroy on EXIT; capture logs under DIR/logs)

Cmd: ./scripts/gcp-acceptance-local.sh up (from repo root)
Watch for: "using subprocess provider magelift-provider-gcp …" (Dial proof) + day2:exec PASS (fix proof)
Expected ~19m infra + Magento cells; measured GCP preview infra-only ~19m21s.

## 4. Evidence + spend + destroy
- EXIT trap destroys; assert_clean prefix-scoped; record spend line (billing not queryable? use cost:estimate cell + note).
- Evidence file under docs/evidence/ + index; sealed JSONL under docs/evidence/runs/.

## 5. Then per plan
- standard (RUNTIME=gke-standard override), HA, order-9 search, order-10 loop — same packed session?
  (Plan says ONE packed GCP session: preview, standard, HA + live Dial proof + order-9 + order-10. Warm reuse rules: cold stack for runtime/profile changes. Preview→standard (Autopilot→Autopilot? standard profile default runtime?) — check profile defaults before deciding warm vs cold. HA defaults to gke-standard → cold stack.)

## Sign decision for preview create-once: NO fresh sign (promote verifies existing)
- `gcloud auth print-identity-token --audiences=sigstore` FAILS for user accounts (needs SA); WSL signed as devops@digital-lab-341608.iam.gserviceaccount.com (read from .sig cert SAN; issuer https://accounts.google.com).
- `cosign verify` with that identity+issuer PASSES on digest 8588b13... — so promote succeeds without MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV. No impersonation needed.
- Compat matrix (SDK API stays v1; dialproof bundles are throwaway): newCLI+newBundle=correct; newCLI+oldBundle=day-2 still redacted + display errors loudly (do not use); oldCLI+newBundle=display would fall through to decrypted (skew-warned, unsupported). Single-version pairing rule covers it.

## CI notes
- Branch pushes trigger no CI (ci.yml: PRs + main only). Local gates are the signal.
- Release Container Images shows skipped on dialproof tags (expected; shop image lives in AR).

## Pre-existing (not mine, left alone)
- `make fmt-check` FAILS at HEAD on `internal/localdev/catalog.go` (mis-indented block from WIP commit d939890). My files are gofmt-clean. Not fixing: out of intent scope; flagging for maintainer.
- `make docs` strict build: PASS with my ADR/provider-doc edits. `make check-clean-room`: PASS.

## Workstation php (user-local, best-effort)
- Built user-local PHP 8.5.4 from Ubuntu debs (php8.5-cli + common + xml + mbstring + intl + zip + curl + libs) into ~/.local/php with ~/.local/bin/php wrapper (LD_LIBRARY_PATH + autoload all .so) and ~/.local/php/conf.d/00-magelift.ini (extension_dir + extensions) for PHP_BINARY-spawned children (composer scripts). composer wrapper at ~/.local/bin/composer (2.10.3). `composer install --working-dir=build` exit 0.
- php-test: 125/128 pass. 3 failures in NativeProcessRunnerTest are PROVEN environmental: proc_open with empty env drops LD_LIBRARY_PATH so user-local php8.5 can't start (127). Passes on any normal php. No patchelf/ldconfig available to fix cleanly. Not my scope (zero php touched); CI covers php-test.
- REQUIRE for future php runs in this shell: export LD_LIBRARY_PATH=$HOME/.local/php/usr/lib/x86_64-linux-gnu PHP_INI_SCAN_DIR=$HOME/.local/php/conf.d (set in persistent shell).

## .10 FAILED, .11 retagged (2026-09-14 ~14:05Z)
- .10 run 34841348135 FAILED after 1h59m at Generate provider lockfiles: `missing bundle for magelift-provider-gcp_0.0.0-dialproof.10_darwin_amd64` — SAME as .9. eb776eb's "beside dist/" guess was wrong.
- Root cause (proven via local snapshot repro with touch-signer): for `formats: [binary]`, GoReleaser does NOT copy the archive to dist/ root. The Binary artifact path IS the build output (`dist/magelift-provider-gcp_<os>_<arch>_v1/magelift-provider-gcp`), and the Signature lands beside it there (upload name still `<asset>.sigstore.json`, bytes identical). Neither dist/ root nor repo root ever holds them.
- Fix (a566df3, pushed): lockfile + verify steps resolve bundle/binary paths from dist/artifacts.json (type Signature/Binary by upload name). Both steps exercised VERBATIM against snapshot dist/ (lock writes valid lock; verify reaches cosign with correct paths — fails only on the fake touch bundle as expected). Also fixed the verify step's `dist/magelift-provider-*` glob, which matched only build DIRS and would have failed next.
- .11 tagged from a566df3 (run 34854228510, ETA ~2h). .11 bundle = workflow fix + Execute fix = consumable Dial triple. Delete .8/.9/.10/.11 tags+releases after .11 triple consumed, tick box 1.3.
- Note: snapshot repro needed GOMEMLIMIT (OOM without) and trimmed goos/goarch (release has no --single-target).

## Fast dialproof path (user request 2026-09-14 ~14:40Z)
- 2h/release debug loop was full 12-compile matrix. Fix (5c13afe): `.goreleaser.dialproof.yaml` (linux/amd64 only, no sbom/taps/attest; same signs/checksum/archive shape) + release.yml conditionals (dialproof tags → slim config; attest + cask-verify skip dialproof). Proven via local snapshot + verbatim lockfile step.
- Cancelled .11 full-matrix run (nothing consumable lost: devbox needs linux_amd64 only; multi-platform naming already validated via artifacts.json pattern). .12 tagged from 5c13afe (run 34857288057, ETA ~10 min).
- Long-term real-release speed (matrix split / GHA cache) is a follow-up, not this loop.

## STOP-SAFE state 2026-09-14 ~16:35Z (tmux restart)
- mldp3 run COMPLETE: 13/13 PASS (bootstrap:wif, composer x2, day2 x5 incl exec, search:health, migrate:dump, deploy:candidate 225s, deploy:repeat, cost:estimate), destroy + assert_clean ok, HARNESS_EXIT=0. Log: /tmp/gcp-preview-mldp3.log. Run ID run-20260914t153422z-422805. Cloud empty (0 clusters, 0 SQL).
- .12 tag+release DELETED after consumption. Box 1.3 verify done except evidence commit + tick.
- Sealed: docs/evidence/runs/gcp-gke-autopilot-magento-mldp3-20260914.sealed.jsonl (13/13, cleanupVerified, UNTRACKED). PROBLEM: contains 65x digital-lab-341608 + real AR paths; gcap28 sealed has ZERO. `certification seal` does not redact — find the August redaction step before committing (grep history for registry.example.invalid writer). Candidates: /tmp/mldp3-unsealed.jsonl (also unredacted, fine for /tmp).
- Resume env: session CLI + .12 triple at /tmp/magelift-gcp-mldp3/ (magelift, magelift-provider-gcp, .sigstore.json, .lock). Seed at /tmp/magento-249-sanitized-definer-free.sql.gz (sha a2cd333c). Digest/identity/issuer in runbook section above. Shell needs: export LD_LIBRARY_PATH=$HOME/.local/php/usr/lib/x86_64-linux-gnu PHP_INI_SCAN_DIR=$HOME/.local/php/conf.d (fresh tmux shell won't have these; only needed for php/composer).
- Commits this session (all pushed): 164d079 chore tasks-ignore, e423031 Execute fix, a566df3 artifacts.json, 5c13afe slim dialproof, 41c34e4 + e0572de plan docs.

## Standard attempt 1 (mldp3) FAILED on GCP capacity (2026-09-14 ~17:15Z)
- Memorystore Valkey create failed after 594s: Error code 8 "No resource available to create the requested Valkey Instance in the specified region, please try again later" (provider google-beta@9.36.1). Transient GCP-side exhaustion in europe-west1, not a MageLift bug. Zero cells ran. EXIT trap destroyed everything; assert_clean ok; HARNESS_EXIT=1. Log: /tmp/gcp-standard-mldp3.log.
- Pool ml-mldp3-standard tombstoned to 2026-10-14 → NAME mldp3 spent for profile standard. Retry as mldp4 (fresh DIR /tmp/magelift-gcp-mldp4-standard, same region).
- Note: standard/HA run IN-PROCESS by design (subprocess only for gcp/gke-autopilot proof cell; cli/subprocess.go). No fallback notice is printed for other targets.

## Standard mldp4 GREEN (2026-09-14 ~18:10Z)
- 13/13 PASS (deploy:candidate 101s, HTTP 200, CLI 2.4.9), destroy + assert_clean ok, HARNESS_EXIT=0. Run run-20260914t171509z-569079. Evidence committed (md + sealed, zero IDs, docs build pass).
- Names spent: ml-mldp3-standard (capacity fail), ml-mldp4-standard (green).

## Order-9 GCP search cell PROVEN (2026-09-14 ~20:55Z, mldp6 KEEP stack)
- Root cause of first reindex failure: #env() under env.php system/ never
  resolves (config:show returned the raw literal). Fix: CONFIG__DEFAULT__
  search bindings in CoreEnvBindings + localdev (commit 6c53304).
- Fix deployed via dialproof.13 provider (slim CI green first try ~20min):
  preview showed ~3 updates (web+cron Deployments), infra-only apply ok.
  NOTE: full `deploy` on the live stack failed with "create migrate Job:
  Unauthorized" (tried to re-create the migrate Job); infra-only is the
  correct tool for spec updates on a live stack.
- Proof: config:show resolved (host mldp6-preview-search, engine
  opensearch); indexer:reindex catalogsearch_fulltext exit 0
  ("rebuilt successfully in 00:00:01"); storefront q=Probe → HTTP 200 +
  product hit (43899B); pod mldp6-preview-search-0 deleted + Ready
  (~44s); re-query NO reindex → HTTP 200 + hit (43860B).
- Product ML-SEARCH-PROBE-1 via REST (admin searchprobe created with
  stdin password, never logged). Probe secrets shredded post-proof.

## Env-leak lesson (2026-09-14): persistent shell exported
PULUMI_CONFIG_PASSPHRASE_FILE from mldp6 into the mldp7 KEEP run, so
mldp7's stack secrets use mldp6's passphrase file. Rule: unset
stack-specific vars between runs. mldp7 stack ops must export
PULUMI_CONFIG_PASSPHRASE_FILE=/tmp/magelift-gcp-mldp6-search/pulumi-passphrase.

## AWS local acceptance needs no `magelift bootstrap` (2026-09-15)
- Harness is self-sufficient: creates its own state bucket
  (`ensure_state_bucket`), KMS/keychain secrets, ownership marker.
- Running `magelift bootstrap` FIRST breaks the pre-create gate twice:
  1. its state bucket lacks the run ownership marker
     ("allowing exact owned ... during resume preflight" only),
  2. its 3 GitHub-OIDC roles (magelift-<project>-preview-{build,ci,deploy})
     trip `assert_clean` ("leftover IAM roles: 3") with no allowlist.
- Fix: delete the 3 roles (detach policies first); leave the bootstrap
  state KMS key for session-close deletion; relaunch harness alone.

## AWS stack KMS key needs CloudWatch Logs statement (2026-09-15)
- Run mlaw1 #6 create-once failed on first encrypted resource:
  `AccessDeniedException: The specified KMS key ... is not allowed to be
  used with Arn 'arn:aws:logs:...'` for log group
  mlaw1-preview-observability-web-logs.
- Manually created stack key (6e929b63) had only the default root
  policy. CloudWatch Logs requires an explicit key-policy statement for
  the logs.<region>.amazonaws.com service principal (documented AWS
  special case; RDS/ElastiCache/Secrets work via caller grants).
- Fix: added Sid AllowCloudWatchLogsUseOfKey scoped by
  ArnLike kms:EncryptionContext:aws:logs:arn to log-group:*mlaw1*.
- EXIT destroy + assert_clean still passed (HARNESS_EXIT=1, zero live
  leftovers); tag index lagged with 18 mappings (eventual consistency).

## AWS run #8: CloudFront cert must cover mediaDomain (2026-09-15)
- Create failed: InvalidViewerCertificate — us-east-1 cert covers only
  ml-aws-20260823ba.acourtiol.com + ml-aws-media-20260823ba.acourtiol.com
  (August domains). mediaDomain mlaw1-media.example.test rejected.
- Fix: mediaDomain -> ml-aws-media-20260823ba.acourtiol.com (base.yaml).
- EXIT trap deletes prerequisite secrets on full-cycle failure (run #8:
  gone without tombstones); gate-fail path (run #7) had left them.
  Rule: verify + recreate secrets after EVERY run before relaunch.

## AWS run #9: amazon-mq needs catalog.rabbitMq.instanceType (2026-09-15)
- queueMode:db PASS, queueMode:ecs-rabbitmq PASS, queueMode:amazon-mq FAIL:
  "AWS MQ admission requires engine, version, and instance type".
- Preview preset blanks rabbitMQInstanceType (defaults.go); engine version
  comes from the app matrix (2.4.9 -> 4.2) automatically.
- Fix: catalog.rabbitMq.instanceType=mq.m7g.large in base.yaml (schema-valid).
- EXIT cleanup timed out (900s) on CloudFront disable+delete; manual: disable
  distribution, wait Deployed, delete distribution + OAC + media bucket.

## AWS run #9 destroy also failed on amazon-mq admission (2026-09-15)
- The FAIL patch (queueMode=amazon-mq without rabbitMq.instanceType) broke
  BOTH the cell deploy AND the EXIT `magelift destroy` (preview errored the
  same way), leaving the full stack up: VPC, RDS, Valkey, ALB, 2 ECS
  clusters, roles, log groups, CF, S3 + an automated RDS snapshot.
- Manual teardown order that worked: RDS instance (skip snapshot), Valkey
  group, NAT EC2, EIP (by AllocationId), ECS services --force, clusters,
  ALB listeners/LB/TGs, log groups, IAM roles (detach+delete), RDS subnet+
  param groups, cache subnet group, subnets, RTs, SG rule revoke (python)
  then delete, IGW, VPC endpoint (vpce-* — easy to miss!), VPC.
- Automated RDS snapshot cannot be deleted manually but expired on its own
  within ~1h (backup retention 1d); gate query counts it while present.
- No mlaw2 pivot needed: mlaw1 scope is clean (0/0/0/0/0/0/0 + 3 secrets).

## AWS run #11: OpenSearch VpcId unconfigurable + fix (2026-09-15)
- amazon-mq PASS (rabbitMq fix works), searchMode:disabled PASS,
  searchMode:provisioned FAIL: provider rejects explicit
  vpc_options.vpc_id ("Value for unconfigurable attribute").
- August ran serverless/AOSS, never provisioned: first live hit.
- Fix (095e0d7): drop VpcId from DomainVpcOptionsArgs (derived from
  subnets); unit tests 40 pass; binary rebuilt.
- EXIT destroy failed on the same preview error again -> manual
  teardown #2 incl. MQ broker delete (fast) + CF + full VPC chain.

## AWS run #12: VPC OpenSearch needs legacy ES SLR (2026-09-15)
- VpcId fix works (preview passed, destroy also works now: run #12 EXIT
  destroy + assert_clean ok, no manual teardown).
- New failure at domain CREATE: ValidationException "must enable a
  service-linked role ... to access your VPC" even though
  AWSServiceRoleForAmazonOpenSearchService exists. VPC path checks the
  legacy es.amazonaws.com SLR -> created
  AWSServiceRoleForAmazonElasticsearchService (account-level, one-time).

## AWS run #13: ElastiCache auth_token forbids `/` (2026-09-15)
- Preview failed before create: cache secret was base64 (contains `/`),
  which ElastiCache auth_token rejects. Previous 5 base64 generations
  were luckily `/`-free.
- Fix: put-secret-value with `openssl rand -hex 24` (same ARN, no YAML
  change). Rule: cache secrets must be hex, never base64.

## AWS run #14: native Magento needs https:// in search hostname (2026-09-15)
- 5 PASS then databaseEngine:rds-mysql FAIL: setup:upgrade "Could not
  validate a connection to the OpenSearch. Unknown 400 error".
- Root: SearchClient::buildOSConfig takes the scheme from the hostname
  (default http). Unprefixed AWS hostname -> http://host:443 -> the
  HTTPS-only domain answers 400. (Search cells use --infra-only so they
  never validate; the first full deploy after provisioned trips it.)
- Fix (4a63fc7): prefix https:// on native hostname bindings when
  httpsMode==1; ElasticSuite keeps bare host:port + flag. Tests 91 pass.
- EXIT destroy + assert_clean ok (no manual teardown needed).

## EU run: never interrupt live k8s-acceptance mid-deploy (2026-09-15)
- Two interrupts (Scaleway 8 min in, OVH 25/30 min in) both orphaned
  billable resources: the EXIT-trap destroy races workdir cleanup and
  loses. Manual ordered teardown required both times.
- Rule: launch live EU runs with a 60-min window and do not touch them.
  If interrupted anyway, sweep immediately (see next two entries).

## OVH sweep: `cloud network private list` does not exist (2026-09-15)
- `ovhcloud cloud network private list` prints help (only `vrack`
  subcommand exists). Piping it to grep yields zero matches = FALSE
  CLEAN. A rerun then fails with 400 "private network already exists
  for this vlan ID".
- Rule: sweep OVH networks/gateways only via the signed API reader
  (`ovh_api_request` in k8s-acceptance-local.sh): GET
  /cloud/project/{P}/network/private plus
  /region/{R}/gateway. Never trust the CLI for these.

## OVH orphan: gateway holds the subnet, delete cascades (2026-09-15)
- Subnet delete failed 400 "One or more ports have an IP allocation"
  35+ min after MKS deletion. Holder was the orphan gateway
  (ovh915-preview-net-gateway) with interfaces on the subnet.
- Fix order: DELETE regional gateway first, then subnet, then network.
  In this case the gateway delete cascaded: subnet+network 404'd after.
- Harness gap (not fixed this session): the orphan cleanup loop retries
  subnet/network but never looks for the gateway. Worth a follow-up.

## Scaleway run: managed passwords need every class (2026-09-15)
- Redis create failed: "password must ... contain at least one digit,
  one uppercase, one lowercase and one special character".
  `Special:true` only widens the pool; minima default to 0.
- Fix: MinLower/MinUpper/MinNumeric/MinSpecial = 2 on the cache and
  database generators; mock-graph test pins the guarantee.
- Same run: fr-par Kapsule was in provider-side `shortage` (all cluster
  types); admission correctly refused. Cell moved to nl-ams-1, same
  shape. Catalog defaults also refreshed (Redis 8.6.6, Kapsule 1.36.4,
  lowercase node types).

## OVH run: pool stuck INSTALLING 60 min in eu-west-par-a (2026-09-15)
- KubeNodePool never left INSTALLING; provider plugin timed out after
  1h0m0s (16 resources created, pool failed). Managed DBs on the same
  b3-8 flavor converged in ~300s, so the stall is MKS-pool-specific.
- Next experiment: eu-west-par-b, same shape. Admission never validates
  the MKS node flavor (only DB flavors) - worth a follow-up.

## BUG (defer to order-12): destroy-after-expiry blocked in-program
- `magelift destroy` on an expired preview fails: CLI planning allows
  expired (AllowExpiredPreview=true) but component.New re-runs strict
  spec.Validate() inside the Pulumi program, which rejects expiry.
  Affects all five stack packages (aws, eksops, gcp, ovh, scaleway).
- Bit the OVH trap when the 3600s TTL expired during the 67-min pool
  timeout: destroy preview failed, trap looped, manual teardown needed.
- Fix shape: carry the allow-expired flag on the Spec into Program() so
  component.New skips ONLY the expiry check on destroy. Home: order-12
  "cleanup reconcile that finishes interrupted runs", with live proof.
- Workaround for order-11: TTL 7200 so expiry cannot hit mid-run.

## OVH refresh PARKED 2026-09-15: MKS pools do not converge (6 attempts)
- PAR+b3-8/eu-west-par-a: pool INSTALLING 60 min -> provider timeout.
  16 resources created; TTL expired mid-failure -> destroy blocked by
  the expiry bug (see below) -> manual teardown.
- PAR+b3-8/eu-west-par-b: pool INSTALLING, then the READY cluster
  vanished OVH-side (no local actor). Manual teardown of DBs+gateway
  +subnet+network.
- PAR+b2-8, PAR+b2-7, MIL+b2-7: instant 404 "Flavor not found".
  B2 is NOT in the MKS flavor capability list (docs table stale);
  only Gen-3 (b3/c3/r3, 23 flavors) is offered. b3-8 (2c/8GB) is the
  smallest balanced MKS flavor in PAR and MIL.
- MIL+b3-8: interrupted at 17 min while pool creating -> inconclusive.
- vlan-0 slot is project-global: DELETE reports success but the
  registration lingers; the next create 400s. Subnet delete fails on
  ports; network delete cascades anyway. Gateway quota is 1/region -
  an orphan gateway blocks new runs. Sweep gateways via API (the CLI
  has no network-private list).
- Project status ok, compute quota free. Managed DBs/network/gateway
  converge every time; only MKS pools fail. Verdict: PAR MKS pool
  provisioning degraded or broken today; MIL+b3-8 still untried.
- On retry: ONE clean MIL+b3-8 (or PAR+b3-8) run, TTL 7200, DO NOT
  INTERRUPT. 3 interrupts this session caused 3 manual teardowns.

## Order-12: `cost --live` never priced anything (2026-09-15)

- `magelift cost --live` on AWS returned every compute row as
  "unsupported / no matching AWS price". Cause: FIVE independent
  filter defects, all proven against the live Price List API:
- Location map: eu-west-3 "Europe (Paris)" and eu-south-1
  "Europe (Milan)" match NOTHING; all five queried services
  (ECS, ElastiCache, RDS, ES, MQ) use the "EU (X)" form.
  Fixed in `internal/cloud/aws/pricing/pricing.go`.
- Fargate products carry NO operatingSystem/preInstalledSw/
  capacitystatus attributes; the ECS queries filtered on all
  three, so they could never match in ANY region. Dropped to
  productFamily-only in `internal/cloud/aws/cost/estimate.go`.
- productFamily values: ElastiCache is "Cache Instance" (not
  "ElastiCache Instance"); OpenSearch is "Amazon OpenSearch
  Service Instance" (not "Amazon OpenSearch Service"); MQ is
  "Broker Instances" (not "RabbitMQ Broker").
- usagetype substrings: OpenSearch data nodes are "ESInstance"
  (not "InstanceUsage"); MQ cluster brokers are
  "RabbitMQ-3-InstanceUsage" (not "BrokerUsage"), fail-closed
  to honest unavailable on rename. MQ instanceType filter must
  strip the "mq." prefix (attribute is "m7g.medium" style).
- After the fix: Fargate vCPU $17.74, memory $3.87, Valkey
  $10.51/month live on eu-west-3 preview. RDS/OpenSearch/MQ
  shapes fixed but still await a session that runs them.

## Order-12: `cost --budget` page size (2026-09-15)

- `cost --budget` hard-failed; probe showed
  DescribeNotificationsForBudget MaxResults=1000 exceeds the API
  max of 100. Split `awsBudgetNotificationPageSize=100` in
  `internal/cloud/aws/cost/budget.go`. Preview correctly reports
  not-configured (no inheritance); staging-class read lists the
  five account budgets with amounts and the no-ownership notice.

## Order-12: allow-expired forgave everything (2026-09-15)

- The `isOnlyExpirationError` string check matched whenever the
  expiry error was PRESENT among others, so allow-expired
  planning forgave all defects at the CLI layer (the program
  re-check caught them, but only after a confusing plan).
- Replaced with AWS-style validator selection (`Validate` vs
  `ValidateAllowExpiredPreview`) in ovh/gcp/scaleway/eksops and
  deleted the three helpers. New per-provider tests pin:
  expired+flag plans AND carries the flag, strict still rejects,
  non-expiry defect still rejected.
