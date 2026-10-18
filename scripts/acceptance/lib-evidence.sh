#!/usr/bin/env bash
# Evidence append helpers for the readable matrix and the RC1 JSONL contract.
# Columns: cell | result | duration | provider | account | date
# Do not write secrets into either evidence format. The JSONL format carries
# the immutable image digest and the Composer credential scheme, never a
# credential value.
# Certification (GCP 07-07 / AWS certify) requires append_row provenance only; # hand-typed or hand-edited matrix rows are not valid SC1-SC5 evidence (T-07-12).
# GCP harness defaults ACCEPTANCE_EVIDENCE to
# .magelift/gcp-matrix/matrix-results.md (six-column table).
# shellcheck shell=bash

: "${ACCEPTANCE_EVIDENCE:=${MAGELIFT_ACCEPTANCE_EVIDENCE:-.magelift/matrix-results.md}}"
: "${ACCEPTANCE_SHARED_EVIDENCE:=${MAGELIFT_ACCEPTANCE_SHARED_EVIDENCE:-.magelift/acceptance-evidence.jsonl}}"
: "${ACCEPTANCE_EVIDENCE_RUN_ID:=${MAGELIFT_ACCEPTANCE_RUN_ID:-run-$(date -u +%Y%m%dt%H%M%Sz)-$$}}"

acceptance_evidence_ensure() {
	local dir
	dir=$(dirname "$ACCEPTANCE_EVIDENCE")
	mkdir -p "$dir"
	if [[ ! -f "$ACCEPTANCE_EVIDENCE" ]]; then
		cat >"$ACCEPTANCE_EVIDENCE" <<'EOF'
| cell | result | duration | provider | account | date |
|------|--------|----------|----------|---------|------|
EOF
	fi
}

# append_row CELL RESULT DURATION PROVIDER ACCOUNT DATE
append_row() {
	local cell="${1:?cell required}"
	local result="${2:?result required}"
	local duration="${3:?duration required}"
	local provider="${4:?provider required}"
	local account="${5:?account required}"
	local date="${6:?date required}"
	acceptance_evidence_ensure
	printf '| %s | %s | %s | %s | %s | %s |\n' \
		"$cell" "$result" "$duration" "$provider" "$account" "$date" \
		>>"$ACCEPTANCE_EVIDENCE"
}

acceptance_shared_evidence_ensure() {
	local dir
	dir=$(dirname "$ACCEPTANCE_SHARED_EVIDENCE")
	mkdir -p "$dir"
	if [[ ! -f "$ACCEPTANCE_SHARED_EVIDENCE" ]]; then
		touch "$ACCEPTANCE_SHARED_EVIDENCE"
	fi
}

acceptance_shared_dimension() {
	local provider="$1"
	local cell="$2"
	local key value
	case "$provider" in
	aws) printf '%s' "ecs-fargate" ;;
	gcp) printf '%s' "gke-autopilot" ;;
	ovh) printf '%s' "mks" ;;
	scaleway) printf '%s' "kapsule" ;;
	*) printf '%s' "unknown" ;;
	esac
}

acceptance_shared_compute_mode() {
	local runtime="${1:-${MAGELIFT_ACCEPTANCE_RUNTIME:-}}"
	if [[ -n "${MAGELIFT_ACCEPTANCE_COMPUTE_MODE:-}" ]]; then
		printf '%s' "$MAGELIFT_ACCEPTANCE_COMPUTE_MODE"
		return 0
	fi
	case "$runtime" in
	ecs-fargate) printf '%s' "fargate" ;;
	gke-autopilot) printf '%s' "autopilot" ;;
	gke-standard) printf '%s' "standard" ;;
	kapsule|mks) printf '%s' "managed-node-pools" ;;
	*) printf '%s' "unknown" ;;
	esac
}

acceptance_shared_kubernetes_mode() {
	local runtime="${1:-${MAGELIFT_ACCEPTANCE_RUNTIME:-}}"
	if [[ -n "${MAGELIFT_ACCEPTANCE_KUBERNETES_MODE:-}" ]]; then
		printf '%s' "$MAGELIFT_ACCEPTANCE_KUBERNETES_MODE"
		return 0
	fi
	case "$runtime" in
	gke-autopilot) printf '%s' "autopilot" ;;
	gke-standard) printf '%s' "standard" ;;
	kapsule) printf '%s' "multi-zone" ;;
	mks) printf '%s' "multi-zone" ;;
	ecs-fargate) printf '%s' "none" ;;
	*) printf '%s' "unknown" ;;
	esac
}

