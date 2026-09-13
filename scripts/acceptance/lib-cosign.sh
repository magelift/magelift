#!/usr/bin/env bash
# Provider-neutral Cosign / Sigstore identity for sign and promote.
# Signing identity is an OIDC subject (GitHub Actions, Google SA, etc.), not
# the Magento cloud target. AWS, GCP, OVH, and Scaleway share these helpers.
# shellcheck shell=bash

# Optional $1 is one provider-specific identity fallback
# (MAGELIFT_AWS_CERTIFICATE_IDENTITY, MAGELIFT_GCP_CERTIFICATE_IDENTITY, …).
acceptance_certificate_identity() {
	printf '%s' "${MAGELIFT_CERTIFICATE_IDENTITY:-${1:-}}"
}

# Optional $1 is one provider-specific issuer fallback.
acceptance_certificate_oidc_issuer() {
	printf '%s' "${MAGELIFT_CERTIFICATE_OIDC_ISSUER:-${1:-https://token.actions.githubusercontent.com}}"
}

# Optional $1 identity fallback, $2 issuer fallback.
acceptance_require_certificate_identity() {
	local identity issuer
	identity="$(acceptance_certificate_identity "${1:-}")"
	issuer="$(acceptance_certificate_oidc_issuer "${2:-}")"
	if [[ -z "$identity" ]]; then
		printf 'set MAGELIFT_CERTIFICATE_IDENTITY (or MAGELIFT_<PROVIDER>_CERTIFICATE_IDENTITY) to the expected Sigstore identity\n' >&2
		return 2
	fi
	if [[ -z "$issuer" ]]; then
		printf 'set MAGELIFT_CERTIFICATE_OIDC_ISSUER to the expected Sigstore OIDC issuer\n' >&2
		return 2
	fi
	MAGELIFT_CERTIFICATE_IDENTITY="$identity"
	MAGELIFT_CERTIFICATE_OIDC_ISSUER="$issuer"
	export MAGELIFT_CERTIFICATE_IDENTITY MAGELIFT_CERTIFICATE_OIDC_ISSUER
}

acceptance_identity_token_configured() {
	[[ -n "${MAGELIFT_COSIGN_IDENTITY_TOKEN_FILE:-}" || -n "${MAGELIFT_COSIGN_IDENTITY_TOKEN:-}" || -n "${MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV:-}" ]]
}

# Requires a harness `run` function that invokes magelift.
# Optional $2/$3 are identity and issuer fallbacks for require.
acceptance_maybe_sign_digest() {
	local digest="${1:?digest required}"
	if ! acceptance_identity_token_configured; then
		return 0
	fi
	printf '+ magelift sign (non-interactive Cosign identity token)\n' >&2
	run sign --digest "$digest"
}

# Requires a harness `run` function that invokes magelift.
# Optional $2 identity fallback, $3 issuer fallback.
acceptance_promote_digest() {
	local digest="${1:?digest required}"
	acceptance_require_certificate_identity "${2:-}" "${3:-}"
	run promote \
		--digest "$digest" \
		--certificate-identity "$MAGELIFT_CERTIFICATE_IDENTITY" \
		--certificate-oidc-issuer "$MAGELIFT_CERTIFICATE_OIDC_ISSUER"
}
