#!/usr/bin/env bash
# Ensure the repo venv has the pinned docs stack, then print its bin dir.
# System/Homebrew mkdocs does not bundle mkdocs-material; always go through
# this helper so `make docs` and `make docs-serve` match CI.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENV="$ROOT/.venv"

if [[ ! -x "$VENV/bin/mkdocs" ]]; then
	python3 -m venv "$VENV"
fi
"$VENV/bin/pip" install --disable-pip-version-check -q -r "$ROOT/docs/requirements.txt"
printf '%s\n' "$VENV/bin"
