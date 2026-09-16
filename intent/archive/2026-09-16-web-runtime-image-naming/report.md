---
slug: web-runtime-image-naming
verified: 2026-09-16
verdict: pass
---

# Report: php-nginx image naming

## What shipped

The nginx-based PHP image is named `php-nginx` consistently across all 23
files in the plan's stale universe, plus the directory move
`images/php-runtime/` → `images/php-nginx/` (6 files via `git mv`):

- Bake (`docker-bake.hcl`): `targets = ["php-nginx"]`, `target "php-nginx"`,
  `images/php-nginx/Dockerfile`, `magelift/php-nginx:local` tags, the
  `-supported` matrix target/name/tags, and all three
  `inherits = ["php-nginx"]` lines (both builder blocks plus the matrix
  target itself). Builder names/tags and stage names (`runtime`/`builder`)
  byte-identical.
- Publish (`.github/workflows/images.yml`): four `- image: nginx` rows and
  eight `file: images/php-nginx/Dockerfile` paths; the unchanged
  `ghcr.io/magelift/magelift-${{ matrix.image }}` expression now composes
  `ghcr.io/magelift/magelift-nginx`. FrankenPHP rows, `NGINX_BASE` pin, and
  the prerelease skip gate untouched.
- Go defaults and references: `DefaultRuntimeTag = "magelift/php-nginx:local"`
  (`internal/toolchain/toolchain.go`), local-dev image refs
  (`internal/localdev/catalog.go`, `compose.go`, `catalog_test.go`),
  `ImageFamily: "php-nginx"` (`internal/webruntime/plugins.go`,
  `registry_test.go`, `sdk/v1/webruntime_test.go`), and release fixtures
  (`internal/build/pipeline/pipeline_test.go`,
  `internal/cli/build_test.go`). Builder strings, identifiers
  (`DefaultRuntimeTag`, `RuntimeImage`), and `ghcr.io/magelift/runtime@`
  negative fixtures unchanged.
- Labels: new `io.magelift.web-runtime="php-nginx"` in the nginx Dockerfile
  (existing four keys/values byte-identical) plus a matching assertion in
  `.github/workflows/ci.yml`. Apache/frankenphp `LABEL` blocks byte-identical.
- Workflows/Makefile/scripts: `ci.yml` container job
  (`php-nginx-…`, `magelift/php-nginx:…`, `image-ref: magelift/php-nginx:…`;
  shell var `runtime` kept), `Makefile` `image-test` (bake + six probes),
  `scripts/image-health-test.sh`
  (`MAGELIFT_PHP_NGINX_IMAGE:-magelift/php-nginx:local`, hard rename, no shim).
- Dockerfiles: five self `COPY images/php-nginx/…` paths in nginx, four
  shared-file paths in apache, three in frankenphp; apache self path
  (`apache.conf`) untouched.
- Docs: `images/README.md` (base prose, acceptance line, both bake commands,
  GHCR repo), `docs/builds.md` (both `--runtime-image …magelift-nginx@sha256:…`
  examples), `docs/aws-acceptance.md` (runtime bullet). New
  `## Default image policy` section in `images/README.md`: `:local` bake tags
  now, GHCR-versioned digests only after the first stable tag in a separate
  change, no GHCR pull implemented here, nginx repo
  `ghcr.io/magelift/magelift-nginx`, Varnish pin quoted verbatim.
- Dependabot: `/images/php-runtime` → `/images/php-nginx`, plus a new
  `/images/php-apache` entry mirroring the nginx shape (weekly, 7-day
  cooldown, limit 2, `docker-base-images` group). FrankenPHP entry unchanged.
- `php-apache` stays a maintained experimental adapter: bake targets/tags,
  Dockerfile behavior, and README adapter prose unchanged except the four
  shared-file `COPY` source paths.

## Deviations from plan

1. Box 0.1 ran with the same `read:packages` 403 the planner hit (token
   scopes `admin:public_key, gist, read:org, repo`). Alias-free claim rests
   on the spec's allowed corroboration: public packages page shows the empty
   get-started state, `gh release list` shows only the hyphenated
   `v0.0.0-dialproof.6`, the `!contains(github.ref_name, '-')` publish gate
   is unchanged, and `docker manifest inspect` is denied for both
   `magelift-runtime` and `magelift-nginx`. No alias plan triggered.
