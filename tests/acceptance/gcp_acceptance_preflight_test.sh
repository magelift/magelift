#!/usr/bin/env bash
# Offline regression for gcp_acceptance_preflight (gcloud/Python doubles only).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PREFLIGHT_SH="$ROOT/providers/gcp/scripts/gcp-acceptance-preflight.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

[[ -f "$PREFLIGHT_SH" ]] || fail "missing $PREFLIGHT_SH"

mkdir -p "$TMP/bin" "$TMP/pylib"

printf 'ok\n' >"$TMP/preflight-http-mode"

cat >"$TMP/pylib/sitecustomize.py" <<'PY'
import json
import os
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

_mode_path = Path(os.environ.get("GCP_PREFLIGHT_TEST_ROOT", "")) / "preflight-http-mode"
if not _mode_path.is_file():
    raise SystemExit(0)

_mode = _mode_path.read_text(encoding="utf-8").strip()
TOKENINFO_URL = "https://oauth2.googleapis.com/tokeninfo"
RESOURCE_MANAGER = "https://cloudresourcemanager.googleapis.com/v1/projects/"


class _FakeResponse:
    def __init__(self, status: int, body: dict) -> None:
        self.status = status
        self._raw = json.dumps(body).encode()

    def read(self) -> bytes:
        return self._raw

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False


class _FakeHTTPError(urllib.error.HTTPError):
    def __init__(self, status: int, body: dict) -> None:
        super().__init__(url="", code=status, msg="", hdrs=None, fp=None)
        self._raw = json.dumps(body).encode()

    def read(self) -> bytes:
        return self._raw


def _fake_open(request, timeout=30, context=None):
    url = request.full_url
    if url.startswith(TOKENINFO_URL):
        return _FakeResponse(200, {"email": "fixture@example.com"})
    if url.startswith(RESOURCE_MANAGER) and ":testIamPermissions" not in url:
        project = urllib.parse.unquote(url[len(RESOURCE_MANAGER) :].split("?")[0])
        return _FakeResponse(
            200,
            {"projectId": project, "lifecycleState": "ACTIVE"},
        )
    if ":testIamPermissions" in url:
        if _mode == "http_error":
            raise _FakeHTTPError(403, {"error": {"message": "denied"}})
        body = json.loads(request.data.decode())
        permissions = body.get("permissions", [])
        if _mode == "denied":
            granted = permissions[:1]
        else:
            granted = permissions
        return _FakeResponse(200, {"permissions": granted})
    raise urllib.error.URLError("unexpected url")


def _fake_build_opener(*handlers, **kwargs):
    class _Opener:
        def open(self, request, timeout=30):
            return _fake_open(request, timeout=timeout)

    return _Opener()


urllib.request.urlopen = _fake_open
urllib.request.build_opener = _fake_build_opener
PY

cat >"$TMP/bin/gcloud" <<'GCLOUD'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${GCP_PREFLIGHT_GCLOUD_TRACE:?}"
if [[ "$1" == projects && "$2" == describe ]]; then
  printf '%s\n' "${FAKE_PROJECT_DESCRIBE_ID:-example-gcp-project}"
  exit 0
fi
if [[ "$1" == services && "$2" == list && "$3" == --enabled ]]; then
  if [[ -n "${FAKE_ENABLED_APIS_FILE:-}" && -f "$FAKE_ENABLED_APIS_FILE" ]]; then
    cat "$FAKE_ENABLED_APIS_FILE"
  else
    printf '%s\n' compute.googleapis.com container.googleapis.com
  fi
  exit 0
fi
if [[ "$1" == auth && "$2" == list ]]; then
  printf 'fixture@example.com\n'
  exit 0
fi
if [[ "$1" == auth && "$2" == application-default && "$3" == print-access-token ]]; then
  printf 'fixture-adc-token\n'
  exit 0
fi
if [[ "$1" == auth && "$2" == print-access-token ]]; then
  printf 'fixture-user-token\n'
  exit 0
fi
printf 'unexpected gcloud invocation: %s\n' "$*" >&2
exit 99
GCLOUD
chmod +x "$TMP/bin/gcloud"

COMMON_APIS=(
  compute.googleapis.com
  container.googleapis.com
  cloudbilling.googleapis.com
)
printf '%s\n' "${COMMON_APIS[@]}" >"$TMP/enabled-apis"

apis_for_shell() {
  local api quoted=()
  for api in "${COMMON_APIS[@]}"; do
    quoted+=("$(printf '%q' "$api")")
  done
  printf '%s' "${quoted[*]}"
}
APIS_LITERAL="$(apis_for_shell)"

