#!/usr/bin/env bash
# Local, credit-efficient AWS acceptance: preview by default, destroy on EXIT.
# Requires real AWS credentials (aws-cli / SDK default chain) and Pulumi via MageLift.
set -Eeuo pipefail

: "${MAGELIFT_BIN:?set MAGELIFT_BIN to a built magelift executable}"
: "${MAGELIFT_CONFIG:?set MAGELIFT_CONFIG to an acceptance configuration file}"
: "${MAGELIFT_AWS_ACCEPTANCE_DIGEST:?set MAGELIFT_AWS_ACCEPTANCE_DIGEST to a signed immutable image reference}"
: "${MAGELIFT_AWS_CERTIFICATE_IDENTITY:?set MAGELIFT_AWS_CERTIFICATE_IDENTITY to the expected Sigstore identity}"
: "${MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER:=https://token.actions.githubusercontent.com}"

profile="${MAGELIFT_AWS_ACCEPTANCE_PROFILE:-preview}"
case "$profile" in
preview|standard|high-availability) ;;
*)
	printf 'unsupported acceptance profile: %s (use preview unless credits allow more)\n' "$profile" >&2
	exit 2
;;
esac

if [[ "$profile" != preview && "${MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY:-}" != true ]]; then
	printf 'refusing %s without MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY=true\n' "$profile" >&2
	exit 2
fi

config=("$MAGELIFT_BIN" --config "$MAGELIFT_CONFIG" --env "$profile" --no-interaction --output json)
run() {
	printf '+ magelift %s\n' "$*" >&2
	"${config[@]}" "$@"
}

created=0
cleanup() {
	if [[ "$created" == 1 && "${MAGELIFT_AWS_ACCEPTANCE_KEEP:-false}" != true ]]; then
		printf '+ magelift destroy --yes (EXIT trap)\n' >&2
		"${config[@]}" destroy --yes || printf 'acceptance cleanup failed for %s; inspect with aws-cli and destroy manually\n' "$profile" >&2
	fi
}
trap cleanup EXIT

started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf 'aws acceptance start profile=%s at=%s\n' "$profile" "$started_at" >&2

run config validate
run doctor
run login
run preview
run promote \
	--digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" \
	--certificate-identity "$MAGELIFT_AWS_CERTIFICATE_IDENTITY" \
	--certificate-oidc-issuer "$MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER"
created=1
run deploy --digest "$MAGELIFT_AWS_ACCEPTANCE_DIGEST" --yes
run outputs
run health --mode runtime

printf 'aws acceptance ok profile=%s; destroy runs on EXIT unless MAGELIFT_AWS_ACCEPTANCE_KEEP=true\n' "$profile" >&2
