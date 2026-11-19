#!/bin/sh
# MageLift installer: downloads the release, verifies the Sigstore bundle on
# checksums.txt plus the archive SHA-256, and installs the binary. Cosign is
# bootstrapped from a pinned release below, so no signing tooling is needed.
#
# Pin rotation: when the release workflow moves cosign-release, update
# COSIGN_VERSION together with every COSIGN_PIN below from
# https://github.com/sigstore/cosign/releases/download/<version>/cosign_checksums.txt
# (cosign-<os>-<arch> lines). Never bump one without the other.
#
# Channels: without MAGELIFT_VERSION the script installs the latest stable
# release. Prereleases (rc, alpha, beta) never become latest; install one
# with an explicit MAGELIFT_VERSION.
# Usage: curl -fsSL https://magelift.dev/install.sh | sh
# Optional: MAGELIFT_INSTALL_DIR=/custom/bin MAGELIFT_VERSION=v1.2.3 sh
# Test-only overrides: MAGELIFT_RELEASE_BASE, MAGELIFT_COSIGN_BASE,
# MAGELIFT_COSIGN_VERSION, MAGELIFT_COSIGN_PIN.
set -eu

REPO="magelift/magelift"
BIN="magelift"
COSIGN_VERSION="${MAGELIFT_COSIGN_VERSION:-v3.1.4}"

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
  [ -n "$TAG" ] || fail "no stable release published yet (or network unreachable); set MAGELIFT_VERSION to an explicit tag"
fi

VERSION="${TAG#v}"
ARCHIVE="${BIN}_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE="${MAGELIFT_RELEASE_BASE:-https://github.com/$REPO/releases/download/$TAG}"

# Bootstrap pins: expected SHA-256 of the cosign release binary per
# platform. Overridable only for the installer harness (fixture cosign).
if [ "${MAGELIFT_COSIGN_PIN:-}" ]; then
  COSIGN_PIN="$MAGELIFT_COSIGN_PIN"
else
  case "$OS/$ARCH" in
  linux/amd64) COSIGN_PIN="424e2c97d0b45fd7a6b608a87f090f35633f4c23ece5f092d48bba11aaf4bbfa" ;;
  linux/arm64) COSIGN_PIN="ade83b7aac62e131cacfe96d958948b2150cc9efec46a570a2649b8e27dedad2" ;;
  darwin/amd64) COSIGN_PIN="9d1e4abe2b6be279ed0d2ff706fde1ccd738747c834e3e6d44039c238867334f" ;;
  darwin/arm64) COSIGN_PIN="9f9b8cd557cf2e5b3df3373a00baa005a86b02391e3e06dd0ce37b3a1533d3a4" ;;
  *) fail "unsupported platform '$OS/$ARCH' for the cosign bootstrap" ;;
  esac
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

printf 'Downloading %s (%s/%s)\n' "$TAG" "$OS" "$ARCH"
curl -fsSL -o "$TMP/$ARCHIVE" "$BASE/$ARCHIVE" || fail "download failed: $BASE/$ARCHIVE"
curl -fsSL -o "$TMP/checksums.txt" "$BASE/checksums.txt" || fail "download failed: checksums.txt"

# SHA-256 is not optional: abort when no verifier exists.
if command -v sha256sum >/dev/null 2>&1; then
  sha256_file() { sha256sum "$1" | cut -d' ' -f1; }
  sha256_check() { (cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | sha256sum -c - >/dev/null); }
elif command -v shasum >/dev/null 2>&1; then
  sha256_file() { shasum -a 256 "$1" | cut -d' ' -f1; }
  sha256_check() { (cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | shasum -a 256 -c - >/dev/null); }
else
  fail "no sha256sum or shasum available; refusing to install unverified bits"
fi

# Bootstrap cosign from the pinned release, then require the Sigstore bundle
# on checksums.txt. Every step fails closed: a missing or mismatched
# bootstrap, bundle, or signature aborts the install.
COSIGN_BASE="${MAGELIFT_COSIGN_BASE:-https://github.com/sigstore/cosign/releases/download}"
COSIGN_FILE="cosign-$OS-$ARCH"
curl -fsSL -o "$TMP/$COSIGN_FILE" "$COSIGN_BASE/$COSIGN_VERSION/$COSIGN_FILE" ||
  fail "download failed: cosign $COSIGN_VERSION for $OS/$ARCH"
if [ "$(sha256_file "$TMP/$COSIGN_FILE")" != "$COSIGN_PIN" ]; then
  fail "cosign bootstrap checksum mismatch (expected pinned $COSIGN_VERSION)"
fi
chmod +x "$TMP/$COSIGN_FILE"

curl -fsSL -o "$TMP/checksums.txt.sigstore.json" "$BASE/checksums.txt.sigstore.json" ||
  fail "download failed: checksums.txt.sigstore.json (bundle is required)"
"$TMP/$COSIGN_FILE" verify-blob \
  --bundle "$TMP/checksums.txt.sigstore.json" \
  --certificate-identity "https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$TAG" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  "$TMP/checksums.txt" >/dev/null ||
  fail "sigstore verification failed for checksums.txt"
printf 'Sigstore bundle verified (release.yml @ %s)\n' "$TAG"

sha256_check || fail "checksum verification failed for $ARCHIVE"

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
