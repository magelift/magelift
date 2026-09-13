## 1. Planning artifacts

- [x] 1.1 Promote existing change specs into `openspec/specs/` as the main product baseline.
- [x] 1.2 Write missing product capabilities and additive requirements listed in `proposal.md`.
- [x] 1.3 Replace `openspec/BACKLOG.md` with one ordered execution index and an explicit active implementation objective.
- [x] 1.4 Point remaining live-evidence work at `provider-native-lifecycle-adapters` and `multi-cloud-resilience-observability-edge` instead of copying those checkboxes here.

## 2. Gate

P1.1 GCP public TLS / Magento-origin / Armor data-plane is closed. `openspec/BACKLOG.md` (2026-08-18 TTM order) names packed GCP session 1 as the active cloud objective. Pre-KEEP local CLI (3.2, 3.3, 3.4, 4.2) has unit proof. Live dump-retrieve, rollback-vs-schema, and SendGrid delivery attach to session 1.

- [x] 2.1 Close or reassign the BACKLOG active objective so this change's P1 rows are eligible. Reassigned 2026-08-18: session 1 KEEP is unblocked after 4.2.

## 3. P1 — new contracts not owned by the in-progress provider changes

Reconciled 2026-08-18 against the Go graph and `docs/knowledge`. Do not reimplement closed rows. **TTM:** session 1 KEEP.

- [x] 3.1 CLI contract tests for `--no-interaction`, `--yes` on destructive commands, `--output json`/`yaml`/`table`, and exit codes 0/2/3/4. Covered by `internal/cli/operator_contract_test.go`, `health_test.go` (code 4), `root_test.go` (non-interactive env selection), and `--yes` tests in env/state/secrets/dev/releases. Knowledge: `docs/knowledge/reference/MageLift operator output compatibility baseline 2026-08-14.md`.
- [x] 3.2 Add explicit health layers (infrastructure, service, Magento, dependency, deployment) as a `layer` field on `health.Check`. `ClassifyLayer` maps known probe IDs; adapters may set `RuntimeHealth.Layer` when they can observe a class. Overall status is still the worst check, so Magento down with a healthy database is unhealthy and exit 4 (`TestHealthRuntimeMagentoDownExitsUnhealthy`). `doctor` remains local-only and does not emit runtime health. Magento-wired live probes stay session 1. **KEEP unit proof.**
- [x] 3.3 Add dump **from a live environment** and retrieve that file locally. `magelift env dump <environment> --to <path>` runs mysqldump over the dumpimport transport (host / compose / kube), writes only to `--to`, labels the dump `unsanitized Magento data`, and does not print `MAGELIFT_DUMPIMPORT_PASSWORD`. Overwrite requires `--yes`. Covered by `TestExportKubeWritesLocalFileWithoutPasswordOnArgv` and `TestEnvDumpWritesLocalFileAndLabelsUnsanitized`. Live retrieve stays session 1. **KEEP unit proof.**
- [x] 3.4 Fail closed when rollback targets a digest that cannot run on the live schema. `RefuseIncompatibleRollback` compares recorded `schemaEpoch` values and refuses before cutover when live is newer, or when either epoch is missing. Errors name restore or forward-fix. `--ack-forward-only` still required; migrations are never reversed. Covered by `TestRefuseIncompatibleRollback` and `TestRollbackRefusesOlderSchemaBeforeCutover`. Live Magento `setup:db:status` wiring stays session 1. **KEEP unit proof.**
- [x] 3.5 `config migrate` loads through `config.Load` (unknown core fields fail), canonicalizes schema v1, and keeps secret references (`Migrate`, `TestConfigMigrateNormalizesCurrentSchema`, `TestStrictUnknownKey`, `TestSafeEffectiveOutputPreservesSecretReferences`). Older schema versions are rejected, not rewritten, because only v1 exists.
- [x] 3.6 Exit codes are in `docs/operations.md`. Local commands are `magelift dev`, documented in `docs/local-vs-cloud.md` and generated `docs/cli-reference.md`. There is no `magelift local` alias.

## 4. P1 remaining that session 1 consumes / P2 after RC1

- [x] 4.1 Keep preview budgets from displaying production limits; flag remaining expensive preview catalog choices. AWS preview × `amazon-mq` already fail-closes before a broker is registered (`assertPreviewAmazonMQRejected`). `--budget` on preview reports `not-configured` without account budget amounts (`TestEstimatorPreviewBudgetDoesNotInheritAccountBudgets`); account-free mode flags Amazon MQ, provisioned OpenSearch, and multi-instance Aurora (`TestAccountFreePreviewFlagsExpensiveCatalog`).
- [x] 4.2 Add **cloud** transactional email validation for SendGrid and SES secret references on fixed environments. `email.credential` must be a provider-matching secret reference (`aws-secrets-manager://` / `ssm://` on AWS, `gcp-secret-manager://` on GCP). SendGrid without a credential fails and names `email.credential`. Preview environments that omit `email` resolve to `disabled` and do not inherit production SendGrid/SES credentials. Covered by `TestCloudSendGridRequiresCredentialSecret`, `TestCloudSendGridValidatesSecretReference`, `TestCloudSESValidatesSecretReference`, `TestPreviewOmitsEmailDoesNotReuseProductionSendGrid`. Local `sendgrid`/`ses` SMTP wiring (`normalizeEmailSettings`) is not a cloud delivery adapter. Live SendGrid delivery is session 1 (`live.integration`). SES cloud delivery is not an RC1 Magento cell. **KEEP unit proof.**
- [ ] 4.3 Publish Homebrew cask and Scoop artifacts with checksums and signatures after the first public tag. Org repos already exist (`magelift/homebrew-tap`, `magelift/scoop-bucket`); `.goreleaser.yaml` already publishes into them. `gh release list --repo magelift/magelift` is empty. **Blocked** until a public GitHub release tag exists. Do not create new tap/bucket repos.
- [x] 4.4 Export reconstructable production change evidence (digest, actor, config provenance, backup policy) without secret values. `magelift evidence` (`TestEvidenceExportsDigestActorProvenanceWithoutSecrets`).
- [x] 4.5 User skill bundle is the five `agents/skills/` names only (`TestBundledManifestIsCompleteAndStable`). Contributor skills, humanizer, and watermarks are not in that list.

## 5. P3 — explicit nice-to-have

- [x] 5.1a `local.email.mode: mailpit` used to fail closed; superseded by 5.1 once the image contract existed.
- [x] 5.1 Add a verified Mailpit image, health check, and Magento SMTP wiring. Pinned `axllent/mailpit:v1.30.7`, `/mailpit readyz`, Compose profile `email`, Magento SMTP to `mailpit:1025` (`TestPlanMailpitWiresSMTPAndComposeService`).
- [x] 5.2 Specify and implement dump sanitization as an opt-in; until then label dumps unsanitized. `env dump --sanitize` hashes mailboxes (`TestSanitizeSQLHashesMailboxesAndLeavesDefiners`, `TestEnvDumpSanitizePassesOptInToExporter`). Default remains `unsanitized Magento data`.
- [x] 5.3 FrankenPHP worker mode. `application.webRuntime: frankenphp-worker` is accepted; AWS/GCP run `frankenphp run --worker /app/public/index.php` (`TestRuntimeFrankenPHPWorkerUsesWorkerCommand`). nginx-fpm remains the default.
- [x] 5.4 Optional provider management UIs behind capability detection. `magelift env ui` prints a session-only tunnel; missing UIs exit 3 without blocking dump (`TestEnvUIUnsupportedDoesNotBlockDump`).
