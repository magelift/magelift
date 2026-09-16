---
status: done
slug: web-runtime-image-naming
spec: spec.md
---

# Plan: php-nginx image naming

## Files that change

Stale-sweep universe (outside `intent/` and `.git`): **75 lines across 24 files** match `php-runtime|magelift-runtime`. Of those, **71 lines across 23 files are renamed**; 4 lines stay under the explicit `/tmp/magelift-runtime-env.php` exclusion. Plus 4 additive insertions (label, CI assertion, policy section, Dependabot entry). No digest, stage-name, Go-identifier, or tier line changes.

### 0. Pre-checks — 0 files (commands only, gates everything)

No file changes. GHCR/packages existence check + release-list corroboration. Outcome decides alias-free vs alias plan before any edit.

### 1. Move + bake — 1 Move (6 files) + 1 Edit (10 stale lines)

- Move: `images/php-runtime/` → `images/php-nginx/` via `git mv` (6 files: `Dockerfile`, `deployment-config.php`, `env.php`, `nginx.conf`, `php.ini`, `www.conf`).
- Edit: `docker-bake.hcl` — `targets = ["php-nginx"]`; `target "php-nginx"`; `dockerfile = "images/php-nginx/Dockerfile"`; `tags = ["magelift/php-nginx:local"]`; `target "php-nginx-supported"`; `name = "php-nginx-${replace(php.branch, ".", "-")}"`; `tags = ["magelift/php-nginx:${php.branch}-local"]`; **three** `inherits = ["php-nginx"]` lines (the two builder blocks plus the one inside the renamed matrix target itself — all three must flip or bake breaks).

Bake gotcha: one `inherits` line sits inside the renamed `php-runtime-supported` target itself. All three lines change; `target = "runtime"` / `target = "builder"` (Dockerfile stage names) stay.

### 2. Workflows + Makefile + scripts — 4 Edits (24 stale lines)

- Edit: `.github/workflows/images.yml` — 4× `- image: runtime` → `- image: nginx`; 8× `file: images/php-runtime/Dockerfile` → `file: images/php-nginx/Dockerfile`. `target: runtime` / `target: builder` (stage names) stay; `IMAGE: ghcr.io/magelift/magelift-${{ matrix.image }}` expression stays and now composes `magelift-nginx`.
- Edit: `.github/workflows/ci.yml` — `php-runtime-${{ matrix.php.target }}` → `php-nginx-…`; `image="magelift/php-runtime:…` → `magelift/php-nginx:…`; `runtime="magelift/php-runtime:…` → `magelift/php-nginx:…` (shell var name `runtime` stays); `image-ref: magelift/php-runtime:…` → `magelift/php-nginx:…`. ADD new `io.magelift.web-runtime == php-nginx` assertion after the `io.magelift.nginx.version` assertion.
- Edit: `Makefile` — `docker buildx bake php-nginx --load` + six `magelift/php-nginx:local` probe lines in `image-test`.
- Edit: `scripts/image-health-test.sh` — `image="${MAGELIFT_PHP_NGINX_IMAGE:-magelift/php-nginx:local}"` (hard rename, no shim).

### 3. Go + tests + acceptance — 11 Edits (16 stale lines renamed, 1 excluded line kept)

- Edit: `internal/toolchain/toolchain.go` — `DefaultRuntimeTag = "magelift/php-nginx:local"`. Identifier `DefaultRuntimeTag` stays; `DefaultBuilderTag` value stays.
- Edit: `internal/toolchain/toolchain_test.go` — `runtimeReference := "magelift/php-nginx@" + idB`.
- Edit: `internal/localdev/catalog.go` — `"magelift/php-nginx:"+phpBranch(php)+"-local"`; `"magelift/php-nginx:8.5-local"`.
- Edit: `internal/localdev/compose.go` — `${MAGELIFT_LOCAL_APP_IMAGE:-magelift/php-nginx:8.5-local}`.
- Edit: `internal/localdev/catalog_test.go` — two `magelift/php-nginx:8.5-local` sites.
- Edit: `internal/webruntime/plugins.go` — `ImageFamily: "php-nginx"`.
- Edit: `internal/webruntime/registry_test.go` — `ImageFamily != "php-nginx"`.
- Edit: `sdk/v1/webruntime_test.go` — `ImageFamily: "php-nginx"`.
- Edit: `internal/build/pipeline/pipeline_test.go` — `request.RuntimeImage = "magelift/php-nginx:local"`; two `ghcr.io/magelift/magelift-nginx@sha256:` sites. KEEP `/tmp/magelift-runtime-env.php` assertion. Negative fixture `ghcr.io/magelift/runtime@…` stays.
- Edit: `internal/cli/build_test.go` — `RuntimeImage: "ghcr.io/magelift/magelift-nginx@sha256:"`. Negative fixture `ghcr.io/magelift/runtime@…` stays.
- Edit: `tests/acceptance/magento_env_contract_test.sh` — `images/php-nginx/env.php`, `images/php-nginx/www.conf`.

