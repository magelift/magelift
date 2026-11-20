#!/usr/bin/env bash
# Check the nginx health endpoint before Magento starts.
set -Eeuo pipefail

name="magelift-health-nginx-$$"
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

image="${MAGELIFT_PHP_NGINX_IMAGE:-magelift/php-nginx:local}"
docker run -d --name "$name" -p "127.0.0.1:${port}:8080" --entrypoint nginx \
	"$image" -g 'daemon off;' -c /etc/nginx/nginx.conf >/dev/null

url="http://127.0.0.1:${port}/health"
for _ in $(seq 1 40); do
	if body="$(curl -sf "$url" 2>/dev/null)"; then
		if [[ "$body" == OK* ]]; then
			# Prefer in-container curl when present (matches ECS health check).
			if docker exec "$name" curl -sf http://127.0.0.1:8080/health >/dev/null 2>&1; then
				printf '%s /health ok (host+container curl)\n' nginx
			else
				printf '%s /health ok (host curl; container curl missing)\n' nginx
			fi
			exit 0
		fi
		printf '%s /health unexpected body: %q\n' nginx "$body" >&2
		exit 1
	fi
	sleep 0.25
done
docker logs "$name" >&2 || true
printf '%s /health failed\n' nginx >&2
exit 1
