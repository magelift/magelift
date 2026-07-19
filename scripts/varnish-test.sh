#!/usr/bin/env bash
set -Eeuo pipefail

image="${MAGELIFT_VARNISH_IMAGE:-docker.io/library/varnish:8.0.2@sha256:4b595728592a5b9709c9aac15368ca492e9742fb269ed12466b434a62b2c1b63}"
name="magelift-varnish-test"
container_id=""

docker rm -f "$name" >/dev/null 2>&1 || true
cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

container_id="$(docker run -d --name "$name" --read-only \
	--tmpfs /var/lib/varnish:rw,exec,uid=1000,gid=1000,mode=0750,size=384m \
	-e VARNISH_BACKEND_HOST=127.0.0.1 \
	-e VARNISH_BACKEND_PORT=8080 \
	-e VARNISH_HTTP_PORT=6081 \
	-e VARNISH_SIZE=256M \
	"$image")"

for _ in 1 2 3 4 5 6 7 8 9 10; do
	rc="$(docker inspect -f '{{.State.Status}}' "$container_id" 2>/dev/null || true)"
	if [ "$rc" = "running" ]; then
		exit 0
	fi
	if [ "$rc" = "exited" ] || [ "$rc" = "dead" ]; then
		docker logs "$container_id" >&2
		echo "Varnish did not start with its read-only root and executable VSM tmpfs" >&2
		exit 1
	fi
	sleep 1
done

docker logs "$container_id" >&2
echo "Varnish startup did not become observable" >&2
exit 1
