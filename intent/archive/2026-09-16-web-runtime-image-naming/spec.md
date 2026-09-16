---
status: done
slug: web-runtime-image-naming
intent: intent.md
---

# Spec: php-nginx image naming

## Requirements

### Requirement: bake targets and tags renamed

The rename SHALL move every nginx bake target, local tag, and bake file path from `php-runtime` to `php-nginx` while leaving builder names, adapter targets, digests, and platforms untouched.

#### Scenario: bake file contains only new nginx names

- **WHEN** `grep -n 'php-runtime\|php-nginx\|php-builder' docker-bake.hcl` is run
- **THEN** it sees exactly `targets = ["php-nginx"]`, `target "php-nginx" {`, `dockerfile = "images/php-nginx/Dockerfile"`, `tags = ["magelift/php-nginx:local"]`, `target "php-nginx-supported" {`, `name = "php-nginx-${replace(php.branch, ".", "-")}"`, and `tags = ["magelift/php-nginx:${php.branch}-local"]`
- **AND** it sees `target "php-builder" {`, `target "php-builder-supported" {`, `name = "php-builder-${replace(php.branch, ".", "-")}"`, and `tags = ["magelift/php-builder:local"]` / `tags = ["magelift/php-builder:${php.branch}-local"]` byte-identical to today
- **AND** it sees zero lines matching `php-runtime`

#### Scenario: builder inherits the renamed nginx base

- **WHEN** `grep -n 'inherits' docker-bake.hcl` is run
- **THEN** the `php-builder` block and the `php-builder-supported` block both read `inherits = ["php-nginx"]`
- **AND** `inherits = ["php-apache"]` and `inherits = ["frankenphp-classic"]` are unchanged

#### Scenario: directory move matches bake paths

- **WHEN** `ls images/php-nginx/Dockerfile images/php-nginx/nginx.conf images/php-nginx/php.ini images/php-nginx/www.conf images/php-nginx/env.php images/php-nginx/deployment-config.php` is run
- **THEN** all six paths exist and `test ! -e images/php-runtime` passes

### Requirement: GHCR publish matrix renamed

The `images.yml` publish matrix SHALL publish the nginx image as `ghcr.io/magelift/magelift-nginx` while `magelift-builder` keeps its repo and all base digests stay pinned.

#### Scenario: matrix values produce the new repo name

- **WHEN** `grep -n 'image: \|file: \|target: \|IMAGE: ' .github/workflows/images.yml` is run
- **THEN** the four nginx rows read `- image: nginx` with `file: images/php-nginx/Dockerfile` and `target: runtime`, the four builder rows read `- image: builder` with `file: images/php-nginx/Dockerfile` and `target: builder`, and `IMAGE: ghcr.io/magelift/magelift-${{ matrix.image }}` composes `ghcr.io/magelift/magelift-nginx` for the nginx rows
- **AND** the four `frankenphp-classic` rows are unchanged and zero lines match `php-runtime` or `image: runtime`

#### Scenario: publish wiring and pins untouched

- **WHEN** `grep -n 'NGINX_BASE=\|if: .*contains(github.ref_name' .github/workflows/images.yml` is run
- **THEN** it sees `NGINX_BASE=docker.io/library/nginx:1.30.4-trixie@sha256:d5792f71a9496b833bc08ea834a758c46e2b6a6306c10f4be926f38a656cdc1c` and `if: ${{ !contains(github.ref_name, '-') }}` unchanged

### Requirement: Go default tags and references renamed

The Go toolchain defaults, local-dev image references, web-runtime descriptors, and their tests SHALL use `magelift/php-nginx` locally and `ghcr.io/magelift/magelift-nginx` in release fixtures, with builder strings unchanged.

#### Scenario: toolchain defaults and exact argv

- **WHEN** `grep -rn 'magelift/php-nginx:local\|magelift/php-builder:local' internal/toolchain/ && GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/toolchain/ -count=1` is run
- **THEN** `internal/toolchain/toolchain.go` contains `DefaultRuntimeTag = "magelift/php-nginx:local"` and `DefaultBuilderTag = "magelift/php-builder:local"`, `internal/toolchain/toolchain_test.go` contains `runtimeReference := "magelift/php-nginx@" + idB`, and the package test passes

