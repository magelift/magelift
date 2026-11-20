#!/usr/bin/env bash
# Config-shape proof for the media deny + PHP allowlist. HTTP behavior
# is scripts/nginx-media-deny-test.sh, run from image-test and the
# container CI job after a runtime image exists.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONF="$ROOT/images/php-nginx/nginx.conf"

grep -q 'location ^~ /media/customer/' "$CONF" || {
	printf 'nginx config lost ^~ deny for /media/customer/\n' >&2
	exit 1
}
grep -q 'location ^~ /media/downloadable/' "$CONF" || {
	printf 'nginx config lost ^~ deny for /media/downloadable/\n' >&2
	exit 1
}
grep -q 'location ^~ /media/import/' "$CONF" || {
	printf 'nginx config lost ^~ deny for /media/import/\n' >&2
	exit 1
}
grep -q 'location ^~ /media/custom_options/' "$CONF" || {
	printf 'nginx config lost ^~ deny for /media/custom_options/\n' >&2
	exit 1
}
grep -q 'theme_customization' "$CONF" || {
	printf 'nginx config lost theme_customization deny\n' >&2
	exit 1
}
grep -q 'index|get|static' "$CONF" || {
	printf 'nginx config lost Magento PHP entry-point allowlist\n' >&2
	exit 1
}
if grep -n 'location ~ \\.php\$' "$CONF" | grep -vq 'index|get|static'; then
	printf 'nginx config still has a generic PHP handler\n' >&2
	exit 1
fi
if grep -q 'location ~ \\.php\$ {' "$CONF"; then
	printf 'nginx config still has a generic PHP handler\n' >&2
	exit 1
fi

printf 'nginx media deny shape ok\n'
