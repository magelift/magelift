#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
case "${1:-}" in
  gcp|azure) cd "$root/providers/$1" ;;
  *) echo 'usage: check-provider-tidy.sh gcp|azure' >&2; exit 2 ;;
esac

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

# Restore the checksums for the release pins that downstream consumers use.
go mod download -modfile="$modfile" "$module" "$module/sdk"
diff -u go.mod "$modfile"
diff -u go.sum "$scratch/go.sum"
