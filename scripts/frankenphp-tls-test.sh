#!/usr/bin/env bash
set -Eeuo pipefail

image="${MAGELIFT_FRANKENPHP_IMAGE:-magelift/frankenphp-classic:8.5-local}"
name="magelift-frankenphp-tls-test"
port="${MAGELIFT_FRANKENPHP_TEST_PORT:-18443}"

docker rm -f "$name" >/dev/null 2>&1 || true
cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run -d --name "$name" -p "127.0.0.1:${port}:8443" "$image" >/dev/null
for _ in 1 2 3 4 5; do
	code="$(curl --silent --insecure \
		--resolve "localhost:${port}:127.0.0.1" \
		--output /dev/null --write-out '%{http_code}' "https://localhost:${port}/" || true)"
	if [ "$code" = "404" ]; then
		exit 0
	fi
	sleep 1
done

docker logs "$name" >&2
echo "FrankenPHP HTTPS listener did not complete a TLS request" >&2
exit 1