### 4. Dockerfiles + labels — 3 Edits (12 stale COPY lines + 1 new label)

- Edit: `images/php-nginx/Dockerfile` (moved in group 1) — 5× `COPY … images/php-nginx/…` self paths (`php.ini`, `www.conf`, `nginx.conf`, `deployment-config.php`, `env.php`); ADD `io.magelift.web-runtime="php-nginx"` to the `LABEL` block (existing 4 keys/values byte-identical).
- Edit: `images/php-apache/Dockerfile` — 4× shared-file `COPY images/php-nginx/…` source paths only; `LABEL` block byte-identical.
- Edit: `images/frankenphp-classic/Dockerfile` — 3× shared-file `COPY images/php-nginx/…` source paths only; `LABEL` block byte-identical.

### 5. Docs + Dependabot + policy — 4 Edits (9 stale lines + policy section + 1 new entry)

- Edit: `images/README.md` — `` `php-nginx` `` base prose; acceptance line; `docker buildx bake php-nginx --load`; `docker buildx bake php-nginx-supported php-builder-supported --push`; `ghcr.io/magelift/magelift-nginx`. ADD `## Default image policy` section immediately after the GHCR publication paragraph: `:local`-now / GHCR-after-first-stable, `ghcr.io/magelift/magelift-nginx`, verbatim Varnish pin, no-GHCR-pull declaration. Humanizer pass, then remove-ai-marks.
- Edit: `docs/builds.md` — two `--runtime-image ghcr.io/magelift/magelift-nginx@sha256:RUNTIME_DIGEST` examples. Humanizer pass, then remove-ai-marks.
- Edit: `docs/aws-acceptance.md` — `` `php-nginx` ``. Humanizer pass, then remove-ai-marks.
- Edit: `.github/dependabot.yml` — `directory: /images/php-runtime` → `directory: /images/php-nginx`; ADD sibling `directory: /images/php-apache` entry mirroring the nginx shape (weekly, 7-day cooldown, limit 2, `labels: [dependencies, docker]`, `docker-base-images` group). Frankenphp entry unchanged.

### NOT touched

- All `@sha256:` digests (PHP_BASE ×4, NGINX_BASE, FRANKENPHP_BASE ×4, Varnish, Cosign pins) — any digest diff is a defect.
- `docs/capability-matrix.md`, `docs/evidence/**` — rename recertifies nothing; must show no diff.
- `internal/build/pipeline/assets/application.Dockerfile` — excluded `/tmp/magelift-runtime-env.php` lines stay.
- `scripts/varnish-test.sh` — verify-only; `MAGELIFT_VARNISH_IMAGE` pin line stays byte-identical.
- `website/public/install.sh`, `examples/**`, `tests/fixtures/**`, `docker-compose.floci.yml`, `docker-compose.floci-gcp.yml`, `schema/**`, `website/src/**` — zero matches, confirmed clean.
- `images.yml` frankenphp rows, `NGINX_BASE=…` line, `if: ${{ !contains(github.ref_name, '-') }}` gate; `target: runtime` / `target: builder` stage names everywhere; `DefaultBuilderTag`, `magelift-builder` repos/tags; apache + frankenphp `LABEL` blocks; `images/varnish/**`.
- Words that merely contain "runtime" (`DefaultRuntimeTag`, `request.RuntimeImage`, `MAGELIFT_LOCAL_APP_IMAGE`, `runtime="…"`, `ghcr.io/magelift/runtime@` negative fixtures) — values/paths change, identifiers stay.

