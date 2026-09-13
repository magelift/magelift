#!/usr/bin/env bash
# Offline contract test for provider-neutral Cosign acceptance helpers.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../../scripts/acceptance/lib-cosign.sh
source "$ROOT/scripts/acceptance/lib-cosign.sh"

FAIL=0

assert_eq() {
	local label="$1" got="$2" want="$3"
	if [[ "$got" != "$want" ]]; then
		printf 'FAIL %s: got=%q want=%q\n' "$label" "$got" "$want" >&2
		FAIL=1
	else
		printf 'ok %s\n' "$label"
	fi
}

unset MAGELIFT_CERTIFICATE_IDENTITY MAGELIFT_AWS_CERTIFICATE_IDENTITY MAGELIFT_GCP_CERTIFICATE_IDENTITY MAGELIFT_OVH_CERTIFICATE_IDENTITY MAGELIFT_SCALEWAY_CERTIFICATE_IDENTITY
unset MAGELIFT_CERTIFICATE_OIDC_ISSUER MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER MAGELIFT_GCP_CERTIFICATE_OIDC_ISSUER MAGELIFT_OVH_CERTIFICATE_OIDC_ISSUER MAGELIFT_SCALEWAY_CERTIFICATE_OIDC_ISSUER
unset MAGELIFT_COSIGN_IDENTITY_TOKEN_FILE MAGELIFT_COSIGN_IDENTITY_TOKEN MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV

assert_eq 'empty identity' "$(acceptance_certificate_identity)" ''
assert_eq 'default issuer' "$(acceptance_certificate_oidc_issuer)" 'https://token.actions.githubusercontent.com'

assert_eq 'aws fallback' "$(acceptance_certificate_identity 'aws@example.invalid')" 'aws@example.invalid'
assert_eq 'unrelated leftover ignored' "$(acceptance_certificate_identity 'gcp@example.invalid')" 'gcp@example.invalid'

MAGELIFT_CERTIFICATE_IDENTITY='generic@example.invalid'
assert_eq 'generic wins over fallback' "$(acceptance_certificate_identity 'aws@example.invalid')" 'generic@example.invalid'

assert_eq 'issuer fallback' "$(acceptance_certificate_oidc_issuer 'https://accounts.google.com')" 'https://accounts.google.com'
MAGELIFT_CERTIFICATE_OIDC_ISSUER='https://issuer.example.invalid'
assert_eq 'generic issuer wins' "$(acceptance_certificate_oidc_issuer 'https://accounts.google.com')" 'https://issuer.example.invalid'

if acceptance_identity_token_configured; then
	printf 'FAIL token configured with empty env\n' >&2
	FAIL=1
else
	printf 'ok token not configured\n'
fi
MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV='["gcloud","auth","print-identity-token"]'
if ! acceptance_identity_token_configured; then
	printf 'FAIL argv should mark token configured\n' >&2
	FAIL=1
else
	printf 'ok argv configured\n'
fi

unset MAGELIFT_CERTIFICATE_IDENTITY
if acceptance_require_certificate_identity 2>/dev/null; then
	printf 'FAIL require should fail without identity\n' >&2
	FAIL=1
else
	printf 'ok require fails closed\n'
fi

acceptance_require_certificate_identity 'ovh@example.invalid'
assert_eq 'require exports identity' "$MAGELIFT_CERTIFICATE_IDENTITY" 'ovh@example.invalid'
assert_eq 'require exports issuer' "$MAGELIFT_CERTIFICATE_OIDC_ISSUER" 'https://issuer.example.invalid'

if [[ "$FAIL" -ne 0 ]]; then
	exit 1
fi
printf 'lib_cosign_test OK\n'
