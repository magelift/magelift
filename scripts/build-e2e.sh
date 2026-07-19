#!/usr/bin/env bash
set -euo pipefail

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fixture="$work/repository"
mkdir -p "$fixture"
cp -R "$repository_root/tests/fixtures/build/." "$fixture/"

git -C "$fixture" init --quiet
git -C "$fixture" config user.email fixture@magelift.invalid
git -C "$fixture" config user.name "MageLift fixture"
git -C "$fixture" add .
git -C "$fixture" commit --quiet -m "test: create build fixture"

export XDG_CACHE_HOME="$work/cache"
result="$work/result.json"
go run "$repository_root/cmd/magelift" \
    --config "$fixture/magelift.yaml" \
    --output json \
    build >"$result"

php -r '
$result = json_decode(file_get_contents($argv[1]), true, flags: JSON_THROW_ON_ERROR);
if (!is_file($result["manifest"]) || !preg_match("/^sha256:[a-f0-9]{64}$/", $result["image"]["digest"])) {
    fwrite(STDERR, "build result is missing its manifest or immutable digest\n");
    exit(1);
}
$manifest = json_decode(file_get_contents($result["manifest"]), true, flags: JSON_THROW_ON_ERROR);
if ($manifest["imageDigest"] !== $result["image"]["digest"] || $manifest["sourceRevision"] === "") {
    fwrite(STDERR, "artifact manifest does not match the built image\n");
    exit(1);
}
' "$result"

image=$(php -r '$result = json_decode(file_get_contents($argv[1]), true, flags: JSON_THROW_ON_ERROR); echo $result["image"]["imageReference"];' "$result")
docker run --rm --entrypoint sh "$image" -c '
test "$(id -u)" = 10001
test ! -e /app/.git
test ! -e /app/.magelift
test ! -e /app/magelift.yaml
test ! -e /app/auth.json
test -f /app/app/etc/env.php
! grep -Eiq "password|secret|MAGELIFT" /app/app/etc/env.php
'
