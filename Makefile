.DEFAULT_GOAL := help
.NOTPARALLEL:

# Soft-cap Go at 75% of available RAM so a Pulumi-sized compile cannot
# take the host. Do not pin GOMAXPROCS or -p; the toolchain schedules
# those. .NOTPARALLEL only blocks accidental `make -j` of two heavy
# Makefile targets.
unexport GOMAXPROCS
export GOMEMLIMIT := $(shell "$(CURDIR)/scripts/go-memlimit.sh")

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-36s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# PATH gofmt can be an older distro binary (for example /usr/bin/gofmt -> go-1.26).
# CI and make fmt-check must use the go.mod toolchain so composite literals
# do not flip between indent styles.
GOFMT := $(shell go env GOROOT)/bin/gofmt

fmt: ## Format Go sources
	$(GOFMT) -w $$(find . -type f -name '*.go' -not -path './vendor/*')

fmt-check: ## Check Go formatting without changing files
	test -z "$$($(GOFMT) -l $$(find . -type f -name '*.go' -not -path './vendor/*'))"

test: test-fast ## Run all three Go module suites without race

test-fast: ## Fast Go tests for root, sdk, and providers/gcp (no race)
	./scripts/go-test-modules.sh

test-community:
	./scripts/go-test-modules.sh --community

test-race: ## Race checks for provider/CLI lifecycle, SDK, and GCP operations
	./scripts/go-test-race-targeted.sh

test-race-full: ## Full root-module race suite (explicit broad check, excludes nested modules)
	go test -race ./...

sdk-test: ## Run the SDK module suite standalone with race (GOWORK=off hermeticity)
	cd sdk && GOWORK=off go test -race ./... -count=1

provider-gcp-test: ## Run the GCP provider module suite (workspace; no race)
	cd providers/gcp && go test ./... -count=1

community-test-helpers: ## Regression tests for the make, Go, and PHP helper scripts
	bash tests/acceptance/go_test_modules_test.sh
	bash tests/acceptance/php_test_helper_test.sh
	bash tests/acceptance/make_community_targets_test.sh

development-loop-test: ## Offline independent gates and GCP acceptance preflight regressions
	bash tests/acceptance/development_loop_runner_test.sh
	bash tests/acceptance/artifact_gate_test.sh
	bash tests/acceptance/gcp_product_gate_test.sh
	bash tests/acceptance/gcp_acceptance_preflight_test.sh
	bash tests/acceptance/gcp_acceptance_ownership_test.sh
	bash tests/acceptance/development_loop_gate_test.sh

verify-community: test-community test-race php-test lifecycle-golden-check check-clean-room core-leanness community-test-helpers development-loop-test ## Community alpha gate (modules + off-cloud path; no unbounded race)

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

php-test: ## Validate and test the Composer package (host PHP or pinned container)
	./scripts/php-test.sh

lifecycle-golden: ## Regenerate the Go migration-shell golden from the PHP lifecycle plan
	./scripts/php-test.sh --lifecycle-golden > internal/platform/testdata/lifecycle-deploy.json

lifecycle-golden-check: ## Fail when the committed golden differs from the PHP plan
	@actual=$$(mktemp); trap 'rm -f "$$actual"' EXIT; ./scripts/php-test.sh --lifecycle-golden >"$$actual" && diff -u internal/platform/testdata/lifecycle-deploy.json "$$actual"

image-test: ## Build and inspect the local PHP runtime image
	docker buildx bake php-nginx --load
	test "$$(docker run --rm magelift/php-nginx:local id -u)" = 10001
	docker run --rm magelift/php-nginx:local php-fpm --test
	test "$$(docker run --rm --entrypoint nginx magelift/php-nginx:local -v 2>&1)" = "nginx version: nginx/1.30.4"
	docker run --rm --entrypoint nginx magelift/php-nginx:local -t -c /etc/nginx/nginx.conf
	docker run --rm --entrypoint nginx magelift/php-nginx:local -T -c /etc/nginx/nginx.conf | grep -q "location /media/"
	for prefix in customer downloadable import custom_options; do docker run --rm --entrypoint nginx magelift/php-nginx:local -T -c /etc/nginx/nginx.conf | grep -q "location ^~ /media/$$prefix/" || exit 1; done
	docker run --rm --entrypoint nginx magelift/php-nginx:local -T -c /etc/nginx/nginx.conf | grep -q "theme_customization"
	docker run --rm --entrypoint nginx magelift/php-nginx:local -T -c /etc/nginx/nginx.conf | grep -q "index|get|static"
	./scripts/nginx-media-deny-test.sh
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

