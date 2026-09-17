.DEFAULT_GOAL := help
.NOTPARALLEL:

# MageLift's Pulumi dependency graph is large. Keep local Go compilation and
# test scheduling bounded so one command cannot exhaust a contributor's Mac.
# This also keeps an accidental `make -j` from starting multiple builds.
export GOMAXPROCS := 1
export GOFLAGS := -p=1
export GOMEMLIMIT := 1GiB

.PHONY: help generate generate-check cli-docs cli-docs-check certification-docs certification-docs-check fmt fmt-check test sdk-test lint license-check check-clean-room php-test image-test frankenphp-image-test builder-image-test varnish-test build-e2e-test pulumi-mock-test floci-test-aws floci-gcp-test provider-gcp-build core-leanness local-gates acceptance-dependencies-check acceptance-harness-test aws-acceptance-local aws-recovery-acceptance-local aws-database-recovery-acceptance-local aws-secret-recovery-acceptance-local aws-sqs-acceptance-local aws-cloudwatch-acceptance-local aws-cloudfront-acceptance-local gcp-acceptance-local gcp-collector-acceptance-local gcp-cloudsql-acceptance-local gcp-cloudsql-destroy-retention-acceptance-local gcp-cloudsql-cleanup-ledger-acceptance-local gcp-recovery-acceptance-local gcp-secret-recovery-acceptance-local gcp-pubsub-acceptance-local gcp-observability-acceptance-local gcp-edge-acceptance-local ovh-acceptance-local ovh-recovery-acceptance-local ovh-database-recovery-acceptance-local scaleway-acceptance-local scaleway-recovery-acceptance-local scaleway-secret-recovery-acceptance-local scaleway-observability-acceptance-local fastly-acceptance-local newrelic-acceptance-local newrelic-otlp-acceptance-local skills-test extension-test docs docs-serve workflow-check verify release-smoke ci-act-go

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-36s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

fmt: ## Format Go sources
	gofmt -w $$(find . -type f -name '*.go' -not -path './vendor/*')

fmt-check: ## Check Go formatting without changing files
	test -z "$$(gofmt -l $$(find . -type f -name '*.go' -not -path './vendor/*'))"

test: ## Run Go tests with the race detector
	go test -race ./...

sdk-test: ## Run the SDK module suite standalone (no workspace)
	cd sdk && GOWORK=off go test -race ./... -count=1

provider-gcp-build: ## Build the autonomous GCP provider plugin binary
	go build -o dist/magelift-provider-gcp ./providers/gcp/cmd/magelift-provider-gcp

core-leanness: ## Prove the CLI carries no GCP provider code or GCP cloud SDKs
	test -z "$$(go list -deps ./cmd/magelift | grep -E 'magelift/providers/|magelift/internal/cloud/gcp|cloud\.google\.com/go|google\.golang\.org/api/')"
# NOTE: OVH/Scaleway SDKs stay linked (cleanup recovery needs them) but
# unregistered by default; see registry.NewDefault and ADR 0013. AWS ECS
# stays in-process by design until aws-provider-parity extracts it. Full
# provider-SDK independence is a post-parity gate, not this one.

lint: ## Run static Go checks
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./...

license-check: ## Check Go dependency licenses
	go run github.com/google/go-licenses/v2@v2.0.1 check ./... --disallowed_types=forbidden,unknown \
		--ignore=github.com/ovh/pulumi-ovh \
		--ignore=github.com/ovh/okms-sdk-go

check-clean-room: ## Fail if vendored ece-tools / cloud-patches / ACC cli trees appear
	./scripts/check-clean-room.sh

release-smoke: ## Serial single-target goreleaser smoke (safe on low-RAM Macs)
	./scripts/release-smoke-local.sh

ci-act-go: ## Run Go CI jobs locally via nektos/act (serial; no Actions minutes)
	./scripts/ci-act-go.sh

php-test: ## Validate and test the Composer package
	composer validate --working-dir=build --strict
	composer audit --working-dir=build --locked
	composer analyse --working-dir=build
	composer psalm --working-dir=build
	php build/vendor/bin/phpunit -c build/phpunit.xml --fail-on-deprecation --fail-on-notice --fail-on-warning

lifecycle-golden: ## Regenerate the Go migration-shell golden from the PHP lifecycle plan
	php build/bin/magelift-lifecycle-export > internal/platform/testdata/lifecycle-deploy.json

lifecycle-golden-check: ## Fail when the committed golden differs from the PHP plan
	php build/bin/magelift-lifecycle-export | diff -u internal/platform/testdata/lifecycle-deploy.json -