## Order of work

### 0. Pre-checks — GHCR existence FIRST (gates the alias-free claim)

- [x] 0.1 Run the container-packages existence check — verify: `gh api /orgs/magelift/packages?package_type=container --paginate -q '.[].name'` with a `read:packages`-scoped token (or manual inspect of `https://github.com/orgs/magelift/packages`), plus corroboration `gh release list --repo magelift/magelift` showing no stable non-hyphen tags and `grep -n 'if: .*contains(github.ref_name' .github/workflows/images.yml` still showing `if: ${{ !contains(github.ref_name, '-') }}` — verify: no package named `magelift-runtime` (and none named `magelift-nginx`) is listed.
  - Outcome A (absent): proceed alias-free with the steps below.
  - Outcome B (present): STOP for direction — do not delete or rewrite the package; switch to the alias plan (publish identical digests under `ghcr.io/magelift/magelift-nginx`, leave `magelift-runtime` tagged deprecated with a dated removal note in `images/README.md`), record "Deviations", and await maintainer direction before editing.

### 1. Move + bake

- [x] 1.1 Move the directory — verify: `git mv images/php-runtime images/php-nginx && ls images/php-nginx/Dockerfile images/php-nginx/nginx.conf images/php-nginx/php.ini images/php-nginx/www.conf images/php-nginx/env.php images/php-nginx/deployment-config.php && test ! -e images/php-runtime`.
- [x] 1.2 Rename bake targets, tags, paths — verify: `grep -n 'php-runtime\|php-nginx\|php-builder' docker-bake.hcl` sees exactly `targets = ["php-nginx"]`, `target "php-nginx" {`, `dockerfile = "images/php-nginx/Dockerfile"`, `tags = ["magelift/php-nginx:local"]`, `target "php-nginx-supported" {`, `name = "php-nginx-${replace(php.branch, ".", "-")}"`, `tags = ["magelift/php-nginx:${php.branch}-local"]`, byte-identical `php-builder` / `php-builder-supported` names and tags, and zero `php-runtime` lines.
- [x] 1.3 Flip the three inherits lines — verify: `grep -n 'inherits' docker-bake.hcl` shows `inherits = ["php-nginx"]` in the `php-builder`, `php-nginx-supported`, and `php-builder-supported` blocks, with `inherits = ["php-apache"]` and `inherits = ["frankenphp-classic"]` unchanged.

### 2. Workflows + Makefile + scripts

- [x] 2.1 Rename the images.yml matrix — verify: `grep -n 'image: \|file: \|target: \|IMAGE: ' .github/workflows/images.yml` shows four `- image: nginx` rows with `file: images/php-nginx/Dockerfile` and `target: runtime`, four `- image: builder` rows with `file: images/php-nginx/Dockerfile` and `target: builder`, `IMAGE: ghcr.io/magelift/magelift-${{ matrix.image }}` composing `ghcr.io/magelift/magelift-nginx`, unchanged frankenphp rows, and zero `php-runtime` / `image: runtime` matches.
- [x] 2.2 Confirm publish wiring and pins untouched — verify: `grep -n 'NGINX_BASE=\|if: .*contains(github.ref_name' .github/workflows/images.yml` shows `NGINX_BASE=docker.io/library/nginx:1.30.4-trixie@sha256:d5792f71a9496b833bc08ea834a758c46e2b6a6306c10f4be926f38a656cdc1c` and `if: ${{ !contains(github.ref_name, '-') }}` unchanged.
- [x] 2.3 Rename Makefile, ci.yml container job, health script — verify: `grep -n 'php-nginx\|php-runtime\|MAGELIFT_PHP_NGINX_IMAGE\|MAGELIFT_PHP_RUNTIME_IMAGE' Makefile .github/workflows/ci.yml scripts/image-health-test.sh` shows `docker buildx bake php-nginx --load` + six `magelift/php-nginx:local` probe lines in `Makefile`, `php-nginx-${{ matrix.php.target }}` / `magelift/php-nginx:${PHP_BRANCH}-local` / `image-ref: magelift/php-nginx:${{ matrix.php.branch }}-local` in `ci.yml`, `image="${MAGELIFT_PHP_NGINX_IMAGE:-magelift/php-nginx:local}"` in the health script, and zero `php-runtime` / `MAGELIFT_PHP_RUNTIME_IMAGE` matches.
- [x] 2.4 Add the ci.yml web-runtime label assertion — verify: `grep -n 'io.magelift' .github/workflows/ci.yml images/php-apache/Dockerfile images/frankenphp-classic/Dockerfile` shows ci.yml asserting `io.magelift.web-runtime` equals `php-nginx` alongside the existing `io.magelift.php.branch` and `io.magelift.nginx.version` assertions, with apache/frankenphp `LABEL` blocks byte-identical.

