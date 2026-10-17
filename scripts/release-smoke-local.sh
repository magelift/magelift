#!/usr/bin/env bash
# Local packaging smoke: goreleaser check + one snapshot release (host only)
# + legal files + provider matrix presence. Signing, SBOM, and publish are
# skipped: full multi-platform matrices with signatures belong on CI.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# shellcheck source=go-memlimit.sh
source "$ROOT/scripts/go-memlimit.sh"
magelift_apply_go_memlimit

printf '+ goreleaser check (both configs)\n'
go run github.com/goreleaser/goreleaser/v2@v2.18.3 check -f .goreleaser.yaml
go run github.com/goreleaser/goreleaser/v2@v2.18.3 check -f .goreleaser.dialproof.yaml

# The dialproof config is the slim linux/amd64 shape; the full matrix
# belongs on CI.
printf '+ goreleaser release --snapshot GOMEMLIMIT=%s (dialproof config)\n' \
	"$GOMEMLIMIT"
# Keep existing local release artifacts untouched.
WORK="$(mktemp -d "${TMPDIR:-/tmp}/magelift-release-smoke.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
export MAGELIFT_RELEASE_SMOKE_DIST="$WORK/dist"
SMOKE_DIST="$MAGELIFT_RELEASE_SMOKE_DIST"
{ printf "dist: '%s'\n" "${SMOKE_DIST//\'/\'\'}"; cat .goreleaser.dialproof.yaml; } > "$WORK/goreleaser.yaml"
go run github.com/goreleaser/goreleaser/v2@v2.18.3 release --snapshot --clean \
	-f "$WORK/goreleaser.yaml" --skip=publish,sign,sbom

cli_archive="$(find "$SMOKE_DIST" -maxdepth 1 -type f -name 'magelift_*.tar.gz' -o -maxdepth 1 -type f -name 'magelift_*.zip' | head -1 || true)"
if [[ -z "$cli_archive" ]]; then
	printf 'CLI archive missing under %s/\n' "$SMOKE_DIST" >&2
	find "$SMOKE_DIST" -maxdepth 2 2>/dev/null | head -40 >&2 || true
	exit 1
fi

provider_bin="$(find "$SMOKE_DIST" -type f \( -name 'magelift-provider-gcp' -o -name 'magelift-provider-gcp.exe' \) | head -1 || true)"
if [[ -z "$provider_bin" ]]; then
	printf 'magelift-provider-gcp binary missing under dist/\n' >&2
	find "$SMOKE_DIST" -maxdepth 4 -type f 2>/dev/null | head -40 >&2 || true
	exit 1
fi

# The release matrix must carry the provider into checksums.txt, or the
# lockfile generator has nothing to pin on a real tag.
grep -q 'magelift-provider-gcp_' "$SMOKE_DIST/checksums.txt" \
	|| { printf 'provider missing from "$SMOKE_DIST/checksums.txt"\n' >&2; exit 1; }

# Archives.files in .goreleaser.yaml; confirm the real archive packs them.
tar -tzf "$cli_archive" > "$WORK/archive-files.txt"
rg -q 'LICENSE' "$WORK/archive-files.txt"
rg -q 'NOTICE' "$WORK/archive-files.txt"

bin_dir="$(mktemp -d "$WORK/bin.XXXXXX")"
tar -xzf "$cli_archive" -C "$bin_dir"
if [[ "$cli_archive" == *.zip ]]; then
	printf 'skipping version exec for windows artifact on this host\n'
else
	"$bin_dir/magelift" version >/dev/null
fi

printf 'bytes cli_archive=%s installed_cli=%s provider=%s (tools and Docker images excluded)\n' \
	"$(wc -c < "$cli_archive" | tr -d ' ')" \
	"$(wc -c < "$bin_dir/magelift" | tr -d ' ')" \
	"$(wc -c < "$provider_bin" | tr -d ' ')"

printf 'release smoke ok archive=%s provider=%s\n' "$cli_archive" "$provider_bin"