image-test: ## Build and inspect the local PHP runtime image
	docker buildx bake php-nginx --load
	test "$$(docker run --rm magelift/php-nginx:local id -u)" = 10001
	docker run --rm magelift/php-nginx:local php-fpm --test
	test "$$(docker run --rm --entrypoint nginx magelift/php-nginx:local -v 2>&1)" = "nginx version: nginx/1.30.4"
	docker run --rm --entrypoint nginx magelift/php-nginx:local -t -c /etc/nginx/nginx.conf
	docker run --rm --entrypoint nginx magelift/php-nginx:local -T -c /etc/nginx/nginx.conf | grep -q "location /media/"
	docker run --rm --read-only --tmpfs /tmp:uid=10001,gid=10001 magelift/php-nginx:local php -r 'file_put_contents(sys_get_temp_dir()."/probe", "ok");'
	docker run --rm magelift/php-nginx:local php -r '$$required = ["apcu", "bcmath", "ftp", "gd", "intl", "mbstring", "pdo_mysql", "redis", "soap", "sockets", "sodium", "xsl", "zip", "Zend OPcache"]; $$missing = array_values(array_filter($$required, fn(string $$extension): bool => !extension_loaded($$extension))); if ($$missing !== []) { fwrite(STDERR, "Missing PHP extensions: " . implode(", ", $$missing) . PHP_EOL); exit(1); }'
	./scripts/image-health-test.sh nginx

frankenphp-image-test: ## Build and inspect the FrankenPHP classic adapter
	docker buildx bake frankenphp-classic --load
	test "$$(docker run --rm --entrypoint id magelift/frankenphp-classic:8.5-local -u)" = 10001
	docker run --rm --entrypoint frankenphp magelift/frankenphp-classic:8.5-local version
	docker run --rm --entrypoint frankenphp magelift/frankenphp-classic:8.5-local validate --config /etc/frankenphp/Caddyfile --adapter caddyfile
	docker run --rm --entrypoint php magelift/frankenphp-classic:8.5-local -r '$$required = ["apcu", "bcmath", "curl", "dom", "fileinfo", "ftp", "gd", "iconv", "intl", "json", "mbstring", "mysqlnd", "Zend OPcache", "openssl", "pdo", "pdo_mysql", "pdo_sqlite", "redis", "soap", "sockets", "sodium", "xml", "xmlreader", "xmlwriter", "xsl", "zip"]; $$missing = array_values(array_filter($$required, fn(string $$extension): bool => !extension_loaded($$extension))); if ($$missing !== []) { fwrite(STDERR, "Missing PHP extensions: " . implode(", ", $$missing) . PHP_EOL); exit(1); }'
	./scripts/frankenphp-tls-test.sh
	./scripts/image-health-test.sh frankenphp

builder-image-test: ## Build and inspect the isolated PHP build runner image
	docker buildx bake php-builder --load
	test "$$(docker run --rm --entrypoint id magelift/php-builder:local -u)" = 10001
	docker run --rm --entrypoint composer magelift/php-builder:local --version

varnish-test: ## Verify the integrated Varnish sidecar security contract
	./scripts/varnish-test.sh

build-e2e-test: builder-image-test ## Build the fixture through the CLI, Docker, and BuildKit
	./scripts/build-e2e.sh

pulumi-mock-test: ## Run Pulumi mock graph tests for cloud adapters (no account)
	go test -race ./internal/cloud/... -count=1

floci-test-aws: ## Run account-free AWS contract tests through Floci
	./scripts/floci-test-aws.sh

floci-gcp-test: ## Run account-free GCP contract tests through floci-gcp
	./providers/gcp/scripts/floci-gcp-test.sh

local-gates: pulumi-mock-test acceptance-harness-test floci-test-aws floci-gcp-test ## Account-free stack: Pulumi mocks, harness, Floci AWS/GCP

# Offline harness steps print a label and die at a ceiling so a stuck child
# cannot hang `make acceptance-harness-test` with no output. Shape/dry-run
# checks finish in seconds. evidence_seal builds ./cmd/magelift with all
# cores (see the test) and can take several minutes on a cold cache.
HARNESS_TEST_TIMEOUT ?= 120
HARNESS_BUILD_TIMEOUT ?= 900
HARNESS_STEP := bash scripts/acceptance/run-harness-step.sh