# mode step. fast and dry use the 120s ceiling. build uses the long ceiling.
# clean and leftover pass an extra argument to the same stub test.
# A *_test.sh path is tests/acceptance. A gcp- script is the GCP provider
# runner. Every other name is a root scripts/ runner.
HARNESS_STEPS := \
	fast harness_step_timeout_test.sh \
	fast development_loop_gate_test.sh \
	fast development_loop_runner_test.sh \
	fast artifact_gate_test.sh \
	fast gcp_product_gate_test.sh \
	fast gcp_acceptance_preflight_test.sh \
	fast gcp_acceptance_ownership_test.sh \
	fast release_canary_predicate_test.sh \
	dry evidence_append_test.sh \
	fast lifecycle_guard_test.sh \
	fast cloudflare_dns_helper_test.sh \
	fast cutover_dns_cloudflare_test.sh \
	fast lib_cosign_test.sh \
	fast go_memlimit_test.sh \
	fast aws_dry_run_no_account_test.sh \
	fast gcp_dry_run_no_account_test.sh \
	fast scaleway_dry_run_no_account_test.sh \
	fast campaign_isolation_test.sh \
	fast aws_seed_task_definition_test.sh \
	fast shared_evidence_test.sh \
	build evidence_seal_test.sh \
	fast aws_harness_shape_test.sh \
	fast aws_database_recovery_harness_shape_test.sh \
	fast scaleway_database_recovery_harness_shape_test.sh \
	fast ovh_database_recovery_harness_shape_test.sh \
	fast aws_cloudfront_harness_shape_test.sh \
	build gcp_edge_harness_shape_test.sh \
	fast checkpoint_fingerprint_test.sh \
	fast cleanup_ledger_helper_test.sh \
	fast k8s_harness_shape_test.sh \
	dry checkpoint_resume_test.sh \
	fast checkpoint_run_id_test.sh \
	fast checkpoint_cleanup_state_test.sh \
	fast recovery_lifecycle_guard_test.sh \
	fast acceptance_lifecycle_guard_test.sh \
	fast matrix_tier_guard_test.sh \
	fast port_coverage_floci_gate_test.sh \
	clean assert_clean_stub_test.sh \
	leftover assert_clean_stub_test.sh \
	fast assert_clean_prerequisite_test.sh \
	dry gcp_harness_shape_test.sh \
	fast gcp_collector_harness_shape_test.sh \
	fast gcp_gke_native_observability_harness_shape_test.sh \
	fast aws_collector_harness_shape_test.sh \
	fast gcp_cloudsql_harness_shape_test.sh \
	fast gcp_cloudsql_destroy_retention_harness_shape_test.sh \
	fast gcp_cloudsql_cleanup_ledger_harness_shape_test.sh \
	fast gcp_recovery_harness_shape_test.sh \
	fast gcp_secret_recovery_harness_shape_test.sh \
	fast cold_baseline_ledger_test.sh \
	fast magento_env_contract_test.sh \
	fast seed_dump_contract_test.sh \
	fast custom_extension_clean_cache_test.sh \
	fast newrelic_harness_shape_test.sh \
	dry ovh-acceptance-local.sh \
	dry scaleway-acceptance-local.sh \
	dry aws-cloudwatch-acceptance-local.sh \
	dry aws-recovery-acceptance-local.sh \
	dry aws-database-recovery-acceptance-local.sh \
	dry aws-secret-recovery-acceptance-local.sh \
	dry ovh-recovery-acceptance-local.sh \
	dry gcp-observability-acceptance-local.sh \
	dry gcp-recovery-acceptance-local.sh \
	dry gcp_pubsub_harness_shape_test.sh \
	dry gcp-secret-recovery-acceptance-local.sh \
	dry scaleway-recovery-acceptance-local.sh \
	dry scaleway-secret-recovery-acceptance-local.sh \
	dry scaleway-observability-acceptance-local.sh \
	dry scaleway-database-recovery-acceptance-local.sh

acceptance-harness-test: acceptance-dependencies-check ## Run offline acceptance harness shell tests (serial; no AWS)
	@set -eu; \
	set -- $(HARNESS_STEPS); \
	while [ "$$#" -gt 0 ]; do \
	mode="$$1"; step="$$2"; shift 2; \
	case "$$step" in \
	*_test.sh) path="tests/acceptance/$$step" ;; \
	gcp-*) path="providers/gcp/scripts/$$step" ;; \
	*) path="scripts/$$step" ;; \
	esac; \
	timeout="$(HARNESS_TEST_TIMEOUT)"; \
	label="$$step"; \
	extra=""; \
	prefix=""; \
	case "$$mode" in \
	fast) ;; \
	dry) prefix="MAGELIFT_ACCEPTANCE_DRY_RUN=1" ;; \
	build) timeout="$(HARNESS_BUILD_TIMEOUT)"; \
	case "$$step" in \
	evidence_seal_test.sh) label="evidence_seal_test.sh (builds magelift; can take several minutes)" ;; \
	gcp_edge_harness_shape_test.sh) label="gcp_edge_harness_shape_test.sh (go run magento-waf-rules)" ;; \
	esac ;; \
	clean) prefix="MAGELIFT_ACCEPTANCE_AWS_STUB=1"; extra="--clean"; label="assert_clean_stub_test.sh --clean" ;; \
	leftover) prefix="MAGELIFT_ACCEPTANCE_AWS_STUB=1"; extra="--leftover"; label="assert_clean_stub_test.sh --leftover" ;; \
	*) printf 'unknown harness mode %s\n' "$$mode" >&2; exit 2 ;; \
	esac; \
	set +e; \
	env $$prefix $(HARNESS_STEP) "$$timeout" "$$label" -- bash "$$path" $$extra; \
	rc=$$?; \
	set -e; \
	if [ "$$mode" = leftover ]; then test "$$rc" -eq 1; else test "$$rc" -eq 0; fi; \
	done

acceptance-dependencies-check: ## Verify jq/yq v4 and shared acceptance command preflight before mutation
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) dependency_preflight_test.sh -- bash tests/acceptance/dependency_preflight_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) release_checks_gate_test.sh -- bash tests/acceptance/release_checks_gate_test.sh
	@$(HARNESS_STEP) $(HARNESS_TEST_TIMEOUT) nginx_media_deny_shape_test.sh -- bash tests/acceptance/nginx_media_deny_shape_test.sh

skills-test: ## Test bundled skill installation and verification
	go test ./internal/skills

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

verify: generate-check cli-docs-check fmt-check lint test-fast test-race-full sdk-test license-check check-clean-room php-test docs workflow-check ## Run the local verification suite
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

# Every named target is an action. Derived so a new target cannot miss .PHONY.
.PHONY: $(shell awk -F: '/^[a-zA-Z0-9_-]+:/ {print $$1}' $(firstword $(MAKEFILE_LIST)))