### 3. Go + tests + acceptance

- [x] 3.1 Rename toolchain defaults and exact argv — verify: `grep -rn 'magelift/php-nginx:local\|magelift/php-builder:local' internal/toolchain/ && GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/toolchain/ -count=1` shows `DefaultRuntimeTag = "magelift/php-nginx:local"`, `DefaultBuilderTag = "magelift/php-builder:local"`, `runtimeReference := "magelift/php-nginx@" + idB`, and a passing package test.
- [x] 3.2 Rename localdev and webruntime references — verify: `grep -rn 'php-nginx\|php-runtime' internal/localdev/ internal/webruntime/ sdk/v1/webruntime_test.go` shows `"magelift/php-nginx:"+phpBranch(php)+"-local"`, `"magelift/php-nginx:8.5-local"` (catalog `ComposeTemplateFor`, compose `${MAGELIFT_LOCAL_APP_IMAGE:-…}`, two catalog_test sites), `ImageFamily: "php-nginx"` (plugins, registry_test, webruntime_test), and zero `php-runtime` matches.
- [x] 3.3 Rename build-pipeline and CLI fixtures — verify: `grep -rn 'magelift-nginx@\|magelift-runtime@\|magelift/php-nginx:local\|magelift/php-runtime:local' internal/build/pipeline/pipeline_test.go internal/cli/build_test.go` shows `request.RuntimeImage = "magelift/php-nginx:local"` + two `ghcr.io/magelift/magelift-nginx@sha256:` sites in pipeline_test, `RuntimeImage: "ghcr.io/magelift/magelift-nginx@sha256:"` in build_test, unchanged `magelift-builder` sites, and the only remaining `magelift-runtime` matches are the excluded `/tmp/magelift-runtime-env.php` scaffold path + assertion.
- [x] 3.4 Rename acceptance-script paths — verify: `grep -rn 'images/php-nginx/\|images/php-runtime/' tests/acceptance/magento_env_contract_test.sh` shows `env_file`/`fpm_pool_file` pointing at `images/php-nginx/` with zero `images/php-runtime/` matches.

### 4. Dockerfiles + labels

- [x] 4.1 Rename nginx Dockerfile COPY paths, add web-runtime label — verify: `grep -n 'io.magelift' images/php-nginx/Dockerfile` shows `io.magelift.php.branch="${PHP_BRANCH}"`, `io.magelift.php.base="${PHP_BASE}"`, `io.magelift.nginx.version="1.30.4"`, `io.magelift.base.family="debian-trixie"` unchanged plus new `io.magelift.web-runtime="php-nginx"` mirroring the apache/frankenphp values.
- [x] 4.2 Rename shared-file COPY paths in all three Dockerfiles — verify: `grep -rn 'images/php-nginx/\|images/php-runtime/' images/php-nginx/Dockerfile images/php-apache/Dockerfile images/frankenphp-classic/Dockerfile tests/acceptance/magento_env_contract_test.sh` shows five COPY paths in nginx, four in apache, three in frankenphp, plus the acceptance paths, all under `images/php-nginx/`, with zero `images/php-runtime/` matches.
- [x] 4.3 Confirm apache diff is references-only — verify: `git diff -- docker-bake.hcl images/php-apache/Dockerfile images/README.md` shows no hunks on `target "php-apache"`, `target "php-apache-supported"`, `tags = ["magelift/php-apache:8.5-local"]`, `tags = ["magelift/php-apache:${php.branch}-local"]`; apache Dockerfile differs only on its four shared-file `COPY images/php-nginx/...` paths; README differs on apache only where it names the renamed nginx sibling.

