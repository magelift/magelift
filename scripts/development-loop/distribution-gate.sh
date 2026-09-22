#!/usr/bin/env bash
# Distribution gate: stages canary CLI assets and runs website/public/install.sh
# with MAGELIFT_RELEASE_BASE pointed at the staging endpoint (never the public
# GitHub download URL), then checks the provider lock through providerhost.
# Operator actions: pushing and deleting v0.0.0-canary.<sha> are not performed
# by this script.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=development-loop/lib-canary-identity.sh
source "$ROOT/scripts/development-loop/lib-canary-identity.sh"
# shellcheck source=development-loop/lib-gates.sh
source "$ROOT/scripts/development-loop/lib-gates.sh"
# shellcheck source=development-loop/lib-distribution-fixture.sh
source "$ROOT/scripts/development-loop/lib-distribution-fixture.sh"

INSTALL="$ROOT/website/public/install.sh"
SHA=""
GATE_FILE="${MAGELIFT_DEVLOOP_GATE_FILE:-${MAGELIFT_DEVLOOP_GATES:-.magelift/development-loop/gates.jsonl}}"
FIXTURE="${MAGELIFT_DEVLOOP_FIXTURE:-0}"
STAGED=""
WORK=""

usage() {
	sed -n '2,8p' "$0"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--sha)
		SHA="${2:?--sha needs a value}"
		shift 2
		;;
	--gate-file)
		GATE_FILE="${2:?--gate-file needs a value}"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		printf 'distribution-gate: unknown argument %s\n' "$1" >&2
		exit 2
		;;
	esac
done

if [[ -z "$SHA" ]]; then
	printf 'distribution-gate: --sha is required\n' >&2
	exit 2
fi

TAG="$(canary_ref_for_sha "$SHA")" || {
	devloop_append_gate "$GATE_FILE" distribution FAIL 'commit SHA must be 40 lowercase hexadecimal characters'
	exit 1
}

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
x86_64 | amd64) ARCH="amd64" ;;
arm64 | aarch64) ARCH="arm64" ;;
*)
	devloop_append_gate "$GATE_FILE" distribution FAIL "unsupported architecture ${ARCH}"
	exit 1
	;;
esac
VERSION="${TAG#v}"
ARCHIVE="magelift_${VERSION}_${OS}_${ARCH}.tar.gz"
PROVIDER_BINARY="$(devloop_provider_binary_name "$TAG" "$OS" "$ARCH")"
PROVIDER_BUNDLE="${PROVIDER_BINARY}.sigstore.json"
PROVIDER_LOCK="$(devloop_provider_lock_name "$OS" "$ARCH")"
IDENTITY="https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/${TAG}"

record_fail() {
	devloop_append_gate "$GATE_FILE" distribution FAIL "$1"
}

record_skip() {
	devloop_append_gate "$GATE_FILE" distribution SKIP "$1"
}

verify_provider_fixture() {
	if ! (cd "$ROOT" && go test ./internal/providerhost/ -run 'TestDownloadInstall(RefusesTamperedBinary|VerifiesAndCaches)' -count=1 >/dev/null); then
		record_fail 'provider verifier fixture tests failed'
		return 1
	fi
	return 0
}