acceptance_shared_cell_id() {
	local provider="$1" cell="$2" runtime compute_mode kubernetes_mode release edition preset database search queue cache web_cache edge scenario
	runtime="${MAGELIFT_ACCEPTANCE_RUNTIME:-$(acceptance_shared_dimension "$provider" "$cell")}"
	compute_mode="${MAGELIFT_ACCEPTANCE_COMPUTE_MODE:-$(acceptance_shared_compute_mode "$runtime")}"
	kubernetes_mode="${MAGELIFT_ACCEPTANCE_KUBERNETES_MODE:-$(acceptance_shared_kubernetes_mode "$runtime")}"
	release="${MAGELIFT_ACCEPTANCE_RELEASE:-2.4.9}"
	edition="${MAGELIFT_ACCEPTANCE_EDITION:-open-source}"
	preset="${MAGELIFT_ACCEPTANCE_PRESET:-preview}"
	database="${MAGELIFT_ACCEPTANCE_DATABASE:-}"
	search="${MAGELIFT_ACCEPTANCE_SEARCH:-disabled}"
	queue="${MAGELIFT_ACCEPTANCE_QUEUE:-database}"
	cache="${MAGELIFT_ACCEPTANCE_CACHE:-}"
	web_cache="${MAGELIFT_ACCEPTANCE_WEB_CACHE:-}"
	edge="${MAGELIFT_ACCEPTANCE_EDGE:-none}"
	scenario="${MAGELIFT_ACCEPTANCE_SCENARIO:-architecture}"
	if [[ -z "$database" ]]; then
		case "$provider" in aws) database="rds-mysql" ;; gcp) database="cloud-sql-mysql" ;; *) database="mysql" ;; esac
	fi
	if [[ -z "$cache" ]]; then
		case "$provider" in scaleway) cache="redis" ;; *) cache="valkey" ;; esac
	fi
	if [[ -z "$web_cache" ]]; then
		case "$provider" in aws) web_cache="varnish" ;; *) web_cache="none" ;; esac
	fi
	if [[ "$cell" == *:* ]]; then
		key="${cell%%:*}"
		value="${cell#*:}"
		case "$key" in
		computeMode)
			compute_mode="$value"
			scenario="architecture"
			;;
		queueMode)
			case "$value" in
			db) queue="database" ;;
			ecs-rabbitmq|rabbitmq) queue="rabbitmq" ;;
			ecs-artemis|artemis) queue="artemis" ;;
			amazon-mq) queue="amazon-mq" ;;
			*) queue="$value" ;;
			esac
			;;
		database|search|queue|cache|webCache|edge)
			case "$key" in
			database) database="$value" ;;
			search) search="$value" ;;
			queue) queue="$value" ;;
			cache) cache="$value" ;;
			webCache) web_cache="$value" ;;
			edge) edge="$value" ;;
			esac
			;;
		*) scenario="$cell" ;;
		esac
	fi
	printf 'rc1/%s/%s/%s/%s/%s/%s/%s/%s/%s/%s/%s/%s/%s/%s' \
		"$provider" "$runtime" "$compute_mode" "$kubernetes_mode" "$release" "$edition" "$preset" "$database" "$search" "$queue" "$cache" "$web_cache" "$edge" "$scenario"
}