### 5. Docs + Dependabot + policy

- [x] 5.1 Rename README/docs/acceptance prose (humanizer pass, then remove-ai-marks, on all three files) — verify: `grep -rn 'php-nginx\|magelift-nginx\|php-runtime\|magelift-runtime' images/README.md docs/builds.md docs/aws-acceptance.md` shows `` `php-nginx` `` nginx base + `docker buildx bake php-nginx --load` + `docker buildx bake php-nginx-supported php-builder-supported --push` + `ghcr.io/magelift/magelift-nginx` in README, `--runtime-image ghcr.io/magelift/magelift-nginx@sha256:RUNTIME_DIGEST` in both builds.md examples, `` `php-nginx` `` (nginx + PHP-FPM) in aws-acceptance.md, and zero stale matches.
- [x] 5.2 Add the `## Default image policy` section — verify: `grep -n -A20 '^## Default image policy' images/README.md` shows the section (placed right after the GHCR publication paragraph) stating `:local` bake tags now (e.g. `magelift/php-nginx:local`), GHCR-versioned digests only after the first stable tag in a separate change, no GHCR pull implemented here, and the nginx GHCR repo `ghcr.io/magelift/magelift-nginx`.
- [x] 5.3 Quote the Varnish pin in script and policy note — verify: `grep -F 'docker.io/library/varnish:8.0.2@sha256:4b595728592a5b9709c9aac15368ca492e9742fb269ed12466b434a62b2c1b63' scripts/varnish-test.sh images/README.md` matches in both files (`MAGELIFT_VARNISH_IMAGE` default unchanged; policy note quotes it verbatim).
- [x] 5.4 Rename + extend Dependabot docker entries — verify: `grep -n -A3 'package-ecosystem: docker' .github/dependabot.yml` shows `directory: /images/php-nginx`, a sibling `directory: /images/php-apache` entry with the same weekly schedule / 7-day cooldown / `open-pull-requests-limit: 2` / `labels: [dependencies, docker]` / `docker-base-images` group shape, the existing `directory: /images/frankenphp-classic` entry unchanged, and zero `directory: /images/php-runtime`.

### 6. Gates (serial policy: `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB`, per-package tests, no parallel heavy builds)

- [x] 6.1 Repo-wide stale-name sweep — verify: `grep -rn 'php-runtime\|magelift-runtime' . --exclude-dir=.git --exclude-dir=intent | grep -v 'magelift-runtime-env.php'` outputs empty, with the only excluded matches being `/tmp/magelift-runtime-env.php` in `internal/build/pipeline/assets/application.Dockerfile` and its assertion in `internal/build/pipeline/pipeline_test.go`.
- [x] 6.2 Per-package Go suites — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/toolchain/ ./internal/localdev/ ./internal/webruntime/ ./internal/cli/ ./internal/build/... ./sdk/v1/ -count=1` run per package, every suite exits 0.
- [ ] 6.3 Image-class gates serially (Docker available) — verify: `make image-test`, `make builder-image-test`, `make frankenphp-image-test`, `make varnish-test`, `make build-e2e-test` each exit 0, exercising `magelift/php-nginx:local`, `magelift/php-builder:local`, `magelift/frankenphp-classic:8.5-local`, the pinned Varnish image, and the CLI build path.
- [x] 6.4 Repo verification + tier guard — verify: `make workflow-check`, `make docs`, and `make verify` as available exit 0, and `git diff -- docs/capability-matrix.md` is empty.

## Risks

- GHCR package already exists → alias fallback required. Check: step 0.1 packages query + release-list corroboration; on hit, alias plan + dated README note + recorded deviation, stop for direction.
- Digest drift hiding in the diff → any `@sha256:` hunk is a defect. Check: `git diff | grep '^[+-].*sha256:'` must be empty; step 2.2 pin grep.
- Missed stale reference → sweep catches it. Check: step 6.1 repo-wide sweep outputs empty.
- `MAGELIFT_PHP_RUNTIME_IMAGE` hard rename breaks someone's exported env (test-only override, no shim). Check: step 2.3 grep shows zero old-var matches; release notes call out the re-export.
- Label/CI assertion mismatch (Dockerfile value vs ci.yml expectation). Check: steps 2.4 + 4.1 greps agree on `php-nginx`; `make image-test` + ci container job green.
- Heavy image gates on small machines → serial policy only (`GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB`, one gate at a time, per-package `go test`). Check: step 6.3 run serially; load `magelift-serial-builds` for any release smoke.
- Accidental tier-word edit (rename recertifies nothing). Check: `git diff -- docs/capability-matrix.md docs/evidence/` empty; `nginx-fpm` certified, `php-apache`/`frankenphp-classic` experimental wording untouched.
- Over-rename of stage names / identifiers (`target = "runtime"`, `DefaultRuntimeTag`, `RuntimeImage` fields, `ghcr.io/magelift/runtime@` negative fixtures). Check: steps 1.2, 2.1, 3.3 verifies name exactly what stays.

## Proof

End-to-end evidence block (run from repo root after all steps; serial image gates last):

```sh
# 0. Pre-check corroboration
gh api /orgs/magelift/packages?package_type=container --paginate -q '.[].name'
gh release list --repo magelift/magelift