acceptance-harness-test: acceptance-dependencies-check ## Run offline acceptance harness shell tests (serial; no AWS)
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) harness_step_timeout_test.sh -- bash tests/acceptance/harness_step_timeout_test.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) evidence_append_test.sh -- bash tests/acceptance/evidence_append_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) lifecycle_guard_test.sh -- bash tests/acceptance/lifecycle_guard_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) cloudflare_dns_helper_test.sh -- bash tests/acceptance/cloudflare_dns_helper_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) lib_cosign_test.sh -- bash tests/acceptance/lib_cosign_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) campaign_isolation_test.sh -- bash tests/acceptance/campaign_isolation_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) aws_seed_task_definition_test.sh -- bash tests/acceptance/aws_seed_task_definition_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) shared_evidence_test.sh -- bash tests/acceptance/shared_evidence_test.sh
	@$(HARNESS_STEP) $(HARNESS_BUILD_TIMEOUT) 'evidence_seal_test.sh (builds magelift; can take several minutes)' -- bash tests/acceptance/evidence_seal_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) aws_harness_shape_test.sh -- bash tests/acceptance/aws_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) aws_database_recovery_harness_shape_test.sh -- bash tests/acceptance/aws_database_recovery_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) scaleway_database_recovery_harness_shape_test.sh -- bash tests/acceptance/scaleway_database_recovery_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) ovh_database_recovery_harness_shape_test.sh -- bash tests/acceptance/ovh_database_recovery_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) aws_cloudfront_harness_shape_test.sh -- bash tests/acceptance/aws_cloudfront_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_BUILD_TIMEOUT) 'gcp_edge_harness_shape_test.sh (go run magento-waf-rules)' -- bash tests/acceptance/gcp_edge_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) checkpoint_fingerprint_test.sh -- bash tests/acceptance/checkpoint_fingerprint_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) cleanup_ledger_helper_test.sh -- bash tests/acceptance/cleanup_ledger_helper_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) k8s_harness_shape_test.sh -- bash tests/acceptance/k8s_harness_shape_test.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) checkpoint_resume_test.sh -- bash tests/acceptance/checkpoint_resume_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) checkpoint_run_id_test.sh -- bash tests/acceptance/checkpoint_run_id_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) checkpoint_cleanup_state_test.sh -- bash tests/acceptance/checkpoint_cleanup_state_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) recovery_lifecycle_guard_test.sh -- bash tests/acceptance/recovery_lifecycle_guard_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) acceptance_lifecycle_guard_test.sh -- bash tests/acceptance/acceptance_lifecycle_guard_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) matrix_tier_guard_test.sh -- bash tests/acceptance/matrix_tier_guard_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) port_coverage_floci_gate_test.sh -- bash tests/acceptance/port_coverage_floci_gate_test.sh
	@MAGELIFT_ACCEPTANCE_AWS_STUB=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) 'assert_clean_stub_test.sh --clean' -- bash tests/acceptance/assert_clean_stub_test.sh --clean
	@MAGELIFT_ACCEPTANCE_AWS_STUB=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) 'assert_clean_stub_test.sh --leftover' -- bash tests/acceptance/assert_clean_stub_test.sh --leftover; test $$? -eq 1
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) assert_clean_prerequisite_test.sh -- bash tests/acceptance/assert_clean_prerequisite_test.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp_harness_shape_test.sh -- bash tests/acceptance/gcp_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp_collector_harness_shape_test.sh -- bash tests/acceptance/gcp_collector_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp_gke_native_observability_harness_shape_test.sh -- bash tests/acceptance/gcp_gke_native_observability_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) aws_collector_harness_shape_test.sh -- bash tests/acceptance/aws_collector_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp_cloudsql_harness_shape_test.sh -- bash tests/acceptance/gcp_cloudsql_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp_cloudsql_destroy_retention_harness_shape_test.sh -- bash tests/acceptance/gcp_cloudsql_destroy_retention_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp_cloudsql_cleanup_ledger_harness_shape_test.sh -- bash tests/acceptance/gcp_cloudsql_cleanup_ledger_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp_recovery_harness_shape_test.sh -- bash tests/acceptance/gcp_recovery_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp_secret_recovery_harness_shape_test.sh -- bash tests/acceptance/gcp_secret_recovery_harness_shape_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) cold_baseline_ledger_test.sh -- bash tests/acceptance/cold_baseline_ledger_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) magento_env_contract_test.sh -- bash tests/acceptance/magento_env_contract_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) seed_dump_contract_test.sh -- bash tests/acceptance/seed_dump_contract_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) custom_extension_clean_cache_test.sh -- bash tests/acceptance/custom_extension_clean_cache_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) newrelic_harness_shape_test.sh -- bash tests/acceptance/newrelic_harness_shape_test.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) ovh-acceptance-local.sh -- bash scripts/ovh-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) scaleway-acceptance-local.sh -- bash scripts/scaleway-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) aws-cloudwatch-acceptance-local.sh -- bash scripts/aws-cloudwatch-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) aws-recovery-acceptance-local.sh -- bash scripts/aws-recovery-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) aws-database-recovery-acceptance-local.sh -- bash scripts/aws-database-recovery-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) aws-secret-recovery-acceptance-local.sh -- bash scripts/aws-secret-recovery-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) ovh-recovery-acceptance-local.sh -- bash scripts/ovh-recovery-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp-observability-acceptance-local.sh -- bash providers/gcp/scripts/gcp-observability-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp-recovery-acceptance-local.sh -- bash providers/gcp/scripts/gcp-recovery-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp_pubsub_harness_shape_test.sh -- bash tests/acceptance/gcp_pubsub_harness_shape_test.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) gcp-secret-recovery-acceptance-local.sh -- bash providers/gcp/scripts/gcp-secret-recovery-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) scaleway-recovery-acceptance-local.sh -- bash scripts/scaleway-recovery-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) scaleway-secret-recovery-acceptance-local.sh -- bash scripts/scaleway-secret-recovery-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) scaleway-observability-acceptance-local.sh -- bash scripts/scaleway-observability-acceptance-local.sh
	@MAGELIFT_ACCEPTANCE_DRY_RUN=1 $(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) scaleway-database-recovery-acceptance-local.sh -- bash scripts/scaleway-database-recovery-acceptance-local.sh

