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
go run github.com/goreleaser/goreleaser/v2@v2.18.1 check -f .goreleaser.yaml
go run github.com/goreleaser/goreleaser/v2@v2.18.1 check -f .goreleaser.dialproof.yaml

# The dialproof config is the slim linux/amd64 shape; the full matrix
# belongs on CI.
printf '+ goreleaser release --snapshot GOMEMLIMIT=%s (dialproof config)\n' \
	"$GOMEMLIMIT"
rm -rf dist
go run github.com/goreleaser/goreleaser/v2@v2.18.1 release --snapshot --clean \
	-f .goreleaser.dialproof.yaml --skip=publish,sign,sbom

cli_archive="$(find dist -maxdepth 1 -type f -name 'magelift_*.tar.gz' -o -maxdepth 1 -type f -name 'magelift_*.zip' | head -1 || true)"
if [[ -z "$cli_archive" ]]; then
	printf 'CLI archive missing under dist/\n' >&2
	find dist -maxdepth 2 2>/dev/null | head -40 >&2 || true
	exit 1
fi

provider_bin="$(find dist -type f \( -name 'magelift-provider-gcp' -o -name 'magelift-provider-gcp.exe' \) | head -1 || true)"
if [[ -z "$provider_bin" ]]; then
	printf 'magelift-provider-gcp binary missing under dist/\n' >&2
	find dist -maxdepth 4 -type f 2>/dev/null | head -40 >&2 || true
	exit 1
fi

# The release matrix must carry the provider into checksums.txt, or the
# lockfile generator has nothing to pin on a real tag.
grep -q 'magelift-provider-gcp_' dist/checksums.txt \
	|| { printf 'provider missing from dist/checksums.txt\n' >&2; exit 1; }

# Archives.files in .goreleaser.yaml; confirm the real archive packs them.
tar -tzf "$cli_archive" | rg -q 'LICENSE'
tar -tzf "$cli_archive" | rg -q 'NOTICE'

bin_dir="$(mktemp -d "${TMPDIR:-/tmp}/magelift-smoke.XXXXXX")"
trap 'rm -rf "$bin_dir"' EXIT
tar -xzf "$cli_archive" -C "$bin_dir"
if [[ "$cli_archive" == *.zip ]]; then
	printf 'skipping version exec for windows artifact on this host\n'
else
	"$bin_dir/magelift" version >/dev/null
fi

printf 'release smoke ok archive=%s provider=%s\n' "$cli_archive" "$provider_bin"
