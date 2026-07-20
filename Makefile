.DEFAULT_GOAL := help

.PHONY: help generate generate-check cli-docs cli-docs-check fmt fmt-check test lint license-check php-test image-test frankenphp-image-test builder-image-test varnish-test build-e2e-test floci-test aws-acceptance-local gcp-acceptance-local docs workflow-check verify

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

fmt: ## Format Go sources
	gofmt -w $$(find . -type f -name '*.go' -not -path './vendor/*')

fmt-check: ## Check Go formatting without changing files
	test -z "$$(gofmt -l $$(find . -type f -name '*.go' -not -path './vendor/*'))"

test: ## Run Go tests with the race detector
	go test -race ./...

lint: ## Run static Go checks
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...

license-check: ## Check Go dependency licenses
	go run github.com/google/go-licenses/v2@v2.0.1 check ./... --disallowed_types=forbidden,unknown

php-test: ## Validate and test the Composer package
	composer validate --working-dir=build --strict
	composer audit --working-dir=build --locked
	composer analyse --working-dir=build
	composer psalm --working-dir=build
	php build/vendor/bin/phpunit -c build/phpunit.xml --fail-on-deprecation --fail-on-notice --fail-on-warning

image-test: ## Build and inspect the local PHP runtime image
	docker buildx bake php-runtime --load
	test "$$(docker run --rm magelift/php-runtime:local id -u)" = 10001
	docker run --rm magelift/php-runtime:local php-fpm --test
	test "$$(docker run --rm --entrypoint nginx magelift/php-runtime:local -v 2>&1)" = "nginx version: nginx/1.30.4"
	docker run --rm --entrypoint nginx magelift/php-runtime:local -t -c /etc/nginx/nginx.conf
	docker run --rm --read-only --tmpfs /tmp:uid=10001,gid=10001 magelift/php-runtime:local php -r 'file_put_contents(sys_get_temp_dir()."/probe", "ok");'
	docker run --rm magelift/php-runtime:local php -r '$$required = ["bcmath", "gd", "intl", "pdo_mysql", "soap", "sockets", "xsl", "zip", "Zend OPcache"]; $$missing = array_values(array_filter($$required, fn(string $$extension): bool => !extension_loaded($$extension))); if ($$missing !== []) { fwrite(STDERR, "Missing PHP extensions: " . implode(", ", $$missing) . PHP_EOL); exit(1); }'

frankenphp-image-test: ## Build and inspect the FrankenPHP classic adapter
	docker buildx bake frankenphp-classic --load
	test "$$(docker run --rm --entrypoint id magelift/frankenphp-classic:8.5-local -u)" = 10001
	docker run --rm --entrypoint frankenphp magelift/frankenphp-classic:8.5-local version
	docker run --rm --entrypoint frankenphp magelift/frankenphp-classic:8.5-local validate --config /etc/frankenphp/Caddyfile --adapter caddyfile
	./scripts/frankenphp-tls-test.sh

builder-image-test: ## Build and inspect the isolated PHP build runner image
	docker buildx bake php-builder --load
	test "$$(docker run --rm --entrypoint id magelift/php-builder:local -u)" = 10001
	docker run --rm --entrypoint composer magelift/php-builder:local --version

varnish-test: ## Verify the integrated Varnish sidecar security contract
	./scripts/varnish-test.sh

build-e2e-test: builder-image-test ## Build the fixture through the CLI, Docker, and BuildKit
	./scripts/build-e2e.sh

floci-test: ## Run account-free AWS state and lock integration tests through Floci
	./scripts/floci-test.sh

aws-acceptance-local: ## Run a local real-AWS acceptance pass (preview; destroys on exit)
	./scripts/aws-acceptance-local.sh

gcp-acceptance-local: ## Run a local real-GCP acceptance pass (experimental; destroys on exit)
	./scripts/gcp-acceptance-local.sh

docs: ## Build documentation with strict link and navigation checks
	mkdocs build --strict

workflow-check: ## Validate GitHub Actions workflow syntax and expressions
	go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/*.yml

verify: generate-check cli-docs-check fmt-check lint test license-check php-test docs workflow-check ## Run the local verification suite
generate: ## Generate configuration schema and reference
	go generate ./internal/config

generate-check: ## Check generated configuration files for drift
	go run ./cmd/genconfig --check

cli-docs: ## Generate the CLI reference from the Cobra command tree
	go run ./cmd/gendocs

cli-docs-check: ## Check the generated CLI reference for drift
	go run ./cmd/gendocs --check

