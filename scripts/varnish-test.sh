#!/usr/bin/env bash
# Verify Varnish startup + /health pass-through to a backend short-circuit, and
# that MageLift's VCL compiles.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image="${MAGELIFT_VARNISH_IMAGE:-docker.io/library/varnish:8.0.2@sha256:4b595728592a5b9709c9aac15368ca492e9742fb269ed12466b434a62b2c1b63}"
backend="magelift-varnish-backend-$$"
name="magelift-varnish-test-$$"
network="magelift-varnish-net-$$"
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"

cleanup() {
	docker rm -f "$name" "$backend" >/dev/null 2>&1 || true
	docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create "$network" >/dev/null

# Start nginx with the health short-circuit already in place. Avoid `nginx -s reload`
# (alpine images often have no /run/nginx.pid yet, so reload fails on CI).
docker run -d --name "$backend" --network "$network" docker.io/library/nginx:1.27-alpine \
	sh -c 'printf "server { listen 80; location = /health { default_type text/plain; return 200 \"OK\\n\"; } location / { return 404; } }\n" > /etc/nginx/conf.d/default.conf && exec nginx -g "daemon off;"' >/dev/null

# Read-only root + executable VSM tmpfs (ECS sidecar contract).
docker run -d --name "${name}-boot" --read-only \
	--tmpfs /var/lib/varnish:rw,exec,uid=1000,gid=1000,mode=0750,size=384m \
	-e VARNISH_BACKEND_HOST=127.0.0.1 \
	-e VARNISH_BACKEND_PORT=8080 \
	-e VARNISH_HTTP_PORT=6081 \
	-e VARNISH_SIZE=64M \
	"$image" >/dev/null
for _ in $(seq 1 15); do
	rc="$(docker inspect -f '{{.State.Status}}' "${name}-boot" 2>/dev/null || true)"
	if [ "$rc" = "running" ]; then
		docker rm -f "${name}-boot" >/dev/null 2>&1 || true
		break
	fi
	if [ "$rc" = "exited" ] || [ "$rc" = "dead" ]; then
		docker logs "${name}-boot" >&2
		echo "Varnish did not start with its read-only root and executable VSM tmpfs" >&2
		exit 1
	fi
	sleep 1
done

# Stock image pass-through against the health backend.
docker run -d --name "$name" --network "$network" \
	--tmpfs /var/lib/varnish:rw,exec,uid=1000,gid=1000,mode=0750,size=384m \
	-p "127.0.0.1:${port}:6081" \
	-e VARNISH_BACKEND_HOST="$backend" \
	-e VARNISH_BACKEND_PORT=80 \
	-e VARNISH_HTTP_PORT=6081 \
	-e VARNISH_SIZE=64M \
	"$image" >/dev/null

for _ in $(seq 1 30); do
	if body="$(curl -sf "http://127.0.0.1:${port}/health" 2>/dev/null)"; then
		if [[ "$body" == OK* ]]; then
			docker run --rm -v "${ROOT}/images/varnish/default.vcl:/etc/varnish/default.vcl:ro" \
				--entrypoint varnishd "$image" -C -f /etc/varnish/default.vcl >/dev/null 2>&1
			printf 'varnish /health pass-through ok; MageLift VCL compiles\n'
			exit 0
		fi
		printf 'varnish /health unexpected body: %q\n' "$body" >&2
		exit 1
	fi
	sleep 0.5
done
docker logs "$name" >&2 || true
printf 'varnish /health pass-through failed\n' >&2
exit 1
