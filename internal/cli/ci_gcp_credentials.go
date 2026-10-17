package cli

import "strings"

// The auth action's post hook reads the updated GOOGLE_GHA_CREDS_PATH.
func writeGCPIdentityFileRelocation(workflow *strings.Builder) {
	workflow.WriteString(`      - name: Relocate GCP federated credentials
        run: |
          set -eu
          src="${GOOGLE_GHA_CREDS_PATH:?GOOGLE_GHA_CREDS_PATH is required}"
          workspace_real="$(realpath -- "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is required}")"
          runner_temp="$(realpath -- "${RUNNER_TEMP:?RUNNER_TEMP is required}")"
          if [ -L "$src" ] || [ ! -f "$src" ]; then
            echo 'credential file must be a regular file' >&2
            exit 1
          fi
          src_real="$(realpath -- "$src")"
          case "$src_real" in
            "$workspace_real"/gha-creds-*.json) ;;
            *) echo 'unexpected credential file outside GITHUB_WORKSPACE or with an invalid name' >&2; exit 1 ;;
          esac
          case "$runner_temp" in
            "$workspace_real"|"$workspace_real"/*)
              echo 'configure RUNNER_TEMP outside GITHUB_WORKSPACE' >&2
              exit 1 ;;
          esac
          dest="$(mktemp "$runner_temp/gha-creds-reloc-XXXXXXXX")"
          cleanup() { rm -f -- "$dest"; }
          trap cleanup EXIT
          mv -- "$src" "$dest"
          chmod 0600 "$dest"
          {
            echo "GOOGLE_GHA_CREDS_PATH=$dest"
            echo "GOOGLE_APPLICATION_CREDENTIALS=$dest"
            echo "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE=$dest"
          } >>"$GITHUB_ENV"
          trap - EXIT
`)
}
