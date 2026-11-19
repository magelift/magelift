#!/usr/bin/env bash
# Test a consumer checkout in act, then exercise local OCI digest handoff.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
name="magelift-registry-${BASHPID}"
ref=""
registry_id=""
cleanup(){
    if [[ -n "$ref" ]]; then docker image rm "$ref" >/dev/null 2>&1 || true; fi
    if [[ -n "$registry_id" ]]; then docker rm -f -v "$registry_id" >/dev/null 2>&1 || true; fi
    rm -rf "$tmp"
}
trap cleanup EXIT
(cd "$root" && go build -o "$tmp/magelift" ./cmd/magelift)
cp "$root/tests/act/project.yml" "$tmp/workflow.yml"
cp "$root/tests/fixtures/ci/aws/magelift.php" "$tmp/magelift.php"
cd "$tmp"
git init -q -b main
# This workflow consists solely of shell steps. The PHP runner also verifies
# that project evaluation works without any host PHP installation or cloud CLI.
runner=${MAGELIFT_ACT_PHP_IMAGE:-docker.io/library/php:8.5.13-fpm-trixie@sha256:52ba6f178d0a08f348578b03595661b7ff13defafacf4c823d045f2ecba8db8f}
act workflow_dispatch -W workflow.yml -j project --bind --pull=false --container-daemon-socket - --container-options "--user $(id -u):$(id -g)" -P "ubuntu-latest=$runner"
registry_id=$(docker run -d --name "$name" -p 127.0.0.1::5000 docker.io/library/registry:3.1.2@sha256:82f6ff4a7f982960c8aa51617fea3aabc485df6e55cce80d22deb1de36f1bc3c)
address=$(docker port "$registry_id" 5000/tcp)
curl --fail --silent --retry-all-errors --retry 10 --max-time 5 "http://$address/v2/" >/dev/null
ref="$address/magelift/shop:fixture"
mkdir oci
cp magelift.php oci/
printf 'FROM scratch\nCOPY magelift.php /fixture.php\n' > oci/Dockerfile
docker build -q -t "$ref" oci
docker push "$ref"
digest=$(docker image inspect "$ref" --format '{{index .RepoDigests 0}}')
[[ "$digest" == "$address/magelift/shop@sha256:"* ]]
docker image rm "$ref" >/dev/null
docker pull "$digest"
docker image rm "$digest" >/dev/null
echo "act project workflow and local registry digest round trip passed"
