#!/usr/bin/env bash
# Exercise a real, dependency-installed shop in an isolated local checkout.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source_dir=${MAGELIFT_MAGENTO_SOURCE:?set MAGELIFT_MAGENTO_SOURCE to a clean, dependency-installed Magento fixture}
[[ -f "$source_dir/vendor/autoload.php" && -f "$source_dir/composer.lock" && -f "$source_dir/bin/magento" ]] || { echo "Expected a real shop with locked, installed dependencies" >&2; exit 2; }
[[ ! -e "$source_dir/app/etc/env.php" ]] || { echo "Use a clean Magento fixture without an existing installation" >&2; exit 2; }
work=$(mktemp -d "${MAGELIFT_SMOKE_TMPDIR:-${TMPDIR:-/tmp}}/magelift-smoke-XXXXXXXX")
# A separate root isolates bind mounts, files, volumes and Compose's project name.
shop="$work/$(basename "$work" | tr '[:upper:]' '[:lower:]')"
mkdir "$shop"
ports_file="$work/ports"
bin=${MAGELIFT_BIN:-$work/magelift}
smoke_image="magelift-smoke-$(basename "$shop"):local"
cleanup() {
  local status=$?
  if [[ -f "$shop/.magelift/compose.local.yml" ]]; then
    if ! docker compose -f "$shop/.magelift/compose.local.yml" --project-name "magelift-$(basename "$shop")" --profile '*' down --volumes --remove-orphans >/dev/null; then
      echo "Failed to remove smoke containers and volumes for $(basename "$shop")" >&2
      status=1
    fi
  fi
  if docker image inspect "$smoke_image" >/dev/null 2>&1; then
    if ! docker image rm "$smoke_image" >/dev/null; then
      echo "Failed to remove smoke image $smoke_image" >&2
      status=1
    fi
  fi
  if ! rm -rf "$work"; then
    echo "Failed to remove smoke directory $work" >&2
    status=1
  fi
  exit "$status"
}
trap cleanup EXIT
# Build the runtime being tested from this checkout, without replacing a
# developer's existing MageLift image. Containers and this tag are removed.
docker build --target runtime -f "$root/images/php-nginx/Dockerfile" -t "$smoke_image" "$root"
export MAGELIFT_LOCAL_APP_IMAGE="$smoke_image"
(cd "$source_dir" && tar --exclude=.git --exclude=.magelift --exclude=auth.json --exclude=magelift.php --exclude='*.environments.php' --exclude='var/cache' --exclude='generated/code' --exclude='generated/metadata' -cf - .) | (cd "$shop" && tar -xf -)
if [[ ! -x "$bin" ]]; then (cd "$root" && go build -o "$bin" ./cmd/magelift); fi
python3 - "$ports_file" <<'PY'
import socket,sys
keys='APP_HTTP HTTPS DATABASE CACHE SEARCH SESSION QUEUE QUEUE_MANAGEMENT VARNISH'.split()
sockets=[]
with open(sys.argv[1],'w') as f:
    for key in keys:
        sock=socket.socket()
        sock.bind(('127.0.0.1',0))
        sockets.append(sock)
        print(f'export MAGELIFT_LOCAL_{key}_PORT={sock.getsockname()[1]}',file=f)
PY
# shellcheck disable=SC1090
source "$ports_file"
export MAGELIFT_LOCAL_BASE_URL="http://127.0.0.1:$MAGELIFT_LOCAL_APP_HTTP_PORT/"
cd "$shop"
"$bin" init --provider aws --storefront-origin https://storefront.example.test
# A Composer metapackage fixture may carry its own name. Use a distinct Compose
# project even when two copies of that same fixture are tested concurrently.
mv magelift.php fixture.php
cat > magelift.php <<'PHP'
<?php
$fixture = (require __DIR__ . '/fixture.php')->jsonSerialize();
$values = (array) $fixture;
$values['project']['name'] = basename(__DIR__);
return new \MageLift\Project($values);
PHP
"$bin" --no-interaction dev
"$bin" --no-interaction dev
curl --fail --silent --show-error --location --max-time 45 "$MAGELIFT_LOCAL_BASE_URL" -o "$work/storefront.html"
rg -qi '<html|<!doctype html' "$work/storefront.html"
curl --fail --silent --show-error --get --data-urlencode 'query={storeConfig{code}}' "${MAGELIFT_LOCAL_BASE_URL}graphql" | python3 -c 'import json,sys; assert json.load(sys.stdin)["data"]["storeConfig"]["code"]'
python3 - "$MAGELIFT_LOCAL_BASE_URL" <<'PY'
import json, sys, urllib.error, urllib.parse, urllib.request

url = urllib.parse.urljoin(sys.argv[1], 'graphql')
allowed = 'https://storefront.example.test'
query = urllib.parse.urlencode({'query': '{storeConfig{code}}'})
preflight = urllib.request.Request(url, method='OPTIONS', headers={
    'Origin': allowed,
    'Access-Control-Request-Method': 'POST',
    'Access-Control-Request-Headers': 'authorization,content-type,store',
})
with urllib.request.urlopen(preflight, timeout=45) as response:
    assert response.status == 204, response.status
    assert response.headers.get('Access-Control-Allow-Origin') == allowed
    assert response.headers.get('Access-Control-Allow-Credentials') == 'true'
    assert 'authorization' in response.headers.get('Access-Control-Allow-Headers', '').lower()

for origin, expected in [(allowed, allowed), ('https://untrusted.example.test', None)]:
    request = urllib.request.Request(url + '?' + query, headers={'Origin': origin})
    with urllib.request.urlopen(request, timeout=45) as response:
        assert response.headers.get('Access-Control-Allow-Origin') == expected, response.headers
        if expected:
            assert response.headers.get('Access-Control-Allow-Credentials') == 'true'
        assert json.load(response)['data']['storeConfig']['code']

denied = urllib.request.Request(url, method='OPTIONS', headers={
    'Origin': 'https://untrusted.example.test',
    'Access-Control-Request-Method': 'POST',
})
try:
    urllib.request.urlopen(denied, timeout=45)
    raise AssertionError('untrusted preflight succeeded')
except urllib.error.HTTPError as response:
    assert response.code == 403, response.code
    assert response.headers.get('Access-Control-Allow-Origin') is None
PY
echo "Magento install, second startup, native storefront, GraphQL and headless CORS passed"
