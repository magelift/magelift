#!/usr/bin/env bash
set -Eeuo pipefail

: "${MAGELIFT_BIN:?set MAGELIFT_BIN to a built magelift executable}"
: "${MAGELIFT_CONFIG:?set MAGELIFT_CONFIG to an integration configuration file}"
: "${MAGELIFT_AWS_INTEGRATION_PROFILE:?set MAGELIFT_AWS_INTEGRATION_PROFILE to preview, standard, or high-availability}"
: "${MAGELIFT_AWS_INTEGRATION_DIGEST:?set MAGELIFT_AWS_INTEGRATION_DIGEST to a signed immutable image reference}"
: "${MAGELIFT_AWS_CERTIFICATE_IDENTITY:?set MAGELIFT_AWS_CERTIFICATE_IDENTITY to the expected Sigstore identity}"
: "${MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER:?set MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER to the expected Sigstore issuer}"

profile="$MAGELIFT_AWS_INTEGRATION_PROFILE"
case "$profile" in
preview|standard|high-availability) ;;
*)
	printf 'unsupported integration profile: %s\n' "$profile" >&2
	exit 2
;;
esac

config=("$MAGELIFT_BIN" --config "$MAGELIFT_CONFIG" --env "$profile" --no-interaction --output json)
run() {
	printf '+ magelift %s\n' "$*" >&2
	"${config[@]}" "$@"
}

created=0
cleanup() {
	if [[ "$created" == 1 && "${MAGELIFT_AWS_INTEGRATION_KEEP:-false}" != true ]]; then
		"${config[@]}" destroy --yes >/dev/null || printf 'integration cleanup failed for %s\n' "$profile" >&2
	fi
}
trap cleanup EXIT

run config validate
run doctor
run preview
run promote \
	--digest "$MAGELIFT_AWS_INTEGRATION_DIGEST" \
	--certificate-identity "$MAGELIFT_AWS_CERTIFICATE_IDENTITY" \
	--certificate-oidc-issuer "$MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER"
created=1
run deploy --digest "$MAGELIFT_AWS_INTEGRATION_DIGEST" --yes
run outputs
run health --mode runtime
