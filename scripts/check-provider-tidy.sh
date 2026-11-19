#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
provider="${1:-}"
if [[ ! "$provider" =~ ^[a-z0-9-]+$ || ! -f "$root/providers/$provider/go.mod" ]]; then
  printf 'usage: check-provider-tidy.sh <provider> (expected providers/<provider>/go.mod)\n' >&2
  exit 2
fi
cd "$root/providers/$provider"

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
cp go.mod go.sum "$scratch/"
modfile="$scratch/go.mod"
module=github.com/magelift/magelift
export GOWORK=off

# Tidy resolves module requirements against published versions. Use this
# checkout for both first-party modules while it calculates dependencies.
go mod edit -modfile="$modfile" -replace="$module=$root" -replace="$module/sdk=$root/sdk"
go mod tidy -modfile="$modfile"
go mod edit -modfile="$modfile" -dropreplace="$module" -dropreplace="$module/sdk"

diff -u go.mod "$modfile"

# Local replacements have no checksums. Release builds verify those modules
# from the public proxy once their tags are available.
external_sums() {
  awk -v core="$module" '$1 != core && $1 != core "/sdk"' "$1"
}
external_sums go.sum > "$scratch/expected.sum"
external_sums "$scratch/go.sum" > "$scratch/tidy.sum"
diff -u "$scratch/expected.sum" "$scratch/tidy.sum"
