#!/usr/bin/env bash
# Installer trust harness: drives website/public/install.sh against a local
# fixture server covering success plus every fail-closed path (tampered
# archive, missing bundle, failed verification, tampered bootstrap).
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL="$ROOT/website/public/install.sh"
TAG="v9.9.9-harness"
VERSION="${TAG#v}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64 | amd64) ARCH="amd64" ;;
  arm64 | aarch64) ARCH="arm64" ;;
  *) echo "harness: unsupported architecture '$ARCH'" >&2; exit 1 ;;
esac
ARCHIVE="magelift_${VERSION}_${OS}_${ARCH}.tar.gz"
COSIGN_FILE="cosign-$OS-$ARCH"

WORK="$(mktemp -d /tmp/magelift-install-harness.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

free_port() {
  python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'
}

# Fixture cosign: asserts the verify-blob arguments the installer must pass
# (bundle, pinned identity, issuer), records them, and exits per
# COSIGN_FIXTURE_EXIT (default 0).
write_fixture_cosign() {
  local path="$1"
  cat >"$path" <<'COSIGN'
#!/bin/sh
set -eu
marker="${COSIGN_FIXTURE_MARKER:-/dev/null}"
bundle=""; identity=""; issuer=""; subject=""
while [ $# -gt 0 ]; do
  case "$1" in
    verify-blob) shift ;;
    --bundle) bundle="$2"; shift 2 ;;
    --certificate-identity) identity="$2"; shift 2 ;;
    --certificate-oidc-issuer) issuer="$2"; shift 2 ;;
    *) subject="$1"; shift ;;
  esac
done
printf 'bundle=%s\nidentity=%s\nissuer=%s\nsubject=%s\n' "$bundle" "$identity" "$issuer" "$subject" >"$marker"
[ -n "$bundle" ] && [ -f "$bundle" ] || exit 3
[ -n "$identity" ] || exit 4
[ -n "$issuer" ] || exit 5
[ -n "$subject" ] && [ -f "$subject" ] || exit 6
exit "${COSIGN_FIXTURE_EXIT:-0}"
COSIGN
  chmod +x "$path"
}

start_server() {
  local root="$1"
  PORT="$(free_port)"
  python3 -m http.server "$PORT" --directory "$root" --bind 127.0.0.1 >/dev/null 2>&1 &
  SERVER_PID=$!
  for _ in $(seq 1 50); do
    curl -fsSL "http://127.0.0.1:$PORT/" >/dev/null 2>&1 && return 0
    sleep 0.1
  done
  echo "harness: fixture server did not start" >&2
  return 1
}

