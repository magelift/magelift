#!/usr/bin/env bash
# Read-only GCP acceptance preflight: project, enabled APIs, ADC identity, and
# bounded create/cleanup IAM. Sourced by gcp-acceptance-local.sh only.
# shellcheck shell=bash

_gcp_preflight_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
# shellcheck source=../../../scripts/acceptance/lib-campaign-isolation.sh
source "$_gcp_preflight_root/scripts/acceptance/lib-campaign-isolation.sh"

_gcp_preflight_py="${_gcp_preflight_root}/providers/gcp/scripts/gcp-acceptance-preflight.py"

_gcp_acceptance_preflight_permissions=(
  container.clusters.create
  container.clusters.delete
  cloudsql.instances.create
  cloudsql.instances.delete
  memorystore.instances.create
  memorystore.instances.delete
  compute.networks.create
  compute.networks.delete
  storage.buckets.create
  storage.buckets.delete
  secretmanager.secrets.create
  secretmanager.secrets.delete
  iam.serviceAccounts.create
  iam.serviceAccounts.delete
  iam.workloadIdentityPools.create
  iam.workloadIdentityPools.delete
  resourcemanager.projects.setIamPolicy
  serviceusage.services.use
  serviceusage.services.enable
)

gcp_acceptance_preflight() {
  if [[ "${MAGELIFT_ACCEPTANCE_DRY_RUN:-}" == 1 || "${MAGELIFT_ACCEPTANCE_DRY_RUN:-}" == true ]]; then
    return 0
  fi

  if [[ -z "${PROJECT:-}" ]]; then
    printf 'GCP acceptance preflight: explicit project is required\n' >&2
    return 2
  fi
  if [[ ! "$PROJECT" =~ ^[a-z][a-z0-9-]{4,28}[a-z0-9]$ ]]; then
    printf 'GCP acceptance preflight: explicit project ID is invalid: %s\n' "$PROJECT" >&2
    return 2
  fi
  if [[ -z "${NAME:-}" ]]; then
    printf 'GCP acceptance preflight: owned resource prefix (NAME) is required\n' >&2
    return 2
  fi
  if [[ ! "$NAME" =~ ^[a-z][a-z0-9-]*$ ]]; then
    printf 'GCP acceptance preflight: owned resource prefix is invalid\n' >&2
    return 2
  fi
  if ! acceptance_campaign_require_prefix gcp "$NAME"; then
    return 2
  fi
  if ((${#APIS[@]} < 1)); then
    printf 'GCP acceptance preflight: required API list (APIS) is empty\n' >&2
    return 2
  fi

  if ! command -v gcloud >/dev/null 2>&1; then
    printf 'GCP acceptance preflight: gcloud is not available\n' >&2
    return 3
  fi

  local resolved
  if ! resolved="$(gcloud projects describe "$PROJECT" --format='value(projectId)' 2>/dev/null)"; then
    printf 'GCP acceptance preflight: unable to read project %s with the effective credential\n' "$PROJECT" >&2
    return 3
  fi
  if [[ "$resolved" != "$PROJECT" ]]; then
    printf 'GCP acceptance preflight: gcloud project describe does not match explicit project %s\n' "$PROJECT" >&2
    return 2
  fi

  local enabled_raw api
  if ! enabled_raw="$(gcloud services list --enabled --project="$PROJECT" --format='value(config.name)' 2>/dev/null)"; then
    printf 'GCP acceptance preflight: unable to list enabled APIs for project %s\n' "$PROJECT" >&2
    return 3
  fi
  for api in "${APIS[@]}"; do
    if ! grep -Fxq "$api" <<<"$enabled_raw"; then
      printf 'GCP acceptance preflight: required API %s is not enabled on project %s\n' "$api" "$PROJECT" >&2
      return 2
    fi
  done

  if [[ ! -f "$_gcp_preflight_py" ]]; then
    printf 'GCP acceptance preflight: IAM permission check did not complete\n' >&2
    return 3
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    printf 'GCP acceptance preflight: IAM permission check did not complete\n' >&2
    return 3
  fi

  if python3 "$_gcp_preflight_py" "$PROJECT" "${_gcp_acceptance_preflight_permissions[@]}"; then
    return 0
  else
    return "$?"
  fi
}