acceptance-dependencies-check: ## Verify jq/yq v4 and shared acceptance command preflight before mutation
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) dependency_preflight_test.sh -- bash tests/acceptance/dependency_preflight_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) release_checks_gate_test.sh -- bash tests/acceptance/release_checks_gate_test.sh

skills-test: ## Test bundled skill installation and verification
	GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/skills

installer-harness-test: ## Run the installer trust harness against local fixtures
	bash scripts/install-verify-harness.sh

extension-test: ## Build the public extension SDK contract with empty caches
	./scripts/custom-extension-clean-room.sh

aws-acceptance-local: ## Run a local real-AWS acceptance pass (preview; destroys on exit)
	./scripts/aws-acceptance-local.sh

aws-sqs-acceptance-local: ## Run a disposable real-AWS SQS bounded recovery cell (destroys on exit)
	./scripts/aws-sqs-acceptance-local.sh

aws-recovery-acceptance-local: ## Run a disposable real-AWS S3 recovery cell (destroys on exit)
	./scripts/aws-recovery-acceptance-local.sh

aws-database-recovery-acceptance-local: ## Run a disposable real-AWS RDS backup/restore cell (destroys on exit)
	./scripts/aws-database-recovery-acceptance-local.sh

aws-secret-recovery-acceptance-local: ## Run a disposable real-AWS Secrets Manager recovery cell (destroys on exit)
	./scripts/aws-secret-recovery-acceptance-local.sh

aws-cloudwatch-acceptance-local: ## Run a disposable real-AWS CloudWatch native observability cell (destroys on exit)
	./scripts/aws-cloudwatch-acceptance-local.sh

aws-cloudfront-acceptance-local: ## Run a disposable real-AWS CloudFront/WAF cell (destroys on exit)
	./scripts/aws-cloudfront-acceptance-local.sh

gcp-acceptance-local: ## Run a local real-GCP acceptance pass (experimental; destroys on exit)
	./providers/gcp/scripts/gcp-acceptance-local.sh

gcp-collector-acceptance-local: ## Run a disposable GKE collector and New Relic signal cell (requires explicit live gate)
	./providers/gcp/scripts/gcp-collector-acceptance-local.sh

gcp-cloudsql-acceptance-local: ## Run a disposable real-GCP Cloud SQL backup/restore cell (destroys on exit). MAGELIFT_GCP_CLOUDSQL_DESTINATION=in-place for source overwrite.
	./providers/gcp/scripts/gcp-cloudsql-acceptance-local.sh

gcp-cloudsql-destroy-retention-acceptance-local: ## Run a disposable real-GCP Cloud SQL final-backup leftover deletion cell (destroys on exit).
	./providers/gcp/scripts/gcp-cloudsql-destroy-retention-acceptance-local.sh

gcp-cloudsql-cleanup-ledger-acceptance-local: ## Run a disposable real-GCP Cloud SQL interrupt-and-reconcile cell (destroys on exit).
	./providers/gcp/scripts/gcp-cloudsql-cleanup-ledger-acceptance-local.sh

gcp-pubsub-acceptance-local: ## Run a disposable real-GCP Pub/Sub snapshot/seek cell (destroys on exit). MAGELIFT_GCP_PUBSUB_RECOVERY_DESTINATION=isolated for a new owned subscription.
	./providers/gcp/scripts/gcp-pubsub-acceptance-local.sh

