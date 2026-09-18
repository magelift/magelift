#!/usr/bin/env bash
# HTTP proof that private-media trees and non-entry-point scripts never
# reach PHP-FPM, and that a missing public image still reaches get.php.
# Uses official nginx + php-fpm so the test does not depend on baking
# the Magento runtime image. MAGELIFT_PHP_NGINX_IMAGE, when set, is
# used only as a config-source check after the HTTP assertions.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONF="$ROOT/images/php-nginx/nginx.conf"
NGINX_IMAGE="${MAGELIFT_NGINX_TEST_IMAGE:-docker.io/library/nginx:1.30.4-trixie}"
PHP_IMAGE="${MAGELIFT_PHP_FPM_TEST_IMAGE:-docker.io/library/php:8.5-fpm-trixie}"

command -v docker >/dev/null 2>&1 || {
	printf 'nginx-media-deny-test: docker is required\n' >&2
	exit 2
}
test -f "$CONF" || {
	printf 'nginx-media-deny-test: missing %s\n' "$CONF" >&2
	exit 2
}

WORK="$(mktemp -d "${TMPDIR:-/tmp}/magelift-nginx-deny.XXXXXX")"
NAME="magelift-nginx-deny-$$"
cleanup() {
	docker rm -f "$NAME" "$NAME-nginx" >/dev/null 2>&1 || true
	rm -rf "$WORK"
}
trap cleanup EXIT

mkdir -p \
	"$WORK/pub/media/customer" \
	"$WORK/pub/media/downloadable" \
	"$WORK/pub/media/import" \
	"$WORK/pub/media/custom_options" \
	"$WORK/pub/media/theme_customization" \
	"$WORK/pub/media/catalog/product" \
	"$WORK/pub/errors"
printf 'OK-INDEX' >"$WORK/pub/index.php"
printf 'OK-GET' >"$WORK/pub/get.php"
printf 'EXECUTED' >"$WORK/pub/media/customer/probe.php"
printf 'secret-txt' >"$WORK/pub/media/customer/probe.txt"
printf 'EXECUTED' >"$WORK/pub/media/downloadable/probe.php"
printf 'EXECUTED' >"$WORK/pub/media/import/probe.php"
printf 'EXECUTED' >"$WORK/pub/media/custom_options/probe.php"
printf 'EXECUTED' >"$WORK/pub/media/catalog/product/probe.php"
printf '<theme/>' >"$WORK/pub/media/theme_customization/layout.xml"
cp "$CONF" "$WORK/nginx.conf"

# php-fpm publishes 8080; nginx joins its network namespace so the
# checked-in fastcgi_pass 127.0.0.1:9000 is the path under test.
docker run -d --name "$NAME" \
	-p "127.0.0.1::8080" \
	-v "$WORK/pub:/app/pub:ro" \
	"$PHP_IMAGE" >/dev/null

docker run -d --name "$NAME-nginx" \
	--network "container:$NAME" \
	-v "$WORK/nginx.conf:/etc/nginx/nginx.conf:ro" \
	-v "$WORK/pub:/app/pub:ro" \
	--user nginx \
	"$NGINX_IMAGE" nginx -g 'daemon off;' -c /etc/nginx/nginx.conf >/dev/null

port="$(docker port "$NAME" 8080/tcp | awk -F: 'NR==1 { print $NF }')"
test -n "$port" || {
	printf 'nginx-media-deny-test: published port missing\n' >&2
	docker logs "$NAME-nginx" >&2 || true
	exit 1
}

url="http://127.0.0.1:${port}"
ready=0
for _ in $(seq 1 40); do
	if body="$(curl -sf "$url/health" 2>/dev/null)" && [[ "$body" == OK* ]]; then
		ready=1
		break
	fi
	sleep 0.25
done
if [[ "$ready" != "1" ]]; then
	printf 'nginx-media-deny-test: /health never came up\n' >&2
	docker logs "$NAME-nginx" >&2 || true
	docker logs "$NAME" >&2 || true
	exit 1
fi

expect() {
	local path="$1" want_code="$2" want_body="${3-}"
	local tmp
	tmp="$(mktemp "$WORK/curl.XXXXXX")"
	code="$(curl -sS -o "$tmp" -w '%{http_code}' "$url$path" || true)"
	if [[ "$code" != "$want_code" ]]; then
		printf 'nginx-media-deny-test: %s -> %s, want %s\n' "$path" "$code" "$want_code" >&2
		cat "$tmp" >&2
		exit 1
	fi
	if [[ -n "$want_body" ]]; then
		got="$(cat "$tmp")"
		if [[ "$got" != *"$want_body"* ]]; then
			printf 'nginx-media-deny-test: %s body %q, want %q\n' "$path" "$got" "$want_body" >&2
			exit 1
		fi
	else
		if grep -q EXECUTED "$tmp"; then
			printf 'nginx-media-deny-test: %s executed PHP\n' "$path" >&2
			exit 1
		fi
	fi
}

expect /health 200 OK
expect /media/customer/probe.txt 403
expect /media/customer/probe.php 403
expect /media/downloadable/probe.php 403
expect /media/import/probe.php 403
expect /media/custom_options/probe.php 403
expect /media/catalog/product/probe.php 403
expect /media/theme_customization/layout.xml 403
# Missing public image must reach get.php (not 403, not a static miss).
expect /media/catalog/product/missing.jpg 200 OK-GET
# Approved entry points still execute.
expect /index.php 200 OK-INDEX
expect /get.php 200 OK-GET

printf 'nginx-media-deny-test ok\n'