# append_shared_row CELL RESULT DURATION PROVIDER ACCOUNT DATE
# This writes an unsealed core evidence candidate. A dry-run PASS without an
# immutable artifact is recorded as SKIP in the machine-readable candidate;
# the readable fixture remains PASS because it exercises harness control flow,
# not a certification claim. Run `magelift certification seal` before verify.
append_shared_row() {
	local cell="${1:?cell required}"
	local result="${2:?result required}"
	local duration="${3:?duration required}"
	local provider="${4:?provider required}"
	local account="${5:?account required}"
	local date_s="${6:?date required}"
	local run_id="${MAGELIFT_ACCEPTANCE_RUN_ID:-$ACCEPTANCE_EVIDENCE_RUN_ID}"
	local generated_at="${MAGELIFT_ACCEPTANCE_GENERATED_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
	local runtime compute_mode kubernetes_mode release edition preset database search queue cache web_cache edge scenario image_digest php_version php_extensions composer_version composer_configured composer_scheme required reason shared_status session_mode session_stack session_fingerprint artifact_digest fixture_id backup_set observability_setup edge_setup schema_fingerprint migration_fingerprint state_backend migration_owner
	runtime="${MAGELIFT_ACCEPTANCE_RUNTIME:-$(acceptance_shared_dimension "$provider" "$cell")}"
	compute_mode="${MAGELIFT_ACCEPTANCE_COMPUTE_MODE:-$(acceptance_shared_compute_mode "$runtime")}"
	kubernetes_mode="${MAGELIFT_ACCEPTANCE_KUBERNETES_MODE:-$(acceptance_shared_kubernetes_mode "$runtime")}"
	release="${MAGELIFT_ACCEPTANCE_RELEASE:-2.4.9}"
	edition="${MAGELIFT_ACCEPTANCE_EDITION:-open-source}"
	preset="${MAGELIFT_ACCEPTANCE_PRESET:-preview}"
	database="${MAGELIFT_ACCEPTANCE_DATABASE:-}"
	search="${MAGELIFT_ACCEPTANCE_SEARCH:-disabled}"
	queue="${MAGELIFT_ACCEPTANCE_QUEUE:-database}"
	cache="${MAGELIFT_ACCEPTANCE_CACHE:-}"
	web_cache="${MAGELIFT_ACCEPTANCE_WEB_CACHE:-}"
	edge="${MAGELIFT_ACCEPTANCE_EDGE:-none}"
	scenario="${MAGELIFT_ACCEPTANCE_SCENARIO:-architecture}"
	if [[ -z "$database" ]]; then
		case "$provider" in aws) database="rds-mysql" ;; gcp) database="cloud-sql-mysql" ;; *) database="mysql" ;; esac
	fi
	if [[ -z "$cache" ]]; then
		case "$provider" in scaleway) cache="redis" ;; *) cache="valkey" ;; esac
	fi
	if [[ -z "$web_cache" ]]; then
		case "$provider" in aws) web_cache="varnish" ;; *) web_cache="none" ;; esac
	fi
	image_digest="${MAGELIFT_ACCEPTANCE_DIGEST:-${MAGELIFT_AWS_ACCEPTANCE_DIGEST:-${MAGELIFT_GCP_ACCEPTANCE_DIGEST:-${MAGELIFT_OVH_ACCEPTANCE_DIGEST:-${MAGELIFT_SCALEWAY_ACCEPTANCE_DIGEST:-}}}}}"
	artifact_digest="$image_digest"
	fixture_id="${MAGELIFT_ACCEPTANCE_FIXTURE_ID:-}"
	backup_set="${MAGELIFT_ACCEPTANCE_BACKUP_SET:-}"
	observability_setup="${MAGELIFT_ACCEPTANCE_OBSERVABILITY_SETUP:-${MAGELIFT_ACCEPTANCE_OBSERVABILITY:-}}"
	edge_setup="${MAGELIFT_ACCEPTANCE_EDGE_SETUP:-$edge}"
	schema_fingerprint="${MAGELIFT_ACCEPTANCE_SCHEMA_FINGERPRINT:-}"
	migration_fingerprint="${MAGELIFT_ACCEPTANCE_MIGRATION_FINGERPRINT:-}"
	state_backend="${MAGELIFT_ACCEPTANCE_STATE_BACKEND:-}"
	php_version="${MAGELIFT_ACCEPTANCE_PHP_VERSION:-}"
	php_extensions="${MAGELIFT_ACCEPTANCE_PHP_EXTENSIONS:-}"
	composer_version="${MAGELIFT_ACCEPTANCE_COMPOSER_VERSION:-}"
	composer_configured="${MAGELIFT_ACCEPTANCE_COMPOSER_CREDENTIALS_CONFIGURED:-false}"
	composer_scheme="${MAGELIFT_ACCEPTANCE_COMPOSER_CREDENTIAL_SCHEME:-}"
	required="${MAGELIFT_ACCEPTANCE_REQUIRED_CELL:-false}"
	reason="${MAGELIFT_ACCEPTANCE_REASON:-}"
	# A caller must explicitly prove a retained session before a row is called
	# reused. The first row in a create-once flow is a baseline by default.
	session_mode="${MAGELIFT_ACCEPTANCE_SESSION_MODE:-baseline}"
	session_stack="${MAGELIFT_ACCEPTANCE_STACK_ID:-}"
	session_fingerprint="${MAGELIFT_ACCEPTANCE_CHECKPOINT_FINGERPRINT:-}"
	migration_owner="${MAGELIFT_ACCEPTANCE_MIGRATION_OWNER:-false}"
	case "$cell" in
	migrate:dump|deploy:candidate) migration_owner=true ;;
	esac
	if [[ "$cell" == *:* ]]; then
		local key value
		key="${cell%%:*}"
		value="${cell#*:}"
		case "$key" in
		queueMode)
			case "$value" in
			db) queue="database" ;;
			ecs-rabbitmq|rabbitmq) queue="rabbitmq" ;;
			ecs-artemis|artemis) queue="artemis" ;;
			amazon-mq) queue="amazon-mq" ;;
			*) queue="$value" ;;
			esac
			;;
		database) database="$value" ;;
		search) search="$value" ;;
		queue) queue="$value" ;;
		cache) cache="$value" ;;
		webCache) web_cache="$value" ;;
		edge) edge="$value" ;;
		*) scenario="$cell" ;;
		esac
	fi
	shared_status="$result"
	if [[ "$shared_status" == "PASS" && ( "${MAGELIFT_ACCEPTANCE_DRY_RUN:-0}" == "1" || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-}" == "true" ) ]]; then
		shared_status="SKIP"
		[[ -n "$reason" ]] || reason="harness dry-run did not provide an immutable artifact digest"
	fi
	if [[ "$shared_status" == "PASS" && -z "$image_digest" ]]; then
		shared_status="SKIP"
		[[ -n "$reason" ]] || reason="harness dry-run did not provide an immutable artifact digest"
	elif [[ "$shared_status" == "PASS" && ! "$image_digest" =~ ^[^@[:space:]]+@sha256:[0-9a-f]{64}$ ]]; then
		shared_status="SKIP"
		[[ -n "$reason" ]] || reason="harness provided a mutable or malformed artifact identity"
	fi
	if [[ "$shared_status" == "PASS" && ( -z "$php_version" || -z "$php_extensions" || -z "$composer_version" ) ]]; then
		shared_status="SKIP"
		[[ -n "$reason" ]] || reason="harness did not provide the resolved PHP extension and Composer contract"
	fi
	if [[ "$shared_status" == "PASS" && ( -z "$artifact_digest" || -z "$fixture_id" || -z "$backup_set" || -z "$observability_setup" || -z "$edge_setup" || -z "$schema_fingerprint" || -z "$migration_fingerprint" || -z "$state_backend" ) ]]; then
		shared_status="SKIP"
		[[ -n "$reason" ]] || reason="harness did not provide the complete artifact, fixture, backup, observability, edge, schema, migration, and state reuse boundary"
	fi
	if [[ "$shared_status" == "PASS" && ( "$compute_mode" == "unknown" || "$kubernetes_mode" == "unknown" || -z "$session_stack" || -z "$session_fingerprint" ) ]]; then
		shared_status="SKIP"
		[[ -n "$reason" ]] || reason="harness did not provide the exact compute, Kubernetes, or warm-session identity"
	fi
	if [[ "$shared_status" != "PASS" && -z "$reason" ]]; then
		reason="acceptance harness recorded $shared_status"
	fi
	local duration_seconds="${duration%s}"
	if [[ ! "$duration_seconds" =~ ^[0-9]+$ ]]; then
		duration_seconds=0
	fi
	acceptance_shared_evidence_ensure
	local record
	record=$(jq -cn \
		--arg runId "$run_id" --arg cellId "$(acceptance_shared_cell_id "$provider" "$cell")" \
		--arg status "$shared_status" --arg generatedAt "$generated_at" --arg accountRef "$account" \
		--arg provider "$provider" --arg runtime "$runtime" --arg computeMode "$compute_mode" --arg kubernetesMode "$kubernetes_mode" \
		--arg release "$release" --arg edition "$edition" \
		--arg preset "$preset" --arg database "$database" --arg search "$search" --arg queue "$queue" \
			--arg cache "$cache" --arg webCache "$web_cache" --arg edge "$edge" --arg scenario "$scenario" --arg imageDigest "$image_digest" \
			--arg phpVersion "$php_version" --arg phpExtensions "$php_extensions" --arg composerVersion "$composer_version" \
		--arg sessionMode "$session_mode" --arg sessionStack "$session_stack" --arg sessionFingerprint "$session_fingerprint" \
		--arg artifactDigest "$artifact_digest" --arg fixtureId "$fixture_id" --arg backupSet "$backup_set" \
		--arg observabilitySetup "$observability_setup" --arg edgeSetup "$edge_setup" --arg schemaFingerprint "$schema_fingerprint" \
		--arg migrationFingerprint "$migration_fingerprint" --arg stateBackend "$state_backend" \
		--arg composerScheme "$composer_scheme" --arg source "${MAGELIFT_ACCEPTANCE_SOURCE:-acceptance-harness}" \
		--arg reason "$reason" --argjson composerConfigured "$composer_configured" --argjson required "$required" --argjson migrationOwner "$migration_owner" \
		--argjson durationSeconds "$duration_seconds" \
		'{version:"v1", type:"cell", runId:$runId, cellId:$cellId, status:$status,
				dimensions:{provider:$provider, runtime:$runtime, computeMode:$computeMode, kubernetesMode:$kubernetesMode, release:$release, edition:$edition, preset:$preset,
			 database:$database, search:$search, queue:$queue, cache:$cache, webCache:$webCache, edge:$edge, scenario:$scenario},
				 artifact:{phpVersion:$phpVersion, phpExtensions:($phpExtensions | if . == "" then [] else split(",") | map(select(length > 0)) | sort end), composerVersion:$composerVersion, composerCredentialsConfigured:$composerConfigured},
				session:{mode:$sessionMode, stackId:$sessionStack, fingerprint:$sessionFingerprint, migrationOwner:$migrationOwner,
					artifactDigest:$artifactDigest, fixtureId:$fixtureId, backupSet:$backupSet, observabilitySetup:$observabilitySetup,
					edgeSetup:$edgeSetup, schemaFingerprint:$schemaFingerprint, migrationFingerprint:$migrationFingerprint, stateBackend:$stateBackend},
			 provenance:{generatedBy:"magelift-acceptance/v1", generatedAt:$generatedAt, source:$source, runId:$runId},
		 cost:{durationSeconds:$durationSeconds}}
		 | if $durationSeconds > 0 then . else del(.cost) end
		 | if $required then .required=true else . end
		 | if $accountRef != "" then .provenance.accountRef=$accountRef else . end
		 | if $imageDigest != "" then .artifact.imageDigest=$imageDigest else . end
		 | if $composerScheme != "" then .artifact.composerCredentialScheme=$composerScheme else . end
		 | if $reason != "" then .reason=$reason else . end')
	printf '%s\n' "$record" >>"$ACCEPTANCE_SHARED_EVIDENCE"
}

# append_shared_cleanup RESULT PREFIX OWNERSHIP_MARKER REMAINING_CSV
append_shared_cleanup() {
	local result="${1:?result required}"
	local prefix="${2:?prefix required}"
	local marker="${3:?ownership marker required}"
	local remaining="${4:-}"
	local run_id="${MAGELIFT_ACCEPTANCE_RUN_ID:-$ACCEPTANCE_EVIDENCE_RUN_ID}"
	local generated_at="${MAGELIFT_ACCEPTANCE_GENERATED_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
	local reason="${MAGELIFT_ACCEPTANCE_CLEANUP_REASON:-}"
	local live="${MAGELIFT_ACCEPTANCE_CLEANUP_LIVE:-}"
	local delayed="${MAGELIFT_ACCEPTANCE_CLEANUP_DELAYED:-}"
	local protected="${MAGELIFT_ACCEPTANCE_CLEANUP_PROTECTED:-}"
	local metadata="${MAGELIFT_ACCEPTANCE_CLEANUP_METADATA:-}"
	if [[ "$result" != "PASS" && -z "$reason" ]]; then
		reason="cleanup did not prove that the ownership prefix is empty"
	fi
	acceptance_shared_evidence_ensure
	local record
	record=$(jq -cn \
		--arg runId "$run_id" --arg status "$result" --arg prefix "$prefix" --arg marker "$marker" \
		--arg remaining "$remaining" --arg checkedAt "$generated_at" --arg generatedAt "$generated_at" \
		--arg source "${MAGELIFT_ACCEPTANCE_SOURCE:-acceptance-harness}" --arg reason "$reason" \
		--arg live "$live" --arg delayed "$delayed" --arg protected "$protected" --arg metadata "$metadata" \
		'{version:"v1", type:"cleanup", runId:$runId, status:$status,
		 cleanup:{status:$status, prefix:$prefix, ownershipMarker:$marker, checkedAt:$checkedAt},
		 provenance:{generatedBy:"magelift-acceptance/v1", generatedAt:$generatedAt, source:$source, runId:$runId}}
		 | if $remaining != "" then .cleanup.remaining=($remaining | split(",") | map(select(length > 0))) else . end
		 | if $live != "" then .cleanup.live=($live | split(",") | map(select(length > 0))) else . end
		 | if $delayed != "" then .cleanup.delayed=($delayed | split(",") | map(select(length > 0))) else . end
		 | if $protected != "" then .cleanup.protected=($protected | split(",") | map(select(length > 0))) else . end
		 | if $metadata != "" then .cleanup.metadata=($metadata | split(",") | map(select(length > 0))) else . end
		 | if $reason != "" then .reason=$reason else . end')
	printf '%s\n' "$record" >>"$ACCEPTANCE_SHARED_EVIDENCE"
}

# Seed dump identity for session reuse. Missing files stay explicit so a live
# Magento cell cannot pretend a fixture existed.
acceptance_seed_fixture_id() {
	local path="${1:-}"
	local base sha
	if [[ -z "$path" ]]; then
		printf 'seed:none'
		return 0
	fi
	base="$(basename "$path")"
	if [[ -f "$path" ]]; then
		sha="$(shasum -a 256 "$path" | awk '{print $1}')"
		printf 'seed:%s:sha256:%s' "$base" "$sha"
		return 0
	fi
	printf 'seed:%s:missing' "$base"
}

# Export warm-session reuse identities required for unsealed PASS JSONL.
# Values are stack identities (seed, declared backup policy, backend URL), not
# restore or traffic proofs. Call after ACCEPTANCE_CHECKPOINT_FINGERPRINT,
# digest, seed, and Pulumi backend URL are known.
acceptance_export_reuse_boundary() {
	local fixture="${1:?fixture id required}"
	local backup="${2:?backup set required}"
	local observability="${3:?observability setup required}"
	local edge="${4:?edge setup required}"
	local schema="${5:?schema fingerprint required}"
	local migration="${6:?migration fingerprint required}"
	local state="${7:?state backend required}"
	if [[ -z "${ACCEPTANCE_CHECKPOINT_FINGERPRINT:-}" ]]; then
		printf 'acceptance_export_reuse_boundary requires ACCEPTANCE_CHECKPOINT_FINGERPRINT\n' >&2
		return 2
	fi
	export MAGELIFT_ACCEPTANCE_CHECKPOINT_FINGERPRINT="$ACCEPTANCE_CHECKPOINT_FINGERPRINT"
	export MAGELIFT_ACCEPTANCE_FIXTURE_ID="$fixture"
	export MAGELIFT_ACCEPTANCE_BACKUP_SET="$backup"
	export MAGELIFT_ACCEPTANCE_OBSERVABILITY_SETUP="$observability"
	export MAGELIFT_ACCEPTANCE_EDGE_SETUP="$edge"
	export MAGELIFT_ACCEPTANCE_SCHEMA_FINGERPRINT="$schema"
	export MAGELIFT_ACCEPTANCE_MIGRATION_FINGERPRINT="$migration"
	export MAGELIFT_ACCEPTANCE_STATE_BACKEND="$state"
}
