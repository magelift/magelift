#!/usr/bin/env bash
# Curl MageLift runtime /health short-circuits (no Magento bootstrap).
set -Eeuo pipefail

mode="${1:?usage: image-health-test.sh nginx|frankenphp}"
name="magelift-health-${mode}-$$"
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

case "$mode" in
nginx)
	image="${MAGELIFT_PHP_RUNTIME_IMAGE:-magelift/php-runtime:local}"
	docker run -d --name "$name" -p "127.0.0.1:${port}:8080" --entrypoint nginx \
		"$image" -g 'daemon off;' -c /etc/nginx/nginx.conf >/dev/null
	;;
frankenphp)
	image="${MAGELIFT_FRANKENPHP_IMAGE:-magelift/frankenphp-classic:8.5-local}"
	docker run -d --name "$name" -p "127.0.0.1:${port}:8080" "$image" >/dev/null
	;;
*)
	printf 'usage: image-health-test.sh nginx|frankenphp\n' >&2
	exit 2
	;;
esac

url="http://127.0.0.1:${port}/health"
for _ in $(seq 1 40); do
	if body="$(curl -sf "$url" 2>/dev/null)"; then
		if [[ "$body" == OK* ]]; then
			# Prefer in-container curl when present (matches ECS health check).
			if docker exec "$name" curl -sf http://127.0.0.1:8080/health >/dev/null 2>&1; then
				printf '%s /health ok (host+container curl)\n' "$mode"
			else
				printf '%s /health ok (host curl; container curl missing)\n' "$mode"
			fi
			exit 0
		fi
		printf '%s /health unexpected body: %q\n' "$mode" "$body" >&2
		exit 1
	fi
	sleep 0.25
done
docker logs "$name" >&2 || true
printf '%s /health failed\n' "$mode" >&2
exit 1
