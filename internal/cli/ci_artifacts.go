package cli

import "strings"

const uploadArtifactAction = "actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2"
const downloadArtifactAction = "actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2"

func writeRegistryBuildAuth(w *strings.Builder, gcp bool) {
	w.WriteString(`      - name: Validate registry destination
        env:
          IMAGE: ${{ vars.MAGELIFT_RELEASE_IMAGE }}
          REGISTRY: ${{ vars.MAGELIFT_REGISTRY }}
        run: |
          set -eu
          test -n "$IMAGE"
          test -n "$REGISTRY"
          test "${IMAGE%%/*}" = "$REGISTRY"
`)
	if gcp {
		w.WriteString(`      - name: Authenticate Artifact Registry
        if: endsWith(vars.MAGELIFT_REGISTRY, '-docker.pkg.dev')
        env:
          REGISTRY: ${{ vars.MAGELIFT_REGISTRY }}
        run: gcloud auth configure-docker "$REGISTRY" --quiet
`)
	}
	w.WriteString("      - name: Authenticate user GHCR repository\n        if: vars.MAGELIFT_REGISTRY == 'ghcr.io'\n        uses: " + dockerLoginAction + "\n        with:\n          registry: ghcr.io\n          username: ${{ github.actor }}\n          password: ${{ secrets.GITHUB_TOKEN }}\n")
	genericCondition := "vars.MAGELIFT_REGISTRY != 'ghcr.io'"
	if gcp {
		genericCondition += " && !endsWith(vars.MAGELIFT_REGISTRY, '-docker.pkg.dev')"
	}
	w.WriteString("      - name: Validate generic registry credentials\n        if: " + genericCondition + "\n" + `        env:
          USERNAME: ${{ vars.MAGELIFT_REGISTRY_USERNAME }}
          PASSWORD: ${{ secrets.MAGELIFT_REGISTRY_PASSWORD }}
        run: |
          test -n "$USERNAME"
          test -n "$PASSWORD"
`)
	w.WriteString("      - name: Authenticate user registry\n        if: " + genericCondition + "\n        uses: " + dockerLoginAction + "\n" + `        with:
          registry: ${{ vars.MAGELIFT_REGISTRY }}
          username: ${{ vars.MAGELIFT_REGISTRY_USERNAME }}
          password: ${{ secrets.MAGELIFT_REGISTRY_PASSWORD }}
`)
}

func writeBuildAndArchive(w *strings.Builder, configFile string) {
	w.WriteString(`      - name: Build and sign image
        id: build
        env:
          MAGELIFT_RELEASE_IMAGE: ${{ vars.MAGELIFT_RELEASE_IMAGE }}
          COMPOSER_AUTH: ${{ secrets.MAGELIFT_COMPOSER_AUTH }}
        run: |
          set -eu
          test -n "$MAGELIFT_RELEASE_IMAGE"
          image="$MAGELIFT_RELEASE_IMAGE"
          case "${image##*/}" in
            *:*) ;;
            *)
              printf '%s' "$GITHUB_SHA" | grep -Eq '^[a-f0-9]{40}$'
              image="$image:$GITHUB_SHA"
              ;;
          esac
          build_dir="$RUNNER_TEMP/magelift-build"
          result="$build_dir/build-result.json"
          mkdir -p "$build_dir"
          magelift --config ` + shellQuote(configFile) + ` --no-interaction --output json build --push \
            --image "$image" > "$result"
          digest="$(jq -er '.image | (.imageReference | sub(":[^/:]+$"; "")) + "@" + .digest' "$result")"
          cp -- "$(jq -er '.manifest' "$result")" "$build_dir/manifest.json"
          checksum="$(jq -er '.manifestSha256 | select(test("^[a-f0-9]{64}$"))' "$result")"
          printf '%s  manifest.json\n' "$checksum" > "$build_dir/manifest.sha256"
          (cd "$build_dir" && sha256sum -c manifest.sha256)
          printf 'digest=%s\n' "$digest" >> "$GITHUB_OUTPUT"
`)
	w.WriteString("      - name: Retain existing artifact manifest\n        uses: " + uploadArtifactAction + "\n" + `        with:
          name: magelift-build-${{ github.run_id }}
          path: ${{ runner.temp }}/magelift-build/
          if-no-files-found: error
          retention-days: 14
`)
}

func writeBuildArtifactDownload(w *strings.Builder, promotion bool) {
	if promotion {
		w.WriteString(`      - name: Require an existing release
        env:
          DIGEST: ${{ inputs.release_digest }}
          ARTIFACT_RUN_ID: ${{ inputs.artifact_run_id }}
          GH_TOKEN: ${{ github.token }}
          REPOSITORY: ${{ github.repository }}
          WORKFLOW_REF: ${{ github.workflow_ref }}
        run: |
          test -n "$DIGEST"
          test -n "$ARTIFACT_RUN_ID"
          case "$ARTIFACT_RUN_ID" in *[!0-9]*) exit 1 ;; esac
          workflow_path="${WORKFLOW_REF#"$REPOSITORY"/}"
          workflow_path="${workflow_path%@*}"
          gh api "repos/$REPOSITORY/actions/runs/$ARTIFACT_RUN_ID" > "$RUNNER_TEMP/magelift-source-run.json"
          jq -e --arg repo "$REPOSITORY" --arg path "$workflow_path" '.conclusion == "success" and .head_repository.full_name == $repo and .path == $path and (.event == "push" or .event == "workflow_dispatch")' "$RUNNER_TEMP/magelift-source-run.json" > /dev/null
`)
	}
	w.WriteString("      - name: Download existing artifact manifest\n        uses: " + downloadArtifactAction + "\n        with:\n")
	if promotion {
		w.WriteString("          name: magelift-build-${{ inputs.artifact_run_id }}\n          run-id: ${{ inputs.artifact_run_id }}\n          github-token: ${{ github.token }}\n")
	} else {
		w.WriteString("          name: magelift-build-${{ github.run_id }}\n")
	}
	w.WriteString("          path: ${{ runner.temp }}/magelift-build/\n      - name: Verify artifact handoff\n        env:\n")
	if promotion {
		w.WriteString("          DIGEST: ${{ inputs.release_digest }}\n")
	} else {
		w.WriteString("          DIGEST: ${{ needs.build.outputs.digest }}\n")
	}
	w.WriteString(`        working-directory: ${{ runner.temp }}/magelift-build/
        run: |
          set -eu
          sha256sum -c manifest.sha256
          test "$(jq -er '.image | (.imageReference | sub(":[^/:]+$"; "")) + "@" + .digest' build-result.json)" = "$DIGEST"
          test "$(jq -er '.imageDigest' manifest.json)" = "${DIGEST##*@}"
`)
}
