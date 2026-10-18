#!/usr/bin/env bash
# Offline acceptance test for the exact predeclared Secrets Manager boundary.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat >"$TMP/aws" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
case "$*" in
*"head-bucket"*)
  if [[ "${STATE_BUCKET_ABSENT:-0}" == 1 ]]; then
    printf '%s\n' 'An error occurred (404) when calling the HeadBucket operation: Not Found' >&2
    exit 254
  fi
  ;;
*"get-bucket-tagging"*)
  printf '%s\n' '{"TagSet":[{"Key":"magelift:project","Value":"acceptance"},{"Key":"magelift:managed-by","Value":"magelift"},{"Key":"magelift:purpose","Value":"acceptance-state"},{"Key":"magelift:acceptance-run","Value":"magelift-acceptance-preview"}]}'
  ;;
*"list-buckets"*"--output json"*)
  if [[ "${STATE_BUCKET_ABSENT:-0}" == 1 ]]; then
    printf '%s\n' '{"Buckets":[]}'
  elif [[ "${STATE_BUCKET_LEFTOVER:-0}" == 1 ]]; then
    printf '%s\n' '{"Buckets":[{"Name":"magelift-acceptance-preview-state"},{"Name":"magelift-acceptance-leftover"}]}'
  else
    printf '%s\n' '{"Buckets":[{"Name":"magelift-acceptance-preview-state"}]}'
  fi
  ;;
*"describe-secret"*)
  printf '%s\n' '[{"Key":"magelift:project","Value":"acceptance"},{"Key":"magelift:managed-by","Value":"magelift"},{"Key":"magelift:purpose","Value":"acceptance"},{"Key":"magelift:acceptance-run","Value":"magelift-acceptance-preview"}]'
  ;;
*"list-secrets"*"length("*) printf '1\n' ;;
*"list-secrets"*) printf '%s\n' '[["arn:aws:secretsmanager:eu-north-1:123456789012:secret:magelift-acceptance-token-AbCdEf","magelift-acceptance-token"]]' ;;
*) printf '0\n' ;;
esac
EOF
chmod +x "$TMP/aws"
export PATH="$TMP:$PATH"
export MAGELIFT_AWS_ACCEPTANCE_PREREQUISITE_SECRET_ARNS=$'arn:aws:secretsmanager:eu-north-1:123456789012:secret:magelift-acceptance-token-AbCdEf\narn:aws:secretsmanager:eu-north-1:123456789012:secret:magelift-acceptance-other-XyZ123'

# shellcheck source=../../scripts/acceptance/lib-assert-clean-aws.sh
source "$ROOT/scripts/acceptance/lib-assert-clean-aws.sh"

allowed='arn:aws:secretsmanager:eu-north-1:123456789012:secret:magelift-acceptance-token-AbCdEf'
foreign='arn:aws:secretsmanager:eu-north-1:123456789012:secret:magelift-foreign-token-AbCdEf'
acceptance_secret_arn_is_predeclared "$allowed"
if acceptance_secret_arn_is_predeclared "$foreign"; then
	printf 'foreign ARN unexpectedly allowed\n' >&2
	exit 1
fi
acceptance_secret_has_run_tags "$allowed" eu-north-1 acceptance magelift-acceptance-preview

if ! assert_clean_aws acceptance eu-north-1 magelift-acceptance-preview >/dev/null 2>"$TMP/clean.log"; then
	cat "$TMP/clean.log" >&2
	exit 1
fi
grep -q 'allowing exact predeclared acceptance secret' "$TMP/clean.log"

export MAGELIFT_AWS_ACCEPTANCE_PREREQUISITE_SECRET_ARNS="$foreign"
if assert_clean_aws acceptance eu-north-1 magelift-acceptance-preview >/dev/null 2>"$TMP/rejected.log"; then
	printf 'foreign secret was not rejected\n' >&2
	exit 1
fi
grep -q 'leftover Secrets Manager secret' "$TMP/rejected.log"

export MAGELIFT_AWS_ACCEPTANCE_PREREQUISITE_SECRET_ARNS="$allowed"
if ! assert_clean_aws acceptance eu-north-1 magelift-acceptance-preview magelift-acceptance-preview-state >/dev/null 2>"$TMP/state-clean.log"; then
	cat "$TMP/state-clean.log" >&2
	exit 1
fi
grep -q 'allowing exact owned AWS Pulumi state bucket' "$TMP/state-clean.log"

export STATE_BUCKET_ABSENT=1
if ! assert_clean_aws acceptance eu-north-1 magelift-acceptance-preview magelift-acceptance-preview-state >/dev/null 2>"$TMP/state-new-run.log"; then
  cat "$TMP/state-new-run.log" >&2
  exit 1
fi
grep -q 'treating this as a new acceptance run' "$TMP/state-new-run.log"

export STATE_BUCKET_LEFTOVER=1
unset STATE_BUCKET_ABSENT
if assert_clean_aws acceptance eu-north-1 magelift-acceptance-preview magelift-acceptance-preview-state >/dev/null 2>"$TMP/state-leftover.log"; then
	printf 'non-state S3 bucket was not rejected during resume assertion\n' >&2
	exit 1
fi
grep -q 'leftover S3 buckets containing project tag' "$TMP/state-leftover.log"

printf 'assert_clean_prerequisite_test OK\n'