stop_server() {
  if [ "${SERVER_PID:-}" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    SERVER_PID=""
  fi
}

# build_fixture <dir> <archive-variant>: lays out release + cosign trees.
# Variants: ok | tampered-archive | missing-bundle.
build_fixture() {
  local dir="$1" variant="$2"
  mkdir -p "$dir/release" "$dir/cosign/fixture"
  printf '#!/bin/sh\necho magelift-harness-9.9.9\n' >"$dir/magelift"
  chmod +x "$dir/magelift"
  (cd "$dir" && tar -czf "release/$ARCHIVE" magelift)
  (cd "$dir/release" && sha256sum "$ARCHIVE" | sed 's/  /  /' >checksums.txt)
  if [ "$variant" = "tampered-archive" ]; then
    printf 'tamper' >>"$dir/release/$ARCHIVE"
  fi
  if [ "$variant" != "missing-bundle" ]; then
    printf '{"fixture": true}\n' >"$dir/release/checksums.txt.sigstore.json"
  fi
  write_fixture_cosign "$dir/cosign/fixture/$COSIGN_FILE"
  sha256sum "$dir/cosign/fixture/$COSIGN_FILE" | cut -d' ' -f1
}

run_installer() {
  local root="$1" pin="$2" dest="$3" cosign_exit="$4"
  stop_server
  start_server "$root"
  COSIGN_FIXTURE_MARKER="$WORK/marker.txt" \
    COSIGN_FIXTURE_EXIT="$cosign_exit" \
    MAGELIFT_VERSION="$TAG" \
    MAGELIFT_INSTALL_DIR="$dest" \
    MAGELIFT_RELEASE_BASE="http://127.0.0.1:$PORT/release" \
    MAGELIFT_COSIGN_BASE="http://127.0.0.1:$PORT/cosign" \
    MAGELIFT_COSIGN_VERSION="fixture" \
    MAGELIFT_COSIGN_PIN="$pin" \
    sh "$INSTALL"
}

pass=0
fail=0
check() {
  local name="$1"; shift
  if "$@" >/dev/null 2>&1; then
    pass=$((pass + 1)); printf 'ok: %s\n' "$name"
  else
    fail=$((fail + 1)); printf 'FAIL: %s\n' "$name"
  fi
}

# 1. Success: installs, runs, and passes the pinned identity.
rm -rf "$WORK/success" "$WORK/dest-success"
pin="$(build_fixture "$WORK/success" ok)"
output="$(run_installer "$WORK/success" "$pin" "$WORK/dest-success" 0 2>&1)"
check "success installs" test -x "$WORK/dest-success/magelift"
check "installed binary runs" sh -c "$WORK/dest-success/magelift | grep -q magelift-harness"
check "identity pinned to release workflow" grep -q "identity=https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/$TAG" "$WORK/marker.txt"
check "issuer pinned to github oidc" grep -q "issuer=https://token.actions.githubusercontent.com" "$WORK/marker.txt"
check "success announces verification" sh -c "printf '%s' '$output' | grep -q 'Sigstore bundle verified'"

# 2. Tampered archive: checksum failure, nothing installed.
rm -rf "$WORK/tampered" "$WORK/dest-tampered"
pin="$(build_fixture "$WORK/tampered" tampered-archive)"
output="$(run_installer "$WORK/tampered" "$pin" "$WORK/dest-tampered" 0 2>&1)" && status=0 || status=$?
check "tampered archive fails" test "$status" -ne 0
check "tampered archive names checksum" sh -c "printf '%s' '$output' | grep -q 'checksum verification failed'"
check "tampered archive installs nothing" test ! -e "$WORK/dest-tampered/magelift"

# 3. Missing bundle: required failure.
rm -rf "$WORK/nobundle" "$WORK/dest-nobundle"
pin="$(build_fixture "$WORK/nobundle" missing-bundle)"
output="$(run_installer "$WORK/nobundle" "$pin" "$WORK/dest-nobundle" 0 2>&1)" && status=0 || status=$?
check "missing bundle fails" test "$status" -ne 0
check "missing bundle names requirement" sh -c "printf '%s' '$output' | grep -q 'bundle is required'"

# 4. Failed verification: cosign exit 1 aborts.
rm -rf "$WORK/badverify" "$WORK/dest-badverify"
pin="$(build_fixture "$WORK/badverify" ok)"
output="$(run_installer "$WORK/badverify" "$pin" "$WORK/dest-badverify" 1 2>&1)" && status=0 || status=$?
check "failed verification aborts" test "$status" -ne 0
check "failed verification named" sh -c "printf '%s' '$output' | grep -q 'sigstore verification failed'"

# 5. Tampered bootstrap: wrong pin aborts before any release trust.
rm -rf "$WORK/badpin" "$WORK/dest-badpin"
pin="$(build_fixture "$WORK/badpin" ok)"
output="$(run_installer "$WORK/badpin" "0000000000000000000000000000000000000000000000000000000000000000" "$WORK/dest-badpin" 0 2>&1)" && status=0 || status=$?
check "tampered bootstrap fails" test "$status" -ne 0
check "tampered bootstrap named" sh -c "printf '%s' '$output' | grep -q 'bootstrap checksum mismatch'"

stop_server
printf 'installer harness: %d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