gcp-observability-acceptance-local: ## Run a disposable real-GCP native observability cell (destroys on exit)
	./providers/gcp/scripts/gcp-observability-acceptance-local.sh

gcp-edge-acceptance-local: ## Run a disposable real-GCP Cloud CDN/Armor cell (destroys on exit)
	./providers/gcp/scripts/gcp-edge-acceptance-local.sh

gcp-recovery-acceptance-local: ## Run a disposable real-GCP Cloud Storage recovery cell (destroys on exit). MAGELIFT_GCP_RECOVERY_DESTINATION=in-place for source overwrite. MAGELIFT_GCP_RECOVERY_DATA_CLASS=infrastructure-state|audit-evidence|media.
	./providers/gcp/scripts/gcp-recovery-acceptance-local.sh

gcp-secret-recovery-acceptance-local: ## Run a disposable real-GCP Secret Manager recovery cell (destroys on exit). MAGELIFT_GCP_SECRET_RECOVERY_DESTINATION=in-place for source overwrite.
	./providers/gcp/scripts/gcp-secret-recovery-acceptance-local.sh

ovh-acceptance-local: ## Run a local real-OVH MKS acceptance pass (experimental; destroys on exit)
	./scripts/ovh-acceptance-local.sh

ovh-recovery-acceptance-local: ## Run a disposable real-OVHcloud Object Storage recovery cell (destroys on exit)
	./scripts/ovh-recovery-acceptance-local.sh

ovh-database-recovery-acceptance-local: ## Run a disposable real-OVH Managed Database recovery cell (destroys on exit)
	./scripts/ovh-database-recovery-acceptance-local.sh

scaleway-acceptance-local: ## Run a local real-Scaleway Kapsule acceptance pass (experimental; destroys on exit)
	./scripts/scaleway-acceptance-local.sh

scaleway-recovery-acceptance-local: ## Run a disposable real-Scaleway Object Storage recovery cell (destroys on exit)
	./scripts/scaleway-recovery-acceptance-local.sh

scaleway-secret-recovery-acceptance-local: ## Run a disposable real-Scaleway Secret Manager recovery cell (destroys on exit)
	./scripts/scaleway-secret-recovery-acceptance-local.sh

scaleway-observability-acceptance-local: ## Run a disposable real-Scaleway Cockpit observability cell (destroys on exit)
	./scripts/scaleway-observability-acceptance-local.sh

scaleway-database-recovery-acceptance-local: ## Run a disposable real-Scaleway Managed Database recovery cell (destroys on exit)
	./scripts/scaleway-database-recovery-acceptance-local.sh

fastly-acceptance-local: ## Run a disposable real-Fastly adapter acceptance pass (experimental; destroys on exit)
	./scripts/fastly-acceptance-local.sh

newrelic-acceptance-local: ## Run a bounded real-New Relic data-plane acceptance probe (no provider resource)
	./scripts/newrelic-acceptance-local.sh

newrelic-otlp-acceptance-local: ## Run disposable live Go OTLP/New Relic acceptance and revoke its ingest key on exit
	./scripts/newrelic-otlp-acceptance-local.sh

docs: ## Build documentation with strict link and navigation checks
	"$$(scripts/docs-venv.sh)/mkdocs" build --strict

docs-serve: ## Serve docs locally with the pinned MkDocs Material stack
	"$$(scripts/docs-venv.sh)/mkdocs" serve

workflow-check: ## Validate GitHub Actions workflow syntax and expressions
	go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 \
		.github/workflows/*.yml \
		examples/.github/workflows/*.yml \
		examples/sample-shop/.github/workflows/*.yml \
		tests/fixtures/ci/aws/.github/workflows/*.yml \
		tests/fixtures/ci/gcp/.github/workflows/*.yml

verify: generate-check cli-docs-check fmt-check lint test sdk-test license-check check-clean-room php-test docs workflow-check ## Run the local verification suite
generate: certification-docs ## Generate configuration, certification, reference, and skill artifacts
	go generate ./internal/config ./agents

generate-check: certification-docs-check ## Check generated configuration and certification files for drift
	go run ./cmd/genconfig --check

cli-docs: ## Generate the CLI reference from the Cobra command tree
	go run ./cmd/gendocs

cli-docs-check: ## Check the generated CLI reference for drift
	go run ./cmd/gendocs --check

certification-docs: ## Generate source-dated capability, architecture, and resilience coverage
	go run ./cmd/gencertdocs

certification-docs-check: ## Check source-dated certification coverage for drift
	go run ./cmd/gencertdocs --check
