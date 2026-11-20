#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $# -ne 1 || ! "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
	printf 'usage: release-installer-smoke.sh vX.Y.Z[-prerelease]\n' >&2
	exit 2
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAG="$1"
VERSION="${TAG#v}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/magelift-release-install.XXXXXX")"
SERVER_PID=""
cleanup() {
	if [[ -n "$SERVER_PID" ]]; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	rm -rf "$WORK"
}
trap cleanup EXIT

command -v gh >/dev/null || { printf 'gh is required to download draft release assets\n' >&2; exit 2; }
command -v openssl >/dev/null || { printf 'openssl is required for the local provider download fixture\n' >&2; exit 2; }

ASSETS="$WORK/assets"
mkdir -p "$ASSETS"
gh release download "$TAG" --dir "$ASSETS" \
	--pattern "magelift_${VERSION}_linux_amd64.tar.gz" \
	--pattern checksums.txt \
	--pattern checksums.txt.sigstore.json \
	--pattern magelift.providers.lock.linux_amd64

ARCHIVE="magelift_${VERSION}_linux_amd64.tar.gz"
LOCK="$ASSETS/magelift.providers.lock.linux_amd64"
for required in "$ARCHIVE" checksums.txt checksums.txt.sigstore.json magelift.providers.lock.linux_amd64; do
	[[ -s "$ASSETS/$required" ]] || { printf 'draft release is missing %s\n' "$required" >&2; exit 1; }
done

while IFS= read -r name; do
	[[ -n "$name" ]] || continue
	[[ -s "$ASSETS/$name" ]] && continue
	gh release download "$TAG" --dir "$ASSETS" --pattern "$name"
done < <(python3 - "$LOCK" <<'PY'
import json
import sys
from urllib.parse import urlsplit

with open(sys.argv[1], encoding="utf-8") as stream:
    lock = json.load(stream)
for artifact in lock["providers"].values():
    for url in (artifact["url"], artifact["cosign"]["bundle"]):
        print(urlsplit(url).path.rsplit("/", 1)[-1])
PY
)

CERT="$WORK/localhost.crt"
KEY="$WORK/localhost.key"
CA_BUNDLE="$WORK/ca-bundle.pem"
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$KEY" -out "$CERT" \
	-subj '/CN=127.0.0.1' -addext 'subjectAltName=IP:127.0.0.1' -days 1 >/dev/null 2>&1
SYSTEM_CAS="$(python3 -c 'import ssl; print(ssl.get_default_verify_paths().cafile)')"
[[ -s "$SYSTEM_CAS" ]] || { printf 'system CA bundle is required\n' >&2; exit 1; }
cat "$SYSTEM_CAS" "$CERT" >"$CA_BUNDLE"
PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
python3 -u - "$ASSETS" "$PORT" "$CERT" "$KEY" >"$WORK/server.log" 2>&1 <<'PY' &
import functools
import http.server
import pathlib
import ssl
import sys

root = pathlib.Path(sys.argv[1]).resolve()
handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(root))
server = http.server.ThreadingHTTPServer(("127.0.0.1", int(sys.argv[2])), handler)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain(sys.argv[3], sys.argv[4])
server.socket = context.wrap_socket(server.socket, server_side=True)
server.serve_forever()
PY
SERVER_PID=$!
BASE="https://127.0.0.1:$PORT"
for _ in $(seq 1 50); do
	if curl --silent --fail --cacert "$CA_BUNDLE" "$BASE/checksums.txt" -o /dev/null; then break; fi
	sleep 0.1
done
curl --fail --silent --show-error --cacert "$CA_BUNDLE" "$BASE/checksums.txt" -o /dev/null

CLI_DIR="$WORK/bin"
mkdir -p "$CLI_DIR"
CURL_CA_BUNDLE="$CA_BUNDLE" \
	MAGELIFT_VERSION="$TAG" \
	MAGELIFT_RELEASE_BASE="$BASE" \
	MAGELIFT_COSIGN_BASE="https://github.com/sigstore/cosign/releases/download" \
	MAGELIFT_INSTALL_DIR="$CLI_DIR" \
	sh "$ROOT/website/public/install.sh"
installed_version="$("$CLI_DIR/magelift" --output json version | jq -er '.version')"
if [[ "$installed_version" != "$VERSION" && "$installed_version" != "$TAG" ]]; then
	printf 'installed version %s differs from release %s\n' "$installed_version" "$TAG" >&2
	exit 1
fi

LOCAL_LOCK="$WORK/magelift.providers.lock"
python3 - "$LOCK" "$LOCAL_LOCK" "$BASE" <<'PY'
import json
import sys
from urllib.parse import urlsplit

source, destination, base = sys.argv[1:]
with open(source, encoding="utf-8") as stream:
    lock = json.load(stream)
for artifact in lock["providers"].values():
    artifact["url"] = base + "/" + urlsplit(artifact["url"]).path.rsplit("/", 1)[-1]
    artifact["cosign"]["bundle"] = base + "/" + urlsplit(artifact["cosign"]["bundle"]).path.rsplit("/", 1)[-1]
with open(destination, "w", encoding="utf-8") as stream:
    json.dump(lock, stream, indent=2)
    stream.write("\n")
PY

CACHE="$WORK/cache"
for provider in aws gcp azure; do
	shop="$WORK/shop-$provider"
	mkdir -p "$shop"
	cp "$ROOT/tests/fixtures/ci/$provider/magelift.php" "$shop/magelift.php"
	(cd "$shop" && "$CLI_DIR/magelift" --no-interaction --env staging config validate)
	printf 'Installing the %s provider from signed draft artifacts\n' "$provider"
	(cd "$shop" && SSL_CERT_FILE="$CA_BUNDLE" "$CLI_DIR/magelift" \
		--config "$shop/magelift.php" --no-interaction providers install \
		--lockfile "$LOCAL_LOCK" --cache-dir "$CACHE")
done

printf 'release installer smoke passed for %s and AWS, GCP, and Azure providers\n' "$TAG"
