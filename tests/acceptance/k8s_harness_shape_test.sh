#!/usr/bin/env bash
# Offline shape check for the OVH and Scaleway acceptance adapters.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

for provider in ovh scaleway; do
	script="$ROOT/scripts/${provider}-acceptance-local.sh"
	case "$provider" in
	ovh)
		for required in 'ovhcloud' 'managed-kubernetes' 'MAGELIFT_OVH_ACCEPTANCE_DIGEST'; do
			grep -q "$required" "$ROOT/scripts/k8s-acceptance-local.sh" || { printf 'OVH adapter missing %s\n' "$required" >&2; exit 1; }
		done
		;;
	scaleway)
		for required in 'scw ' 'k8s cluster list' 'MAGELIFT_SCALEWAY_ACCEPTANCE_DIGEST'; do
			grep -q "$required" "$ROOT/scripts/k8s-acceptance-local.sh" || { printf 'Scaleway adapter missing %s\n' "$required" >&2; exit 1; }
		done
		for required in 'k8s cluster list "project-id=' 'rdb instance list "project-id='; do
			grep -q "$required" "$ROOT/scripts/k8s-acceptance-local.sh" || { printf 'Scaleway cleanup missing %s\n' "$required" >&2; exit 1; }
		done
		for required in 'redis cluster list' 'vpc private-network list' 'lb lb list'; do
			grep -q "$required" "$ROOT/scripts/k8s-acceptance-local.sh" || { printf 'Scaleway cleanup missing %s\n' "$required" >&2; exit 1; }
		done
		;;
	esac

for required in 'MAGELIFT_K8S_ACCEPTANCE' 'MAGELIFT_K8S_ACCEPTANCE_RUNTIME_HEALTH' 'wait_for_clean' 'append_shared_cleanup' 'update_cell_cleanup_state' 'MAGELIFT_ACCEPTANCE_CELL_DURATION_SECONDS' 'run destroy --yes --skip-lock' 'run_runtime_health' 'runtime health not exercised' 'docker buildx imagetools inspect' 'digest_is_placeholder' 'PULUMI_BACKEND_URL="file://' 'mkdir -p "$LIVE_WORKDIR/pulumi"' 'lib-lifecycle.sh' 'lib-cosign.sh' 'acceptance_promote_digest' 'acceptance_prepare_lifecycle' 'expiresAt = strenv(ACCEPTANCE_EXPIRES_AT)' 'cells-${PROVIDER}-${PROFILE}.txt'; do
		grep -q "$required" "$ROOT/scripts/k8s-acceptance-local.sh" || { printf '%s adapter missing %s\n' "$provider" "$required" >&2; exit 1; }
	done

	checkpoint="$TMP/${provider}-checkpoint.json"
	evidence="$TMP/${provider}-evidence.md"
	shared="$TMP/${provider}-evidence.jsonl"
	log="$TMP/${provider}.log"
	set +e
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	MAGELIFT_ACCEPTANCE_CHECKPOINT="$checkpoint" \
	MAGELIFT_ACCEPTANCE_EVIDENCE="$evidence" \
	MAGELIFT_ACCEPTANCE_SHARED_EVIDENCE="$shared" \
	MAGELIFT_ACCEPTANCE_CELL_CATALOG="$ROOT/scripts/acceptance/cells-${provider}-preview.txt" \
	bash "$script" >"$log" 2>&1
	rc=$?
	set -e
	if [[ "$rc" -ne 0 ]]; then
		cat "$log" >&2
		exit 1
	fi
	if grep -Eq 'magelift (deploy|destroy|preview)|ovhcloud .*(create|delete)|scw .*(create|delete)' "$log"; then
		printf '%s dry-run contains a mutate command\n' "$provider" >&2
		cat "$log" >&2
		exit 1
	fi
	resume_log="$TMP/${provider}-resume.log"
	set +e
	MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
	MAGELIFT_ACCEPTANCE_CHECKPOINT="$checkpoint" \
	MAGELIFT_ACCEPTANCE_EVIDENCE="$evidence" \
	MAGELIFT_ACCEPTANCE_SHARED_EVIDENCE="$shared" \
	MAGELIFT_ACCEPTANCE_CELL_CATALOG="$ROOT/scripts/acceptance/cells-${provider}-preview.txt" \
		bash "$script" >"$resume_log" 2>&1
	resume_rc=$?
	set -e
	if [[ "$resume_rc" -ne 0 ]]; then
		printf '%s resume dry-run exited %s\n' "$provider" "$resume_rc" >&2
		cat "$resume_log" >&2
		exit 1
	fi
	first_cell=$(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$ROOT/scripts/acceptance/cells-${provider}-preview.txt" | head -n 1)
	if ! grep -q "acceptance skip cell=${first_cell} (checkpoint)" "$resume_log"; then
		printf '%s resume must skip the first completed cell\n' "$provider" >&2
		cat "$resume_log" >&2
		exit 1
	fi
done

printf 'k8s_harness_shape_test OK\n'
