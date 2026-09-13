#!/usr/bin/env bash
# Offline safety check: Pub/Sub dry-run must stop before any provider command.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/gcp-pubsub-acceptance-local.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/bin"
for command_name in gcloud go; do
	cat >"$TMP/bin/$command_name" <<EOF
#!/usr/bin/env bash
printf '%s\n' '$command_name' >> '$TMP/provider-calls'
exit 99
EOF
	chmod +x "$TMP/bin/$command_name"
done

export PATH="$TMP/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
export GCP_PROJECT=example-gcp-project
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
output="$(bash "$SCRIPT" 2>&1)"

if [[ "$output" != *"gcp Pub/Sub acceptance dry-run ok; no provider mutation invoked"* ]]; then
	printf 'Pub/Sub dry-run success marker missing\n%s\n' "$output" >&2
	exit 1
fi
if [[ -s "$TMP/provider-calls" ]]; then
	printf 'Pub/Sub dry-run invoked provider/build command(s):\n' >&2
	cat "$TMP/provider-calls" >&2
	exit 1
fi

grep -Fq 'MAGELIFT_GCP_PUBSUB_RECOVERY_DESTINATION' "$SCRIPT" || {
	printf 'Pub/Sub harness must honor MAGELIFT_GCP_PUBSUB_RECOVERY_DESTINATION\n' >&2
	exit 1
}
grep -Fq -- '--destination="${destination}"' "$SCRIPT" || {
	printf 'Pub/Sub harness must pass same-region or isolated destination to the acceptance command\n' >&2
	exit 1
}

printf 'gcp_pubsub_harness_shape_test OK\n'