#### Scenario: localdev and webruntime references

- **WHEN** `grep -rn 'php-nginx\|php-runtime' internal/localdev/ internal/webruntime/ sdk/v1/webruntime_test.go` is run
- **THEN** it sees `"magelift/php-nginx:"+phpBranch(php)+"-local"` in `internal/localdev/catalog.go`, `"magelift/php-nginx:8.5-local"` in `internal/localdev/catalog.go` (`ComposeTemplateFor`), `internal/localdev/compose.go` (`${MAGELIFT_LOCAL_APP_IMAGE:-magelift/php-nginx:8.5-local}`), and `internal/localdev/catalog_test.go` (two sites), plus `ImageFamily: "php-nginx"` in `internal/webruntime/plugins.go`, `internal/webruntime/registry_test.go`, and `sdk/v1/webruntime_test.go`, with zero `php-runtime` matches in those paths

#### Scenario: build pipeline and CLI fixtures

- **WHEN** `grep -rn 'magelift-nginx@\|magelift-runtime@\|magelift/php-nginx:local\|magelift/php-runtime:local' internal/build/pipeline/pipeline_test.go internal/cli/build_test.go` is run
- **THEN** `internal/build/pipeline/pipeline_test.go` contains `request.RuntimeImage = "magelift/php-nginx:local"` and two `request.RuntimeImage = "ghcr.io/magelift/magelift-nginx@sha256:"` sites, `internal/cli/build_test.go` contains `RuntimeImage:   "ghcr.io/magelift/magelift-nginx@sha256:"`, all `magelift-builder` sites are unchanged, and the only remaining `magelift-runtime` matches are the excluded `/tmp/magelift-runtime-env.php` scaffold path and its assertion

### Requirement: image labels follow the decided rule

The nginx Dockerfile SHALL keep every existing `io.magelift.*` key stable and gain `io.magelift.web-runtime="php-nginx"` for adapter parity; sibling adapter labels SHALL NOT change.

#### Scenario: nginx labels keep keys, add web-runtime value

- **WHEN** `grep -n 'io.magelift' images/php-nginx/Dockerfile` is run
- **THEN** it sees `io.magelift.php.branch="${PHP_BRANCH}"`, `io.magelift.php.base="${PHP_BASE}"`, `io.magelift.nginx.version="1.30.4"`, `io.magelift.base.family="debian-trixie"` unchanged, plus the new `io.magelift.web-runtime="php-nginx"` line mirroring `io.magelift.web-runtime="php-apache"` and `io.magelift.web-runtime="frankenphp-classic"`

#### Scenario: CI asserts the new label and sibling labels are untouched

- **WHEN** `grep -n 'io.magelift' .github/workflows/ci.yml images/php-apache/Dockerfile images/frankenphp-classic/Dockerfile` is run
- **THEN** `.github/workflows/ci.yml` asserts `io.magelift.web-runtime` equals `php-nginx` alongside the existing `io.magelift.php.branch` and `io.magelift.nginx.version` assertions, and the apache/frankenphp `LABEL` blocks are byte-identical to today

### Requirement: docs, README, Makefile, scripts, and Dependabot renamed

Human docs, build scripts, CI jobs, and Dependabot SHALL name `php-nginx` / `magelift-nginx` and SHALL add Dependabot docker coverage for `php-apache`.

#### Scenario: README, docs, and acceptance prose renamed

- **WHEN** `grep -rn 'php-nginx\|magelift-nginx\|php-runtime\|magelift-runtime' images/README.md docs/builds.md docs/aws-acceptance.md` is run
- **THEN** `images/README.md` documents `` `php-nginx` `` as the nginx base, shows `docker buildx bake php-nginx --load` and `docker buildx bake php-nginx-supported php-builder-supported --push`, and lists `ghcr.io/magelift/magelift-nginx`; `docs/builds.md` shows `--runtime-image ghcr.io/magelift/magelift-nginx@sha256:RUNTIME_DIGEST` in both examples; `docs/aws-acceptance.md` requires a MageLift runtime image (`` `php-nginx` ``, nginx + PHP-FPM); and zero `php-runtime` / `magelift-runtime` matches remain in those three files

#### Scenario: Makefile, CI container job, and health script renamed