verify_provider_staged() {
	local staged="$1" tag="$2"
	local lock_path="$staged/$PROVIDER_LOCK"
	local provider_bin="$staged/$PROVIDER_BINARY"
	local bundle_path="$staged/$PROVIDER_BUNDLE"
	local digest
	if [[ ! -f "$lock_path" || ! -f "$provider_bin" || ! -f "$bundle_path" ]]; then
		record_fail 'staged provider lock, binary, or bundle is missing'
		return 1
	fi
	digest="$(jq -r '.providers.gcp.digest // empty' "$lock_path")"
	if [[ ! "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
		record_fail 'provider lock digest is malformed'
		return 1
	fi
	local sum
	if command -v sha256sum >/dev/null 2>&1; then
		sum="$(sha256sum "$provider_bin" | cut -d' ' -f1)"
	else
		sum="$(shasum -a 256 "$provider_bin" | cut -d' ' -f1)"
	fi
	if [[ "$digest" != "sha256:${sum}" ]]; then
		record_fail 'provider binary checksum mismatch'
		return 1
	fi
	if ! cosign verify-blob \
		--bundle "$bundle_path" \
		--certificate-identity "$IDENTITY" \
		--certificate-oidc-issuer https://token.actions.githubusercontent.com \
		"$provider_bin" >/dev/null 2>&1; then
		record_fail 'provider sigstore verification failed'
		return 1
	fi
	return 0
}

run_installer() {
	local release_base="$1" install_dir="$2" cosign_pin="$3" cosign_exit="${4:-0}"
	local marker="${WORK}/cosign-marker.txt"
	COSIGN_FIXTURE_MARKER="$marker" \
		COSIGN_FIXTURE_EXIT="$cosign_exit" \
		MAGELIFT_VERSION="$TAG" \
		MAGELIFT_INSTALL_DIR="$install_dir" \
		MAGELIFT_RELEASE_BASE="$release_base" \
		MAGELIFT_COSIGN_BASE="http://127.0.0.1:${DEVLOOP_SERVER_PORT}/cosign" \
		MAGELIFT_COSIGN_VERSION="fixture" \
		MAGELIFT_COSIGN_PIN="$cosign_pin" \
		sh "$INSTALL"
}

run_fixture_gate() {
	WORK="$(mktemp -d "${TMPDIR:-/tmp}/magelift-devloop-distribution.XXXXXX")"
	trap 'devloop_stop_fixture_server; rm -rf "$WORK"' EXIT INT TERM

	local pin dest output status
	pin="$(devloop_build_install_fixture "$WORK/success" "$TAG" ok)"
	devloop_start_fixture_server "$WORK/success"
	dest="$WORK/dest-success"
	mkdir -p "$dest"
	if ! run_installer "http://127.0.0.1:${DEVLOOP_SERVER_PORT}/release" "$dest" "$pin" 0; then
		record_fail 'fixture installer did not pass'
		return 1
	fi
	if [[ ! -x "$dest/magelift" ]]; then
		record_fail 'fixture installer did not install magelift'
		return 1
	fi

	devloop_stop_fixture_server
	pin="$(devloop_build_install_fixture "$WORK/tampered" "$TAG" tampered-archive)"
	devloop_start_fixture_server "$WORK/tampered"
	dest="$WORK/dest-tampered"
	mkdir -p "$dest"
	output="$(run_installer "http://127.0.0.1:${DEVLOOP_SERVER_PORT}/release" "$dest" "$pin" 0 2>&1)" && status=0 || status=$?
	if [[ "$status" -eq 0 ]]; then
		record_fail 'tampered archive was accepted'
		return 1
	fi
	if ! printf '%s' "$output" | grep -q 'checksum verification failed'; then
		record_fail 'tampered archive did not fail closed on checksum'
		return 1
	fi

	if ! verify_provider_fixture; then
		return 1
	fi
	devloop_append_gate "$GATE_FILE" distribution PASS ''
	return 0
}

stage_release_assets() {
	local assets_dir="$1"
	local gh_bin="${MAGELIFT_GH:-gh}"
	if ! "$gh_bin" release view "$TAG" --json isDraft >/dev/null 2>&1; then
		return 1
	fi
	mkdir -p "$assets_dir"
	"$gh_bin" release download "$TAG" \
		--pattern "$ARCHIVE" \
		--pattern checksums.txt \
		--pattern checksums.txt.sigstore.json \
		--pattern "$PROVIDER_LOCK" \
		--pattern "$PROVIDER_BINARY" \
		--pattern "$PROVIDER_BUNDLE" \
		--dir "$assets_dir"
	return 0
}

run_staged_gate() {
	mkdir -p "$ROOT/.magelift/tmp"
	WORK="$(mktemp -d "$ROOT/.magelift/tmp/magelift-devloop-distribution.XXXXXX")"
	trap 'devloop_stop_fixture_server; rm -rf "$WORK"' EXIT INT TERM
	STAGED="$WORK/staged"
	if ! stage_release_assets "$STAGED"; then
		record_skip "draft release assets for ${TAG} are unavailable"
		return 0
	fi
	mkdir -p "$WORK/release-root/release"
	cp "$STAGED/$ARCHIVE" "$STAGED/checksums.txt" "$STAGED/checksums.txt.sigstore.json" "$WORK/release-root/release/"
	# The provider binary is hundreds of megabytes and is verified from the
	# download directory. The installer only fetches the CLI archive.
	devloop_start_fixture_server "$WORK/release-root"
	local dest="$WORK/dest"
	mkdir -p "$dest"
	if ! TMPDIR="$WORK" \
		MAGELIFT_VERSION="$TAG" \
		MAGELIFT_INSTALL_DIR="$dest" \
		MAGELIFT_RELEASE_BASE="http://127.0.0.1:${DEVLOOP_SERVER_PORT}/release" \
		sh "$INSTALL"; then
		record_fail 'installer failed against staged draft assets'
		return 1
	fi
	if ! verify_provider_staged "$STAGED" "$TAG"; then
		return 1
	fi
	devloop_append_gate "$GATE_FILE" distribution PASS ''
	return 0
}

if [[ "$FIXTURE" == 1 ]]; then
	run_fixture_gate
else
	run_staged_gate
fi