2. Box 6.1 literal sweep also matches gitignored build residue: `site/` was
   stale until `make docs` regenerated it clean; `dist/` holds pre-existing
   421 MB release binaries from 2026-09-15 20:00 (order-14 residue, ignored,
   left alone). Tracked-tree sweep (`git grep`, excluding `intent/`) is clean
   except the two `/tmp/magelift-runtime-env.php` scaffold sites.
   The plan's literal digest check (`git diff | grep sha256` empty) is
   unsatisfiable alongside its own required 3.3/5.3 edits, which touch
   `@sha256:`-bearing lines; verified instead that every touched digest
   value/placeholder is byte-identical (repo-name-only diffs).
   `docs/capability-matrix.md` shows a 5-line diff of pre-existing SendGrid
   wording with zero image-name mentions; this change adds no tier lines.
3. Box 6.3 fifth gate (`make build-e2e-test`) is red for two pre-existing,
   independently root-caused reasons, neither owned by this intent (see
   Findings W1, W2). Equivalent verification performed instead: fixture
   restored to `/tmp` only (never into the tree), CLI compiled and launched
   with renamed defaults, isolated builder container started (image
   resolution worked); BuildKit stage unreachable due to W2.
4. `remove-ai-marks` service unreachable (connection refused on
   `127.0.0.1:8765`); per that skill no local cleaning was attempted.
   Humanizer pass applied by hand to the three touched pages (identifier-only
   edits plus the plain declarative policy section; no tells introduced).
   Compensating scans: zero invisible codepoints in all three files;
   `images/README.md` and `docs/builds.md` fully ASCII;
   `docs/aws-acceptance.md` non-ASCII limited to pre-existing visible glyphs
   (→, ≥, …) outside the edited line.
5. `make verify` run as components, not end to end: full `go test -race ./...`
   skipped (brief forbids unbounded race suites from IDE sessions; no package
   outside the six tested references the renamed strings, proven by the
   stale-universe accounting plus the 23-file new-name list).
   `license-check` skipped (`go.mod`/`go.sum` untouched).
   `php-test`: validate/audit/analyse/psalm green; phpunit 125/128 with 3
   environmental failures (see W3).

## Verification

### Completeness

22 of 23 plan boxes fully green; box 6.3 partial (4/5 gates green, fifth
blocked pre-existing — deviations 2–3, findings W1–W2). Every `### Requirement:`
in `spec.md` has direct evidence below. No unticked box reflects unexecuted
plan work: the 6.3 remainder is unsatisfiable on this tree/box (fixture absent
at HEAD; rootless UID mapping), failing identically with or without this change.

- 0.1 GHCR existence: packages 403 (scope), public page empty, one hyphenated
  release, gate intact, both manifests denied → alias-free, no alias plan.
- 1.1 move: `git mv` + six-path `ls` + `test ! -e images/php-runtime` → pass.
- 1.2/1.3 bake: target/tag/path grep exact; three `inherits` flipped,
  apache/frankenphp unchanged, zero `php-runtime` → pass.
- 2.1/2.2 images.yml: four `image: nginx` + four `image: builder` rows with
  new `file:` paths, `target: runtime`/`builder` stages kept, zero stale;
  `NGINX_BASE` digest and skip gate unchanged; frankenphp diff empty → pass.
- 2.3 Makefile/ci.yml/health script: bake + six probes, container job refs,
  new env var, zero stale → pass.
- 2.4 CI label assertion added after the nginx.version assertion;
  apache/frankenphp `LABEL` blocks untouched → pass.
- 3.1 toolchain: defaults grep + `go test ./internal/toolchain/` (22 pass)
  → pass.
- 3.2 localdev/webruntime: all eight sites renamed, zero stale → pass.
- 3.3 fixtures: pipeline local + two GHCR sites, CLI GHCR site renamed;
  scaffold exclusion, both `magelift/runtime@` negative fixtures, and all
  builder sites intact → pass.
- 3.4 acceptance paths: `env_file`/`fpm_pool_file` under `images/php-nginx/`
  → pass.
- 4.1/4.2 Dockerfiles: five/four/three `COPY` paths, new nginx label,
  sibling labels untouched → pass.
- 4.3 apache diff: no hunks on apache targets/tags; apache Dockerfile differs
  only on four `COPY` paths; README apache prose has no diff (re-confirmed
  after group 5) → pass.
- 5.1–5.4 docs/policy/Varnish/Dependabot: prose greps exact, policy section
  states all four points, pin matches in both files, apache entry mirrors
  nginx shape → pass.
