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
GO_SOURCES := find . \( -name .magelift -o -name vendor -o -name .git -o -name node_modules \) -prune -o -type f -name '*.go' -print0

fmt: ## Format Go sources
	$(GO_SOURCES) | xargs -0 $(GOFMT) -w

fmt-check: ## Check Go formatting without changing files
	test -z "$$($(GO_SOURCES) | xargs -0 $(GOFMT) -l)"

test: test-fast ## Run all Go module suites without race

test-fast: ## Fast Go tests for the CLI, SDK, and nested providers (no race)
	./scripts/go-test-modules.sh

test-race: ## Race checks for the CLI, SDK, provider plugins and operations
	./scripts/go-test-race-targeted.sh

test-race-full: ## Full root-module race suite (explicit broad check, excludes nested modules)
	go test -race ./...

sdk-test: ## Run the SDK module suite standalone with race (GOWORK=off hermeticity)
	cd sdk && GOWORK=off go test -race ./... -count=1

provider-tidy-check: ## Check provider dependency files against the current CLI and SDK
	@for manifest in providers/*/go.mod; do \
		[ -f "$$manifest" ] || continue; \
		provider=$${manifest#providers/}; provider=$${provider%/go.mod}; \
		bash scripts/check-provider-tidy.sh "$$provider" || exit; \
	done

provider-aws-test: ## Run the AWS provider module suite (workspace; no race)
	cd providers/aws && go test ./... -count=1

provider-gcp-test: ## Run the GCP provider module suite (workspace; no race)
	cd providers/gcp && go test ./... -count=1

provider-azure-test: ## Run the Azure provider module suite (workspace; no race)
	cd providers/azure && go test ./... -count=1

script-test: ## Run shell regression tests without cloud credentials
	@for name in go_memlimit go_test_modules make_community_targets php_test_helper \
		port_coverage_floci_gate release_checks_gate nginx_media_deny_shape \
		cutover_dns_cloudflare custom_extension_clean_cache release_publish; do \
		bash tests/scripts/$${name}_test.sh || exit; \
	done

verify-community: test-fast test-race php-test lifecycle-golden-check check-clean-room core-leanness script-test ## Verify the CLI, providers, build package, and scripts

provider-gcp-build: ## Build the autonomous GCP provider plugin binary
	go build -o dist/magelift-provider-gcp ./providers/gcp/cmd/magelift-provider-gcp

provider-aws-build: ## Build the autonomous AWS provider plugin binary
	go build -o dist/magelift-provider-aws ./providers/aws/cmd/magelift-provider-aws

provider-azure-build: ## Build the autonomous Azure provider plugin binary
	go build -o dist/magelift-provider-azure ./providers/azure/cmd/magelift-provider-azure

core-leanness: ## Prove the CLI carries no first-party cloud provider implementation
	@if ! deps="$$(go list -deps ./cmd/magelift)"; then \
		echo 'could not list CLI dependencies for the provider boundary check' >&2; exit 1; \
	fi; \
	forbidden="$$(printf '%s\n' "$$deps" | grep -E 'magelift/providers/|magelift/internal/cloud/(aws|gcp|azure)(/|$$)|cloud\.google\.com/go|google\.golang\.org/api/|github\.com/Azure/azure-sdk-for-go|github\.com/AzureAD/microsoft-authentication-library-for-go' || true)"; \
	if [ -n "$$forbidden" ]; then \
		printf 'CLI dependency boundary violation:\n%s\n' "$$forbidden" >&2; exit 1; \
	fi
lint: ## Run static Go checks across the CLI, SDK and provider modules
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./... ./sdk/... ./providers/aws/... ./providers/gcp/... ./providers/azure/...

license-check: ## Check Go dependency licenses
	go run github.com/google/go-licenses/v2@v2.0.1 check ./... --disallowed_types=forbidden,unknown

check-clean-room: ## Fail if vendored ece-tools / cloud-patches / ACC cli trees appear
	./scripts/check-clean-room.sh

release-smoke: ## Serial single-target goreleaser smoke (safe on low-RAM Macs)
	./scripts/release-smoke-local.sh

ci-act-project: ## Exercise consumer PHP config, workflow generation and a local OCI registry
	./scripts/ci-act-project.sh

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
	test "$$(docker run --rm --entrypoint nginx magelift/php-nginx:local -v 2>&1)" = "nginx version: nginx/1.30.7"
	docker run --rm --entrypoint nginx magelift/php-nginx:local -t -c /etc/nginx/nginx.conf
	docker run --rm --entrypoint nginx magelift/php-nginx:local -T -c /etc/nginx/nginx.conf | grep -q "location /media/"
	for prefix in customer downloadable import custom_options; do docker run --rm --entrypoint nginx magelift/php-nginx:local -T -c /etc/nginx/nginx.conf | grep -q "location ^~ /media/$$prefix/" || exit 1; done
	docker run --rm --entrypoint nginx magelift/php-nginx:local -T -c /etc/nginx/nginx.conf | grep -q "theme_customization"
	docker run --rm --entrypoint nginx magelift/php-nginx:local -T -c /etc/nginx/nginx.conf | grep -q "index|get|static"
	./scripts/nginx-media-deny-test.sh
	docker run --rm --read-only --tmpfs /tmp:uid=10001,gid=10001 magelift/php-nginx:local php -r 'file_put_contents(sys_get_temp_dir()."/probe", "ok");'
	docker run --rm magelift/php-nginx:local php -r '$$required = ["apcu", "bcmath", "ftp", "gd", "intl", "mbstring", "pdo_mysql", "redis", "soap", "sockets", "sodium", "xsl", "zip", "Zend OPcache"]; $$missing = array_values(array_filter($$required, fn(string $$extension): bool => !extension_loaded($$extension))); if ($$missing !== []) { fwrite(STDERR, "Missing PHP extensions: " . implode(", ", $$missing) . PHP_EOL); exit(1); }'
	./scripts/image-health-test.sh

builder-image-test: ## Build and inspect the isolated PHP build runner image
	docker buildx bake php-builder --load
	test "$$(docker run --rm --entrypoint id magelift/php-builder:local -u)" = 10001
	docker run --rm --entrypoint composer magelift/php-builder:local --version

varnish-test: ## Verify the integrated Varnish sidecar security contract
	./scripts/varnish-test.sh

build-e2e-test: builder-image-test ## Build the fixture through the CLI, Docker, and BuildKit
	./scripts/build-e2e.sh

pulumi-mock-test: ## Run Pulumi mock graph tests for cloud adapters (no account)
	go test ./providers/aws/stack ./providers/gcp/stack ./providers/azure/stack -count=1

floci-aws-test: ## Run account-free AWS contract tests through Floci
	./providers/aws/scripts/floci-aws-test.sh

floci-gcp-test: ## Run account-free GCP contract tests through Floci
	./providers/gcp/scripts/floci-gcp-test.sh

magento-dev-smoke: ## Install a real Magento fixture locally and verify HTTP, GraphQL and repeat startup
	./scripts/magento-dev-smoke.sh

floci-azure-test: ## Run account-free Azure Blob and Key Vault tests through Floci
	./providers/azure/scripts/floci-azure-test.sh

azure-mysql-tls-test: ## Verify Magento and bootstrap PDO against a local TLS-only MySQL server
	./providers/azure/scripts/mysql-tls-test.sh

local-gates: test-fast floci-aws-test floci-gcp-test floci-azure-test ## Run module tests, Pulumi mocks, and Floci checks

skills-test: ## Test bundled skill installation and verification
	go test ./internal/skills

installer-harness-test: ## Run the installer trust harness against local fixtures
	bash scripts/install-verify-harness.sh

extension-test: ## Build the public extension SDK contract with empty caches
	./scripts/custom-extension-clean-room.sh

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
		tests/fixtures/ci/gcp/.github/workflows/*.yml \
		tests/fixtures/ci/azure/.github/workflows/*.yml \
		tests/act/*.yml

verify: generate-check cli-docs-check fmt-check lint test-fast test-race-full sdk-test provider-tidy-check license-check check-clean-room php-test docs workflow-check ## Run the local verification suite
generate: cli-docs ## Generate configuration, CLI reference, and skill artifacts
	go generate ./internal/config ./agents

generate-check: ## Check the generated configuration files
	go run ./cmd/genconfig --check

cli-docs: ## Generate the CLI reference from the Cobra command tree
	go run ./cmd/gendocs

cli-docs-check: ## Check the generated CLI reference for drift
	go run ./cmd/gendocs --check

# Every named target is an action. Derived so a new target cannot miss .PHONY.
.PHONY: $(shell awk -F: '/^[a-zA-Z0-9_-]+:/ {print $$1}' $(firstword $(MAKEFILE_LIST)))
