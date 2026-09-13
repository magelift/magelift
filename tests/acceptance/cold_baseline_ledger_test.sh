#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ledger="$repo_root/scripts/acceptance/cold-baselines-2026-08-08.tsv"

test -s "$ledger"
awk -F '\t' '
BEGIN {
  expected = 26
  rows = 0
  allowed = "^(certified|experimental|unsupported|unavailable|blocked|not-run)$"
}
/^#/ { next }
{
  rows++
  if (NF != expected) {
    printf "row %d has %d fields, want %d\n", NR, NF, expected > "/dev/stderr"
    failed = 1
  }
  if ($24 !~ allowed) {
    printf "row %d has invalid gate status %s\n", NR, $24 > "/dev/stderr"
    failed = 1
  }
  for (field = 1; field <= 25; field++) {
    if ($field == "") {
      printf "row %d has an empty required field %d\n", NR, field > "/dev/stderr"
      failed = 1
    }
  }
  if ($24 == "certified" && ($9 == "unknown" || $13 == "unknown" || $20 == "not-resolved")) {
    printf "row %d claims certified with unresolved version or artifact\n", NR > "/dev/stderr"
    failed = 1
  }
}
END {
  if (rows < 1) {
    print "cold baseline ledger has no rows" > "/dev/stderr"
    failed = 1
  }
  exit failed
}' "$ledger"

printf '%s\n' 'cold baseline ledger shape: PASS'
