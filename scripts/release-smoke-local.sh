#!/usr/bin/env bash
# Offline release packaging smoke: goreleaser check + one host binary + legal files.
#
# ALWAYS serial / single-target on developer Macs. Parallel goreleaser/go builds
# (6 GOOS/GOARCH × many compiler workers) exhaust RAM/SWAP and have caused
# kernel panics. Do not remove --single-target or raise parallelism here.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export GOMAXPROCS="${GOMAXPROCS:-1}"
# Cap package-level compile parallelism inside the single go build as well.
export GOFLAGS="${GOFLAGS:--p=1}"

printf '+ goreleaser check\n'
go run github.com/goreleaser/goreleaser/v2@v2.12.7 check

printf '+ goreleaser build --snapshot --single-target --parallelism=1 GOMAXPROCS=%s GOFLAGS=%s (host only)\n' \
	"$GOMAXPROCS" "$GOFLAGS"
rm -rf dist
go run github.com/goreleaser/goreleaser/v2@v2.12.7 build --snapshot --clean \
	--single-target --parallelism=1

bin="$(find dist -type f \( -name 'magelift' -o -name 'magelift.exe' \) | head -1 || true)"
if [[ -z "$bin" ]]; then
	printf 'magelift binary missing under dist/\n' >&2
	find dist -maxdepth 4 -type f 2>/dev/null | head -40 >&2 || true
	exit 1
fi

# Archives.files in .goreleaser.yaml — confirm sources exist for real releases.
test -f LICENSE
test -f NOTICE
test -f README.md

chmod +x "$bin" 2>/dev/null || true
if [[ "$bin" == *.exe ]]; then
	printf 'skipping version exec for windows artifact on this host\n'
else
	"$bin" version >/dev/null
fi

# Pack a minimal smoke tarball the way archives.files would.
smoke_dir="$(mktemp -d /tmp/magelift-smoke.XXXXXX)"
trap 'rm -rf "$smoke_dir"' EXIT
cp "$bin" LICENSE NOTICE README.md "$smoke_dir/"
(
	cd "$smoke_dir"
	tar -czf magelift-smoke.tar.gz magelift LICENSE NOTICE README.md 2>/dev/null \
		|| tar -czf magelift-smoke.tar.gz magelift.exe LICENSE NOTICE README.md
)
tar -tzf "$smoke_dir/magelift-smoke.tar.gz" | rg -q 'LICENSE'
tar -tzf "$smoke_dir/magelift-smoke.tar.gz" | rg -q 'NOTICE'

printf 'release smoke ok binary=%s (serial single-target)\n' "$bin"