- **WHEN** `grep -n 'php-nginx\|php-runtime\|MAGELIFT_PHP_NGINX_IMAGE\|MAGELIFT_PHP_RUNTIME_IMAGE' Makefile .github/workflows/ci.yml scripts/image-health-test.sh` is run
- **THEN** `Makefile` `image-test` runs `docker buildx bake php-nginx --load` and six `magelift/php-nginx:local` probe lines, `.github/workflows/ci.yml` builds `php-nginx-${{ matrix.php.target }}`, inspects `magelift/php-nginx:${PHP_BRANCH}-local`, and scans `image-ref: magelift/php-nginx:${{ matrix.php.branch }}-local`, `scripts/image-health-test.sh` defaults to `image="${MAGELIFT_PHP_NGINX_IMAGE:-magelift/php-nginx:local}"`, and zero `php-runtime` / `MAGELIFT_PHP_RUNTIME_IMAGE` matches remain in those three files

#### Scenario: Dependabot covers nginx and apache docker directories

- **WHEN** `grep -n -A3 'package-ecosystem: docker' .github/dependabot.yml` is run
- **THEN** it sees `directory: /images/php-nginx`, a sibling `directory: /images/php-apache` entry with the same weekly schedule, `open-pull-requests-limit: 2`, and `labels: [dependencies, docker]` shape, the existing `directory: /images/frankenphp-classic` entry unchanged, and zero `directory: /images/php-runtime`

#### Scenario: shared-file COPY paths and acceptance paths follow the move

- **WHEN** `grep -rn 'images/php-nginx/\|images/php-runtime/' images/php-nginx/Dockerfile images/php-apache/Dockerfile images/frankenphp-classic/Dockerfile tests/acceptance/magento_env_contract_test.sh` is run
- **THEN** every `COPY ... images/php-nginx/php.ini`, `www.conf`, `deployment-config.php`, and `env.php` path (five in the nginx Dockerfile, four in the apache Dockerfile, three in the frankenphp Dockerfile) plus `env_file`/`fpm_pool_file` in `tests/acceptance/magento_env_contract_test.sh` point at `images/php-nginx/`, with zero `images/php-runtime/` matches

### Requirement: zero stale image names outside intent history

The tree SHALL contain zero stale `php-runtime` / `magelift-runtime` references outside `intent/` history except the explicitly excluded runtime-scaffold tmp filename.

#### Scenario: repo-wide stale-name sweep

- **WHEN** `grep -rn 'php-runtime\|magelift-runtime' . --exclude-dir=.git --exclude-dir=intent | grep -v 'magelift-runtime-env.php'` is run
- **THEN** the output is empty, and the only excluded matches are `/tmp/magelift-runtime-env.php` in `internal/build/pipeline/assets/application.Dockerfile` and its assertion string in `internal/build/pipeline/pipeline_test.go`

### Requirement: php-apache kept behavior-identical

The change SHALL keep `php-apache` as a maintained experimental adapter with no behavior change to its bake targets, Dockerfile, or README adapter prose.

#### Scenario: apache adapter diff is references-only

- **WHEN** `git diff -- docker-bake.hcl images/php-apache/Dockerfile images/README.md` is run
- **THEN** `target "php-apache"`, `target "php-apache-supported"`, `tags = ["magelift/php-apache:8.5-local"]`, and `tags = ["magelift/php-apache:${php.branch}-local"]` show no diff hunks, `images/php-apache/Dockerfile` differs only on its four shared-file `COPY images/php-nginx/...` source paths, and `images/README.md` differs on the apache adapter only where the prose names the renamed nginx sibling

### Requirement: Varnish base pin confirmed and recorded

The change SHALL confirm the Varnish sidecar pin is untouched and SHALL record its exact pinned reference in the repo policy note.

#### Scenario: pin unchanged in script and quoted in policy note

- **WHEN** `grep -F 'docker.io/library/varnish:8.0.2@sha256:4b595728592a5b9709c9aac15368ca492e9742fb269ed12466b434a62b2c1b63' scripts/varnish-test.sh images/README.md` is run
- **THEN** both files match: `scripts/varnish-test.sh` still defaults `MAGELIFT_VARNISH_IMAGE` to that exact digest-pinned reference and the `images/README.md` policy section quotes it verbatim

