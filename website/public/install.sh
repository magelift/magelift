#!/bin/sh
# MageLift installer: downloads the latest release, verifies its SHA-256
# checksum against the signed checksums.txt, and installs the binary.
# Usage: curl -fsSL https://magelift.dev/install.sh | sh
# Optional: MAGELIFT_INSTALL_DIR=/custom/bin MAGELIFT_VERSION=v1.2.3 sh
set -eu

REPO="magelift/magelift"
BIN="magelift"

fail() {
  printf 'install: %s\n' "$1" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "required tool missing: $1"
}

need curl
need tar
need uname

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux | darwin) ;;
  *) fail "unsupported OS '$OS' (use Scoop on Windows: https://github.com/$REPO#install)" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64 | amd64) ARCH="amd64" ;;
  arm64 | aarch64) ARCH="arm64" ;;
  *) fail "unsupported architecture '$ARCH'" ;;
esac

if [ "${MAGELIFT_VERSION:-}" ]; then
  TAG="$MAGELIFT_VERSION"
else
  need sed
  TAG="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')"
  [ -n "$TAG" ] || fail "could not resolve the latest release tag (no release yet?)"
fi

VERSION="${TAG#v}"
ARCHIVE="${BIN}_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$TAG"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

printf 'Downloading %s (%s/%s)\n' "$TAG" "$OS" "$ARCH"
curl -fsSL -o "$TMP/$ARCHIVE" "$BASE/$ARCHIVE" || fail "download failed: $BASE/$ARCHIVE"
curl -fsSL -o "$TMP/checksums.txt" "$BASE/checksums.txt" || fail "download failed: checksums.txt"

# SHA-256 verification is not optional: abort when no verifier exists.
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | sha256sum -c - >/dev/null) \
    || fail "checksum verification failed for $ARCHIVE"
elif command -v shasum >/dev/null 2>&1; then
  (cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | shasum -a 256 -c - >/dev/null) \
    || fail "checksum verification failed for $ARCHIVE"
else
  fail "no sha256sum or shasum available; refusing to install unverified bits"
fi

# Stronger provenance check when cosign is on the machine: the checksum file
# itself was signed keylessly by the release workflow.
if command -v cosign >/dev/null 2>&1; then
  if curl -fsSL -o "$TMP/checksums.txt.sigstore.json" "$BASE/checksums.txt.sigstore.json"; then
    cosign verify-blob \
      --bundle "$TMP/checksums.txt.sigstore.json" \
      --certificate-identity "https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$TAG" \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com \
      "$TMP/checksums.txt" >/dev/null \
      && printf 'Sigstore bundle verified (release.yml @ %s)\n' "$TAG" \
      || fail "sigstore verification failed for checksums.txt"
  fi
fi

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" "$BIN" || fail "archive did not contain $BIN"

DEST="${MAGELIFT_INSTALL_DIR:-}"
if [ -z "$DEST" ]; then
  if [ -w /usr/local/bin ]; then
    DEST="/usr/local/bin"
  else
    DEST="$HOME/.local/bin"
  fi
fi
mkdir -p "$DEST" 2>/dev/null || true

if [ -w "$DEST" ]; then
  mv "$TMP/$BIN" "$DEST/$BIN"
  chmod +x "$DEST/$BIN"
elif command -v sudo >/dev/null 2>&1; then
  printf 'Installing to %s with sudo\n' "$DEST"
  sudo mv "$TMP/$BIN" "$DEST/$BIN"
  sudo chmod +x "$DEST/$BIN"
else
  fail "cannot write to $DEST and sudo is unavailable; set MAGELIFT_INSTALL_DIR"
fi

printf 'Installed %s\n' "$DEST/$BIN"
"$DEST/$BIN" version 2>/dev/null || printf 'Run: %s version\n' "$DEST/$BIN"
case ":$PATH:" in
  *":$DEST:"*) ;;
  *) printf 'Note: %s is not on your PATH\n' "$DEST" ;;
esac