run_preflight() {
  local expected_rc="$1"
  shift
  local rc=0 trace="$TMP/gcloud.trace"
  : >"$trace"
  set +e
  # shellcheck disable=SC2086
  env BASH_ENV= "PATH=$TMP/bin:$PATH" \
    "GCP_PREFLIGHT_TEST_ROOT=$TMP" \
    "PYTHONPATH=$TMP/pylib${PYTHONPATH:+:$PYTHONPATH}" \
    "GCP_PREFLIGHT_GCLOUD_TRACE=$trace" \
    "FAKE_ENABLED_APIS_FILE=$TMP/enabled-apis" \
    MAGELIFT_ACCEPTANCE_DRY_RUN=0 PROJECT=example-gcp-project REGION=us-central1 NAME=magelift-fixture01 PROFILE=preview \
    "$@" bash -c "
      set -euo pipefail
      APIS=($APIS_LITERAL)
      source \"\$1\"
      gcp_acceptance_preflight
    " bash "$PREFLIGHT_SH" >"$TMP/last.out" 2>"$TMP/last.err"
  rc=$?
  set -e
  if [[ "$rc" -ne "$expected_rc" ]]; then
    fail "expected exit $expected_rc got $rc (stderr: $(tr '\n' ' ' <"$TMP/last.err"))"
  fi
}

# Dry-run must not invoke gcloud.
trace="$TMP/dry.trace"
: >"$trace"
rc=0
env BASH_ENV= "PATH=$TMP/bin:$PATH" \
  "GCP_PREFLIGHT_GCLOUD_TRACE=$trace" \
  MAGELIFT_ACCEPTANCE_DRY_RUN=1 \
  PROJECT=example-gcp-project REGION=us-central1 NAME=magelift-fixture01 PROFILE=preview \
  bash -c "
    APIS=($APIS_LITERAL)
    source \"\$1\"
    gcp_acceptance_preflight
  " bash "$PREFLIGHT_SH" >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 0 ]] || fail "dry-run must exit 0"
[[ ! -s "$trace" ]] || fail "dry-run must not call gcloud"

run_preflight 0
grep -q 'projects describe' "$TMP/gcloud.trace" || fail "nominal path must describe project"

run_preflight 2 FAKE_PROJECT_DESCRIBE_ID=wrong-project-id
grep -q 'does not match explicit project' "$TMP/last.err" || fail "wrong project must be reported"

printf '%s\n' compute.googleapis.com >"$TMP/enabled-apis"
run_preflight 2
grep -q 'required API container.googleapis.com is not enabled' "$TMP/last.err" || fail "missing API must be reported"

printf '%s\n' "${COMMON_APIS[@]}" >"$TMP/enabled-apis"
printf 'denied\n' >"$TMP/preflight-http-mode"
run_preflight 2
grep -q '^GCP acceptance preflight: missing create or cleanup permission ' "$TMP/last.err" || fail "denied IAM must use error prefix"

printf 'ok\n' >"$TMP/preflight-http-mode"
run_preflight 2 NAME=magelift
grep -q 'refuses reserved gcp prefix' "$TMP/last.err" || fail "reserved prefix must be rejected"

rc=0
set +e
env BASH_ENV= "PATH=$TMP/bin:$PATH" \
  "GCP_PREFLIGHT_TEST_ROOT=$TMP" \
  "PYTHONPATH=$TMP/pylib" \
  "GCP_PREFLIGHT_GCLOUD_TRACE=$TMP/unavail.trace" \
  "FAKE_ENABLED_APIS_FILE=$TMP/enabled-apis" \
  PROJECT=example-gcp-project REGION=us-central1 NAME=magelift-fixture01 PROFILE=preview \
  bash -c "
    set -euo pipefail
    gcloud() { return 127; }
    export -f gcloud
    APIS=($APIS_LITERAL)
    source \"\$1\"
    gcp_acceptance_preflight
  " bash "$PREFLIGHT_SH" >/dev/null 2>"$TMP/unavail.err" || rc=$?
set -e
[[ "$rc" -eq 3 ]] || fail "unavailable gcloud must exit 3, got $rc"
grep -qE 'gcloud is not available|unable to read project' "$TMP/unavail.err" || fail "unavailable gcloud must fail closed"

printf 'http_error\n' >"$TMP/preflight-http-mode"
run_preflight 3
grep -q '^GCP acceptance preflight:' "$TMP/last.err" || fail "IAM HTTP failure must use error prefix"
if grep -Eq 'fixture-(adc|user)-token|Bearer ' "$TMP/last.err"; then
  fail "stderr must not contain access tokens"
fi

printf 'gcp_acceptance_preflight_test OK (doubles only)\n'
