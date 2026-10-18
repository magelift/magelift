#!/usr/bin/env bash
# Build the site (Astro) + docs (MkDocs) into a single static tree for GitHub Pages.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SITE_DIR="$ROOT/website"
DOCS_OUT="$SITE_DIR/public/docs"

cd "$ROOT"

VENV_BIN="$(scripts/docs-venv.sh)"

rm -rf "$DOCS_OUT"
"$VENV_BIN/mkdocs" build --strict --config-file mkdocs.yml --site-dir "$DOCS_OUT"

python3 "$SITE_DIR/scripts/generate-sitemap.py"

cd "$SITE_DIR"
if [[ ! -d node_modules ]]; then
  npm ci 2>/dev/null || npm install
fi
npm run build

echo "Site ready: $SITE_DIR/dist (docs at /docs/)"
