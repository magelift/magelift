#!/usr/bin/env bash
# Build marketing (Astro) + docs (MkDocs) into a single static tree for Cloudflare Pages.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SITE_DIR="$ROOT/websites/marketing"
DOCS_OUT="$SITE_DIR/public/docs"
VENV="$ROOT/.venv"

cd "$ROOT"

if [[ ! -x "$VENV/bin/mkdocs" ]]; then
  python3 -m venv "$VENV"
  "$VENV/bin/pip" install --disable-pip-version-check -q -r docs/requirements.txt
else
  "$VENV/bin/pip" install --disable-pip-version-check -q -r docs/requirements.txt
fi

rm -rf "$DOCS_OUT"
"$VENV/bin/mkdocs" build --strict --config-file mkdocs.yml --site-dir "$DOCS_OUT"

python3 "$SITE_DIR/scripts/generate-sitemap.py"

cd "$SITE_DIR"
if [[ ! -d node_modules ]]; then
  npm ci 2>/dev/null || npm install
fi
npm run build

echo "Site ready: $SITE_DIR/dist (docs at /docs/)"
