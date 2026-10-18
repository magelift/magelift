#!/usr/bin/env bash
# HTTP proof that private-media trees and non-entry-point scripts never
# reach PHP-FPM, that a missing public image still reaches get.php, and
# that versioned /static/ URLs map to on-disk assets with correct MIME.
# Uses the pinned nginx + php-fpm bases from the bundled Dockerfile;
# no Magento runtime image build is needed for these HTTP assertions.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONF="$ROOT/images/php-nginx/nginx.conf"
NGINX_IMAGE="${MAGELIFT_NGINX_TEST_IMAGE:-$(sed -n 's/^ARG NGINX_BASE=//p' "$ROOT/images/php-nginx/Dockerfile")}"
PHP_IMAGE="${MAGELIFT_PHP_FPM_TEST_IMAGE:-$(sed -n 's/^ARG PHP_BASE=//p' "$ROOT/images/php-nginx/Dockerfile")}"

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

STATIC_BASE="/static/frontend/Magento/luma/en_US"
VERSIONED_PREFIX="/static/version1234567890/frontend/Magento/luma/en_US"

mkdir -p \
	"$WORK/pub/media/customer" \
	"$WORK/pub/media/downloadable" \
	"$WORK/pub/media/import" \
	"$WORK/pub/media/custom_options" \
	"$WORK/pub/media/theme_customization" \
	"$WORK/pub/media/catalog/product" \
	"$WORK/pub/errors" \
	"$WORK/pub/static/frontend/Magento/luma/en_US/css" \
	"$WORK/pub/static/frontend/Magento/luma/en_US/requirejs" \
	"$WORK/pub/static/frontend/Magento/luma/en_US/fonts"
printf 'OK-INDEX' >"$WORK/pub/index.php"
printf 'OK-GET' >"$WORK/pub/get.php"
printf 'EXECUTED' >"$WORK/pub/media/customer/probe.php"
printf 'secret-txt' >"$WORK/pub/media/customer/probe.txt"
printf 'EXECUTED' >"$WORK/pub/media/downloadable/probe.php"
printf 'EXECUTED' >"$WORK/pub/media/import/probe.php"
printf 'EXECUTED' >"$WORK/pub/media/custom_options/probe.php"
printf 'EXECUTED' >"$WORK/pub/media/catalog/product/probe.php"
printf '<theme/>' >"$WORK/pub/media/theme_customization/layout.xml"
printf 'body{color:red}' >"$WORK/pub/static/frontend/Magento/luma/en_US/css/styles-m.css"
printf 'var x=1;' >"$WORK/pub/static/frontend/Magento/luma/en_US/requirejs/require.js"
printf 'wOFF2' >"$WORK/pub/static/frontend/Magento/luma/en_US/fonts/opensans-400.woff2"
cp "$CONF" "$WORK/nginx.conf"

start_stack() {
	local conf_file="$1"
	docker rm -f "$NAME" "$NAME-nginx" >/dev/null 2>&1 || true
	docker run -d --name "$NAME" \
		-p "127.0.0.1::8080" \
		-v "$WORK/pub:/app/pub:ro" \
		"$PHP_IMAGE" >/dev/null

	docker run -d --name "$NAME-nginx" \
		--network "container:$NAME" \
		-v "$conf_file:/etc/nginx/nginx.conf:ro" \
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
}

start_stack "$WORK/nginx.conf"

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

expect_mime() {
	local path="$1" want_code="$2" want_mime="$3"
	local tmp
	tmp="$(mktemp "$WORK/curl.XXXXXX")"
	code="$(curl -sS -o "$tmp" -w '%{http_code}' "$url$path" || true)"
	ctype="$(curl -sS -I -o /dev/null -w '%{content_type}' "$url$path" 2>/dev/null || true)"
	if [[ "$code" != "$want_code" ]]; then
		printf 'nginx-media-deny-test: %s -> %s, want %s\n' "$path" "$code" "$want_code" >&2
		cat "$tmp" >&2
		exit 1
	fi
	if [[ "$ctype" != *"$want_mime"* ]]; then
		printf 'nginx-media-deny-test: %s content-type %q, want substring %q\n' \
			"$path" "$ctype" "$want_mime" >&2
		exit 1
	fi
	if grep -q OK-INDEX "$tmp"; then
		printf 'nginx-media-deny-test: %s fell through to index.php\n' "$path" >&2
		exit 1
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

expect_mime "${STATIC_BASE}/css/styles-m.css" 200 'text/css'
expect_mime "${VERSIONED_PREFIX}/css/styles-m.css" 200 'text/css'
expect_mime "${STATIC_BASE}/requirejs/require.js" 200 'javascript'
expect_mime "${VERSIONED_PREFIX}/requirejs/require.js" 200 'javascript'
expect_mime "${STATIC_BASE}/fonts/opensans-400.woff2" 200 'font/woff2'
expect_mime "${VERSIONED_PREFIX}/fonts/opensans-400.woff2" 200 'font/woff2'

expect "${VERSIONED_PREFIX}/css/no-such-file.css" 404
expect "${STATIC_BASE}/css/no-such-file.css" 404
expect "${VERSIONED_PREFIX}/evil.php" 403
expect "${VERSIONED_PREFIX}/evil.phtml" 403

# Negative proof: without version-prefix rewrite, versioned CSS misses (404/HTML).
awk '
	/^        # Deployed static files live without a version/ { skip=1; next }
	skip && /^        location \/static\/ \{/ { skip=2; next }
	skip==2 && /^        \}$/ { skip=0; next }
	skip { next }
	{ print }
' "$CONF" >"$WORK/nginx-pre-fix.conf"
start_stack "$WORK/nginx-pre-fix.conf"
neg_tmp="$(mktemp "$WORK/curl-neg.XXXXXX")"
neg_code="$(curl -sS -o "$neg_tmp" -w '%{http_code}' \
	"$url${VERSIONED_PREFIX}/css/styles-m.css" || true)"
if [[ "$neg_code" == "200" ]] && grep -q 'body{color:red}' "$neg_tmp"; then
	printf 'nginx-media-deny-test: pre-fix config unexpectedly served versioned CSS\n' >&2
	exit 1
fi
if [[ "$neg_code" != "404" ]] && ! grep -q OK-INDEX "$neg_tmp"; then
	printf 'nginx-media-deny-test: pre-fix negative check inconclusive (code=%s)\n' \
		"$neg_code" >&2
	exit 1
fi

printf 'nginx-media-deny-test ok\n'
