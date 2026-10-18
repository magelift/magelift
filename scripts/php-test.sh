#!/usr/bin/env bash
# Same locked Composer checks on host PHP or the existing pinned Composer image.
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD="$ROOT/build"
COMPOSER_IMAGE="${MAGELIFT_COMPOSER_TEST_IMAGE:-docker.io/library/composer:2.10.3@sha256:af98f42dfff7c68ba8d53c2164fd9fde1087b7d449514baa38c418b1f6bc4bac}"
mode="${1:-tests}"
case "$mode" in tests | --lifecycle-golden | --artifact-manifest) ;; *) printf 'php-test: unknown mode %s\n' "$mode" >&2; exit 2 ;; esac

phpunit_extensions_available() {
  php -r '$required = ["dom", "filter", "json", "libxml", "mbstring", "tokenizer", "xmlwriter"]; foreach ($required as $ext) { if (!extension_loaded($ext)) { exit(1); } }' 2>/dev/null
}

container=0
if [[ "${MAGELIFT_PHP_TEST_CONTAINER:-0}" == 1 ]] || ! phpunit_extensions_available || ! command -v composer >/dev/null 2>&1; then
  container=1
  command -v docker >/dev/null 2>&1 || { printf 'php-test: host prerequisites missing; install Docker for the pinned PHP runner\n' >&2; exit 2; }
fi
# Only source inputs needed by the tests are mounted. Caches are disposable and
# run as the caller, so a container cannot leave root-owned host cache files.
docker_args=(run --rm --user "$(id -u):$(id -g)"
  --env COMPOSER_CACHE_DIR=/tmp/composer-cache
  --tmpfs "/workspace/build/.phpstan:uid=$(id -u),gid=$(id -g)"
  --tmpfs "/workspace/build/.phpunit.cache:uid=$(id -u),gid=$(id -g)"
  -v "$BUILD:/workspace/build" -v "$ROOT/images:/workspace/images:ro" -w /workspace/build)

composer_cmd() {
  if ((container)); then
    docker "${docker_args[@]}" "$COMPOSER_IMAGE" composer "$@"
  else
    composer --working-dir="$BUILD" "$@"
  fi
}
php_cmd() {
  if ((container)); then
    docker "${docker_args[@]}" --entrypoint php "$COMPOSER_IMAGE" "$@"
  else
    (cd "$BUILD" && php "$@")
  fi
}

if [[ ! -f "$BUILD/vendor/autoload.php" ]]; then
  composer_cmd install --no-interaction --prefer-dist >&2
fi
if [[ "$mode" == --lifecycle-golden ]]; then
  php_cmd bin/magelift-lifecycle-export
  exit 0
fi
if [[ "$mode" == --artifact-manifest ]]; then
  php_cmd vendor/bin/phpunit -c phpunit.xml tests/Artifact/ArtifactManifestTest.php --fail-on-deprecation --fail-on-notice --fail-on-warning
  exit 0
fi
composer_cmd validate --strict
composer_cmd audit --locked
composer_cmd analyse
composer_cmd psalm
php_cmd vendor/bin/phpunit -c phpunit.xml --fail-on-deprecation --fail-on-notice --fail-on-warning
