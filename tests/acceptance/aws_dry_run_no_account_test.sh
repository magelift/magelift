#!/usr/bin/env bash
# Makefile dry-run sets only MAGELIFT_ACCEPTANCE_DRY_RUN=1. These wrappers
# must print the contract without AWS region, profile config, or the AWS CLI.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

run_dry() {
	local script="$1"
	local needle="$2"
	local output
	output="$(
		env -u AWS_REGION -u AWS_DEFAULT_REGION -u MAGELIFT_AWS_REGION \
			-u MAGELIFT_AWS_RECOVERY_REGION -u MAGELIFT_AWS_SECRET_RECOVERY_REGION \
			MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
			bash "$ROOT/$script"
	)"
	grep -Fq 'no AWS mutation invoked' <<<"$output"
	grep -Fq "$needle" <<<"$output"
}

run_dry scripts/aws-cloudwatch-acceptance-local.sh 'aws CloudWatch acceptance dry-run'
run_dry scripts/aws-recovery-acceptance-local.sh 'AWS S3 recovery acceptance dry-run ok'
run_dry scripts/aws-secret-recovery-acceptance-local.sh 'AWS Secrets Manager recovery acceptance dry-run ok'

printf 'aws_dry_run_no_account_test OK\n'