# 1. Bake + move
ls images/php-nginx/Dockerfile images/php-nginx/nginx.conf images/php-nginx/php.ini images/php-nginx/www.conf images/php-nginx/env.php images/php-nginx/deployment-config.php && test ! -e images/php-runtime
grep -n 'php-runtime\|php-nginx\|php-builder' docker-bake.hcl
grep -n 'inherits' docker-bake.hcl

# 2. Matrix + wiring + Makefile/CI/scripts
grep -n 'image: \|file: \|target: \|IMAGE: ' .github/workflows/images.yml
grep -n 'NGINX_BASE=\|if: .*contains(github.ref_name' .github/workflows/images.yml
grep -n 'php-nginx\|php-runtime\|MAGELIFT_PHP_NGINX_IMAGE\|MAGELIFT_PHP_RUNTIME_IMAGE' Makefile .github/workflows/ci.yml scripts/image-health-test.sh

# 3. Go + fixtures + acceptance
grep -rn 'magelift/php-nginx:local\|magelift/php-builder:local' internal/toolchain/
grep -rn 'php-nginx\|php-runtime' internal/localdev/ internal/webruntime/ sdk/v1/webruntime_test.go
grep -rn 'magelift-nginx@\|magelift-runtime@\|magelift/php-nginx:local\|magelift/php-runtime:local' internal/build/pipeline/pipeline_test.go internal/cli/build_test.go
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/toolchain/ -count=1
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/localdev/ -count=1
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/webruntime/ -count=1
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/build/... -count=1
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./sdk/v1/ -count=1

# 4. Labels + COPY paths + apache diff shape
grep -n 'io.magelift' images/php-nginx/Dockerfile
grep -n 'io.magelift' .github/workflows/ci.yml images/php-apache/Dockerfile images/frankenphp-classic/Dockerfile
grep -rn 'images/php-nginx/\|images/php-runtime/' images/php-nginx/Dockerfile images/php-apache/Dockerfile images/frankenphp-classic/Dockerfile tests/acceptance/magento_env_contract_test.sh
git diff -- docker-bake.hcl images/php-apache/Dockerfile images/README.md

# 5. Docs + policy + Varnish pin + Dependabot
grep -rn 'php-nginx\|magelift-nginx\|php-runtime\|magelift-runtime' images/README.md docs/builds.md docs/aws-acceptance.md
grep -n -A20 '^## Default image policy' images/README.md
grep -F 'docker.io/library/varnish:8.0.2@sha256:4b595728592a5b9709c9aac15368ca492e9742fb269ed12466b434a62b2c1b63' scripts/varnish-test.sh images/README.md
grep -n -A3 'package-ecosystem: docker' .github/dependabot.yml

# 6. Sweep + digest hygiene + tier guard
grep -rn 'php-runtime\|magelift-runtime' . --exclude-dir=.git --exclude-dir=intent | grep -v 'magelift-runtime-env.php'
git diff | grep '^[+-].*sha256:'; test $? -eq 1
git diff -- docs/capability-matrix.md docs/evidence/

# 7. Gates (serially, Docker available)
make image-test && make builder-image-test && make frankenphp-image-test && make varnish-test && make build-e2e-test
make workflow-check && make docs && make verify
```