### Requirement: default-image policy recorded

The repo SHALL record the `:local`-vs-GHCR default-image policy in `images/README.md` without implementing GHCR pull.

#### Scenario: policy section states now-vs-later defaults

- **WHEN** `grep -n -A20 '^## Default image policy' images/README.md` is run
- **THEN** the section exists and states that local builds/tests resolve `:local` bake tags (e.g. `magelift/php-nginx:local`), that CLI defaults move to GHCR-versioned digests only after the first stable tag in a separate change, that this change implements no GHCR pull, and that the nginx GHCR repo is `ghcr.io/magelift/magelift-nginx`

### Requirement: pre-rename GHCR state verified

The implementation SHALL verify no `magelift-runtime` container packages exist before treating the GHCR repo rename as alias-free, and SHALL fall back to an alias plan if any do.

#### Scenario: packages check shows no runtime repo

- **WHEN** the implementer runs `gh api /orgs/magelift/packages?package_type=container --paginate -q '.[].name'` with a `read:packages`-scoped token (this planner's token has only `admin:public_key, gist, read:org, repo` and got HTTP 403 on both `/orgs/magelift/packages` and `/users/magelift/packages`), or inspects `https://github.com/orgs/magelift/packages` manually
- **THEN** no package named `magelift-runtime` (and none named `magelift-nginx`) is listed, corroborated by `gh release list --repo magelift/magelift` showing no stable non-hyphen tags and the unchanged `if: ${{ !contains(github.ref_name, '-') }}` gate in `.github/workflows/images.yml`

#### Scenario: contingent alias plan if a package exists

- **WHEN** the packages check finds a `magelift-runtime` package
- **THEN** the implementation does not delete or rewrite it; instead it publishes identical digests under `ghcr.io/magelift/magelift-nginx`, leaves `magelift-runtime` tagged deprecated with a dated removal note in `images/README.md`, and records the deviation before proceeding

### Requirement: suites green under serial-build policy

The change SHALL keep the image gates and the verification suite green without parallel heavy builds or tier recertification claims.

#### Scenario: image-class gates pass

- **WHEN** the implementer runs (serially, Docker available) `make image-test`, `make builder-image-test`, `make frankenphp-image-test`, `make varnish-test`, and `make build-e2e-test`
- **THEN** all five targets exit 0, exercising `magelift/php-nginx:local`, `magelift/php-builder:local`, `magelift/frankenphp-classic:8.5-local`, the pinned Varnish image, and the CLI build path

#### Scenario: Go and repo verification pass

- **WHEN** the implementer runs `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/toolchain/ ./internal/localdev/ ./internal/webruntime/ ./internal/cli/ ./internal/build/... ./sdk/v1/ -count=1` per package plus `make verify` as available (at minimum `make workflow-check` and `make docs` after the YAML/docs edits)
- **THEN** every invoked suite exits 0 with `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB` in force, and `docs/capability-matrix.md` shows no diff (rename recertifies nothing)

## Design

### Complete old-to-new name table

| Surface | Old | New |
| --- | --- | --- |
| Bake group default | `targets = ["php-runtime"]` | `targets = ["php-nginx"]` |
| Bake target (single) | `target "php-runtime"` | `target "php-nginx"` |
| Bake dockerfile | `images/php-runtime/Dockerfile` | `images/php-nginx/Dockerfile` |
| Bake local tag | `magelift/php-runtime:local` | `magelift/php-nginx:local` |
| Bake matrix target | `target "php-runtime-supported"` | `target "php-nginx-supported"` |
| Bake matrix name | `php-runtime-${replace(php.branch, ".", "-")}` | `php-nginx-${replace(php.branch, ".", "-")}` |
| Bake matrix tag | `magelift/php-runtime:${php.branch}-local` | `magelift/php-nginx:${php.branch}-local` |
| Builder inherits (both `php-builder`, `php-builder-supported`) | `inherits = ["php-runtime"]` | `inherits = ["php-nginx"]` |
| Builder names/tags | `php-builder*`, `magelift/php-builder:*` | unchanged |
| images.yml matrix (nginx rows) | `- image: runtime` | `- image: nginx` |
| images.yml file (nginx + builder rows) | `file: images/php-runtime/Dockerfile` | `file: images/php-nginx/Dockerfile` |
| GHCR nginx repo (via `ghcr.io/magelift/magelift-${{ matrix.image }}`) | `ghcr.io/magelift/magelift-runtime` | `ghcr.io/magelift/magelift-nginx` |
| GHCR builder / frankenphp repos | `magelift-builder`, `magelift-frankenphp-classic` | unchanged |
| Go const | `DefaultRuntimeTag = "magelift/php-runtime:local"` | `DefaultRuntimeTag = "magelift/php-nginx:local"` |
| Go const | `DefaultBuilderTag = "magelift/php-builder:local"` | unchanged |
| Local-dev image refs | `magelift/php-runtime:8.5-local`, `"magelift/php-runtime:"+phpBranch(php)+"-local"` | `magelift/php-nginx:8.5-local`, `"magelift/php-nginx:"+phpBranch(php)+"-local"` |
| Web-runtime ImageFamily | `"php-runtime"` | `"php-nginx"` |
| Release fixtures | `ghcr.io/magelift/magelift-runtime@sha256:` | `ghcr.io/magelift/magelift-nginx@sha256:` |
| New label (nginx Dockerfile only) | (absent) | `io.magelift.web-runtime="php-nginx"` |
| Kept labels | `io.magelift.php.branch`, `io.magelift.php.base`, `io.magelift.nginx.version="1.30.4"`, `io.magelift.base.family="debian-trixie"` | unchanged |
| Health-script env + default | `MAGELIFT_PHP_RUNTIME_IMAGE:-magelift/php-runtime:local` | `MAGELIFT_PHP_NGINX_IMAGE:-magelift/php-nginx:local` |
| Dependabot docker dirs | `/images/php-runtime` | `/images/php-nginx`, plus new `/images/php-apache` |
| Acceptance script paths | `images/php-runtime/env.php`, `images/php-runtime/www.conf` | `images/php-nginx/env.php`, `images/php-nginx/www.conf` |
| Deliberately kept | `/tmp/magelift-runtime-env.php` (scaffold tmp file, not an image name) | unchanged |

### Builder-inheritance resolution

`php-builder` and `php-builder-supported` both inherit the nginx target directly today. The builder keeps its name, tags, matrix names, and GHCR repo (`magelift-builder` is the runtime-agnostic build runner); only the two `inherits` lines become `["php-nginx"]`. The images.yml builder rows likewise keep `image: builder` / `target: builder` and only move `file:` to `images/php-nginx/Dockerfile`.

### Default-policy note location

New section `## Default image policy` in `images/README.md`, placed right after the GHCR publication paragraph. It states the `:local`-now / GHCR-after-first-stable rule, names `ghcr.io/magelift/magelift-nginx`, quotes the Varnish pin verbatim, and declares no GHCR pull is implemented here. Human pages (`images/README.md`, `docs/builds.md`, `docs/aws-acceptance.md`) go through humanizer, then remove-ai-marks.

### Varnish pin statement

Confirmed pin (unchanged, digest-pinned, from `scripts/varnish-test.sh:7`): `docker.io/library/varnish:8.0.2@sha256:4b595728592a5b9709c9aac15368ca492e9742fb269ed12466b434a62b2c1b63`. `images/README.md` already describes the "pinned Varnish 8.0.2 sidecar"; this change only adds the exact digest reference to the policy note. No digest bumps anywhere (PHP/NGINX/Composer/Varnish pins byte-identical).

### Ordering

1. `git mv images/php-runtime images/php-nginx`; update `docker-bake.hcl` (targets, tags, inherits, paths).
2. Update `images.yml` matrix + file paths; update `ci.yml` container job; update `Makefile` `image-test`; update `scripts/image-health-test.sh` (tag + env var).
3. Update Go sources + tests (`toolchain`, `localdev`, `webruntime`, `cli`, `build/pipeline`, `sdk/v1`); update `magento_env_contract_test.sh` paths.
4. Update Dockerfiles (self COPY paths, apache/frankenphp shared-file COPY paths, new nginx `web-runtime` label).
5. Update `images/README.md` (rename + new `## Default image policy` section), `docs/builds.md`, `docs/aws-acceptance.md`; update `.github/dependabot.yml` (rename entry + add `php-apache`).
6. Run the GHCR existence check first (gates the alias-free claim), then per-package Go tests, image gates, `make workflow-check`, `make docs`, and `make verify` as available.

## Gotchas / policy flags

- GHCR repo rename is the one irreversible-feeling step: it is only safe because nothing has published yet (`images.yml` skips prerelease tags via `if: ${{ !contains(github.ref_name, '-') }}`). Gate everything on the packages existence check; this planner could not complete it (HTTP 403, token lacks `read:packages`), so the implementer runs it first and follows the contingent alias plan if a package exists.
- `:local` semantics unchanged: the CLI and local dev keep resolving locally baked tags; no GHCR pull, no versioned defaults in this change.
- Tier recertification explicitly not happening: `docs/capability-matrix.md` and `docs/evidence/README.md` stay untouched; `nginx-fpm` remains the certified runtime and `php-apache` / `frankenphp-classic` remain experimental.
- Humanizer + remove-ai-marks required on `images/README.md`, `docs/builds.md`, `docs/aws-acceptance.md` after editing.
- Tag-format consumers verified clean: `website/public/install.sh` is a binary installer with zero image references; `examples/`, `tests/fixtures/ci/`, `website/src`, and `schema/` have zero `php-runtime` / `magelift-runtime` matches per repo-wide grep; `docker-compose.floci.yml` and `docker-compose.floci-gcp.yml` name only `floci/*` emulator images.
- `images.yml` currently publishes only `runtime`/`builder`/`frankenphp-classic` (no `php-apache` rows) even though `images/README.md` calls the adapters "published" — that pre-existing gap stays out of scope; this change adds Dependabot coverage for apache, not publish rows.
- The new Dependabot `php-apache` entry mirrors the `php-nginx` entry shape (weekly, 7-day cooldown, limit 2, `docker-base-images` group); keep group naming consistent rather than inventing a third group style.
- Serial builds only: `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB`, per-package `go test`, no parallel heavy `go build`; load `magelift-serial-builds` for any release smoke.
- Diff hygiene: `git diff` must show name/path/label lines only — any digest change (PHP_BASE, NGINX_BASE, COMPOSER_BASE, Varnish) is a defect in this change.
- `MAGELIFT_PHP_NGINX_IMAGE` is a hard rename of a test-only override with no backward-compat shim; anyone exporting the old variable re-exports under the new name.

## Open questions carried forward

- GHCR repo `ghcr.io/magelift/magelift-nginx`? Decided: yes — mirrors `magelift-builder` and `magelift-frankenphp-classic`; composed automatically by the existing `ghcr.io/magelift/magelift-${{ matrix.image }}` expression with `- image: nginx`. Owner: implementer (no further decision).
- Builder keeps its name while its base becomes `php-nginx`? Decided: yes — `php-builder*`, `magelift/php-builder:*`, and `magelift-builder` are unchanged; only the two `inherits = ["php-nginx"]` lines and the images.yml builder `file:` paths move. Owner: implementer (no further decision).
- Label keys vs values? Proposed: keep all existing `io.magelift.*` keys stable (no existing value contains `php-runtime`, so no value renames) and add `io.magelift.web-runtime="php-nginx"` to the nginx Dockerfile for parity with `php-apache` / `frankenphp-classic`, with a matching CI assertion. Default: accept the proposal. Owner: reviewer to confirm at PR time.
- `MAGELIFT_PHP_RUNTIME_IMAGE` env var renamed to `MAGELIFT_PHP_NGINX_IMAGE` (hard rename, test-only script)? Default: yes as specced. Owner: implementer; reviewer may request a one-release fallback shim instead.
- `/tmp/magelift-runtime-env.php` scaffold path kept (generic "runtime" wording, not an image name)? Default: yes, keep, with the explicit stale-sweep exclusion. Owner: reviewer to confirm at PR time.
- GHCR existence unverified from this session (403 without `read:packages`)? Default: implementer runs the packages/UI check plus the `gh release list` and workflow-gate corroboration before landing; alias plan triggers on any hit. Owner: implementer (maintainer assists with Packages UI if token scope cannot be granted).
- Policy note lands as `## Default image policy` in `images/README.md`? Default: yes as specced. Owner: reviewer to confirm placement and prose during the humanizer pass.
