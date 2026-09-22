#!/usr/bin/env bash
# Local fixture layout for the distribution gate and installer harness.
# Stages release assets on a loopback HTTP server; never uses the public
# GitHub download URL.
# shellcheck shell=bash

# GoReleaser publishes magelift-provider-gcp_<version>_<os>_<arch>
# and genproviders writes magelift.providers.lock.<os>_<arch>.
devloop_provider_binary_name() {
	local tag="$1" os="$2" arch="$3"
	printf 'magelift-provider-gcp_%s_%s_%s\n' "${tag#v}" "$os" "$arch"
}

devloop_provider_lock_name() {
	local os="$1" arch="$2"
	printf 'magelift.providers.lock.%s_%s\n' "$os" "$arch"
}

devloop_free_port() {
	python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'
}

devloop_write_fixture_cosign() {
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

# devloop_build_install_fixture DIR TAG VARIANT
# Variants: ok | tampered-archive | missing-bundle.
devloop_build_install_fixture() {
	local dir="$1" tag="$2" variant="$3"
	local version="${tag#v}"
	local os arch archive cosign_file
	os="$(uname -s | tr '[:upper:]' '[:lower:]')"
	arch="$(uname -m)"
	case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	arm64 | aarch64) arch="arm64" ;;
	*) printf 'fixture: unsupported architecture %s\n' "$arch" >&2; return 1 ;;
	esac
	archive="magelift_${version}_${os}_${arch}.tar.gz"
	cosign_file="cosign-$os-$arch"
	mkdir -p "$dir/release" "$dir/cosign/fixture"
	printf '#!/bin/sh\necho magelift-devloop-fixture\n' >"$dir/magelift"
	chmod +x "$dir/magelift"
	(cd "$dir" && tar -czf "release/$archive" magelift)
	(cd "$dir/release" && sha256sum "$archive" | sed 's/  /  /' >checksums.txt)
	if [[ "$variant" == tampered-archive ]]; then
		printf 'tamper' >>"$dir/release/$archive"
	fi
	if [[ "$variant" != missing-bundle ]]; then
		printf '{"fixture": true}\n' >"$dir/release/checksums.txt.sigstore.json"
	fi
	devloop_write_fixture_cosign "$dir/cosign/fixture/$cosign_file"
	sha256sum "$dir/cosign/fixture/$cosign_file" | cut -d' ' -f1
}

devloop_start_fixture_server() {
	local root="$1"
	DEVLOOP_SERVER_PORT="$(devloop_free_port)"
	python3 -m http.server "$DEVLOOP_SERVER_PORT" --directory "$root" --bind 127.0.0.1 >/dev/null 2>&1 &
	DEVLOOP_SERVER_PID=$!
	local attempt
	for attempt in $(seq 1 50); do
		if curl -fsSL "http://127.0.0.1:$DEVLOOP_SERVER_PORT/" >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.1
	done
	printf 'fixture server did not start\n' >&2
	return 1
}

devloop_stop_fixture_server() {
	if [[ -n "${DEVLOOP_SERVER_PID:-}" ]]; then
		kill "$DEVLOOP_SERVER_PID" 2>/dev/null || true
		wait "$DEVLOOP_SERVER_PID" 2>/dev/null || true
		DEVLOOP_SERVER_PID=""
	fi
}
