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
go run github.com/goreleaser/goreleaser/v2@v2.20.2 check -f .goreleaser.yaml
go run github.com/goreleaser/goreleaser/v2@v2.20.2 check -f .goreleaser.dialproof.yaml

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
go run github.com/goreleaser/goreleaser/v2@v2.20.2 release --snapshot --clean --parallelism 1 \
	-f "$WORK/goreleaser.yaml" --skip=publish,sign,sbom

cli_archive="$(find "$SMOKE_DIST" -maxdepth 1 -type f -name 'magelift_*.tar.gz' -o -maxdepth 1 -type f -name 'magelift_*.zip' | head -1 || true)"
if [[ -z "$cli_archive" ]]; then
	printf 'CLI archive missing under %s/\n' "$SMOKE_DIST" >&2
	find "$SMOKE_DIST" -maxdepth 2 2>/dev/null | head -40 >&2 || true
	exit 1
fi

provider_bins=()
provider_names=()
provider_bytes=0
for manifest in "$ROOT"/providers/*/go.mod; do
	[[ -f "$manifest" ]] || continue
	provider="${manifest%/go.mod}"
	provider="${provider##*/}"
	binary_name="magelift-provider-$provider"
	provider_bin="$(find "$SMOKE_DIST" -type f \( -name "$binary_name" -o -name "$binary_name.exe" \) | head -1 || true)"
	if [[ -z "$provider_bin" ]]; then
		printf '%s binary missing under dist/\n' "$binary_name" >&2
		exit 1
	fi
	if ! grep -q "${binary_name}_" "$SMOKE_DIST/checksums.txt"; then
		printf '%s missing from %s/checksums.txt\n' "$binary_name" "$SMOKE_DIST" >&2
		exit 1
	fi
	provider_bins+=("$provider_bin")
	provider_names+=("$provider")
	provider_bytes=$((provider_bytes + $(wc -c < "$provider_bin")))
done
if [[ "${#provider_bins[@]}" -eq 0 ]]; then
	printf 'no provider binaries were built under %s/\n' "$SMOKE_DIST" >&2
	exit 1
fi

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

printf 'bytes cli_archive=%s installed_cli=%s provider_binaries=%s (tools and Docker images excluded)\n' \
	"$(wc -c < "$cli_archive" | tr -d ' ')" \
	"$(wc -c < "$bin_dir/magelift" | tr -d ' ')" \
	"$provider_bytes"

printf 'release smoke ok archive=%s providers=%s\n' "$cli_archive" "${provider_names[*]}"