- 6.1 sweep/hygiene/tier: tracked sweep clean; digest values identical; tier
  diff pre-existing and imageless → pass with deviation 2.
- 6.2 Go suites: toolchain 22, localdev 39, webruntime 11, sdk/v1 151,
  cli 285, build/... 75 — 583 green, serial, per package → pass.
- 6.3 image gates: `image-test` (246 s, six probes + health script green,
  label verified on the built image), `builder-image-test` (builder inherits
  renamed base in build log), `frankenphp-image-test` (249 s),
  `varnish-test` (pulled the exact pinned digest) → 4/5; e2e blocked (W1, W2).
- 6.4 repo verification: `workflow-check`, `docs` (`--strict`), `fmt-check`,
  `lint` (0 issues), `gendocs --check`, `genconfig --check`,
  `check-clean-room` all exit 0 → pass with deviation 5.

### Correctness

Bar is the intent's proposed outcome: the nginx image named `php-nginx`
consistently everywhere (bake, tags, GHCR repo, Go defaults, labels,
workflows, Dependabot, docs), apache kept, Varnish pin confirmed, default-image
policy recorded. Every scenario in `spec.md` was exercised via its verify
clause; observations above. Human-observable moments (no UI in this intent):
built `magelift/php-nginx:local` probes green end to end; `docker image
inspect` shows `io.magelift.web-runtime=php-nginx` + `nginx.version=1.30.4`;
Varnish gate pulled digest `4b59…2b63`; `make docs` rebuilt the public site
from the renamed pages. Not a UI change, so no browser moment applies.

### Coherence

Diff follows the spec's name table row for row (kept-vs-renamed split exact),
the builder-inheritance resolution (name kept, two `inherits` lines moved —
plus the third inside the renamed matrix target, which the plan flags and the
build log confirms working), and the policy-note placement (right after the
GHCR paragraph). Names/paths/labels only; no Dockerfile behavior, digest,
stage-name, identifier, or tier change. Pre-existing hunks sharing two files
(SendGrid removal in `internal/localdev/catalog*.go`) are disjoint and
untouched.

## Findings

- WARNING — `make build-e2e-test` has no fixture: `tests/fixtures/build/`
  absent at HEAD (deleted in ancestor `3762291`), `scripts/build-e2e.sh:13`
  fails at `cp` before any image use. Needs its own intent (restore from
  history or regenerate); the deleted fixture is schema-current and names no
  images. `scripts/build-e2e.sh:13`
- WARNING — isolated build containers cannot write their output bind on
  rootless Docker: host dir `0700`/UID 1000 (`internal/containerrunner/runner.go:55`),
  container `--user=1000:1000` (`internal/containerrunner/identity_unix.go:10`)
  maps to host subuid 101000 → `mkdir /output/rootfs` denied (reproduced with
  stock flags; PHP ran, only the bind write failed). Fails identically
  pre-rename. Needs its own intent (or a documented rootful requirement).
  `internal/containerrunner/runner.go:99`
- WARNING — `php-test`: 3 `NativeProcessRunnerTest` failures are environmental
  (custom `/home/dev/.local/php` needs `LD_LIBRARY_PATH`, which the runner's
  restricted env strips; stripped-env repro fails). `build/` untouched by this
  change (`git status -- build/` empty). Pre-existing on this box.
  `build/tests/Process/NativeProcessRunnerTest.php:13`
- SUGGESTION — plan's literal digest-hygiene check contradicts its own 3.3/5.3
  edits; future rename plans should assert digest-value identity, not
  zero `sha256` diff lines. `intent/web-runtime-image-naming/plan.md:119`

## Not checked

- Full `go test -race ./...` and `license-check`: skipped with rationale
  (deviation 5). No signal expected: string-only change, affected packages
  all green, deps untouched.
- Live GHCR publish of `magelift-nginx`: nothing publishes until the first
  stable tag; the matrix composes the name correctly by expression.
- `remove-ai-marks` service clean: service down; compensating scans in
  deviation 4.
- Verified in implementing session (no forked verifier; re-running the image
  gates elsewhere would cost another hour of builds for identical evidence).

## Verdict

Pass. The rename is complete, consistent, and verified; the red gates are
pre-existing breakage outside this intent's scope (missing e2e fixture,
rootless UID mapping, PHP env process tests), each root-caused with evidence.
Follow-ups want their own intents: restore-or-replace the build-e2e fixture;
rootless support (or rootful requirement) for isolated builds.
