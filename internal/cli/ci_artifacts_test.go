package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"go.yaml.in/yaml/v4"
)

func TestGeneratedArtifactHandoffRejectsCorruptionAndDifferentDigest(t *testing.T) {
	for _, tool := range []string{"bash", "jq", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("requires " + tool)
		}
	}
	var w strings.Builder
	w.WriteString("jobs:\n  staging:\n    steps:\n")
	writeBuildArtifactDownload(&w, false)
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string
				Run  string
			}
		}
	}
	if err := yaml.Unmarshal([]byte(w.String()), &workflow); err != nil {
		t.Fatal(err)
	}
	script := ""
	for _, step := range workflow.Jobs["staging"].Steps {
		if step.Name == "Verify artifact handoff" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("missing verification")
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, scenario := range []string{"valid", "changed manifest", "different digest"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			body := []byte(fmt.Sprintf(`{"imageDigest":%q}`, digest))
			checksum := sha256.Sum256(body)
			if scenario == "changed manifest" {
				body = []byte(`{"imageDigest":"changed"}`)
			}
			if err := os.WriteFile(filepath.Join(dir, "manifest.json"), body, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "manifest.sha256"), []byte(hex.EncodeToString(checksum[:])+"  manifest.json\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			result := fmt.Sprintf(`{"image":{"imageReference":"registry.example.invalid/shop:candidate","digest":%q}}`, digest)
			if err := os.WriteFile(filepath.Join(dir, "build-result.json"), []byte(result), 0o600); err != nil {
				t.Fatal(err)
			}
			requested := "registry.example.invalid/shop:candidate@" + digest
			if scenario == "different digest" {
				requested = "registry.example.invalid/shop:candidate@sha256:" + strings.Repeat("b", 64)
			}
			cmd := exec.Command("bash", "-c", script)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "BASH_ENV=", "DIGEST="+requested)
			output, err := cmd.CombinedOutput()
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("scenario %s error=%v output=%s", scenario, err, output)
			}
		})
	}
}

func TestGeneratedBuildStepKeepsCheckoutCleanUntilMageliftFinishes(t *testing.T) {
	for _, tool := range []string{"bash", "git", "jq", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("requires " + tool)
		}
	}
	var w strings.Builder
	w.WriteString("jobs:\n  build:\n    steps:\n")
	writeBuildAndArchive(&w, "magelift.yaml")
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string
				Run  string
			}
		}
	}
	if err := yaml.Unmarshal([]byte(w.String()), &workflow); err != nil {
		t.Fatal(err)
	}
	script := ""
	for _, step := range workflow.Jobs["build"].Steps {
		if step.Name == "Build and sign image" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("missing build step")
	}

	sha := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name, image, sha, want string
		fail                   bool
	}{
		{"explicit tag", "registry.example.invalid/shop:candidate", "", "registry.example.invalid/shop:candidate", false},
		{"repository", "registry.example.invalid/shop", sha, "registry.example.invalid/shop:" + sha, false},
		{"registry port", "registry.example.invalid:5000/shop", sha, "registry.example.invalid:5000/shop:" + sha, false},
		{"port and tag", "registry.example.invalid:5000/shop:candidate", "", "registry.example.invalid:5000/shop:candidate", false},
		{"missing SHA", "registry.example.invalid/shop", "", "", true},
		{"invalid SHA", "registry.example.invalid/shop", "not-a-commit", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			checkout := filepath.Join(root, "checkout")
			if err := os.Mkdir(checkout, 0o700); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command("git", "-C", checkout, "init").CombinedOutput(); err != nil {
				t.Fatalf("git init: %v output=%s", err, out)
			}
			if out, err := exec.Command("git", "-C", checkout, "config", "user.email", "ci@example.invalid").CombinedOutput(); err != nil {
				t.Fatalf("git config email: %v output=%s", err, out)
			}
			if out, err := exec.Command("git", "-C", checkout, "config", "user.name", "CI").CombinedOutput(); err != nil {
				t.Fatalf("git config name: %v output=%s", err, out)
			}
			readme := filepath.Join(checkout, "README.md")
			if err := os.WriteFile(readme, []byte("clean checkout\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command("git", "-C", checkout, "add", "README.md").CombinedOutput(); err != nil {
				t.Fatalf("git add: %v output=%s", err, out)
			}
			if out, err := exec.Command("git", "-C", checkout, "commit", "-m", "init").CombinedOutput(); err != nil {
				t.Fatalf("git commit: %v output=%s", err, out)
			}

			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			fakeMagelift := `#!/usr/bin/env bash
set -eu
if [ "$(git status --porcelain | wc -l)" -gt 0 ]; then
  echo "checkout dirty before build" >&2
  exit 1
fi
touch "$RUNNER_TEMP/fake-invoked"
image=""
while [ "$#" -gt 0 ]; do
  case "$1" in --image) image="$2"; shift 2 ;; *) shift ;; esac
 done
test -n "$image"
manifest_dir="${RUNNER_TEMP}/magelift-fake-manifest"
mkdir -p "$manifest_dir"
manifest="$manifest_dir/manifest.json"
digest="sha256:` + strings.Repeat("c", 64) + `"
body=$(jq -cn --arg digest "$digest" '{imageDigest:$digest}')
printf '%s' "$body" > "$manifest"
checksum=$(printf '%s' "$body" | sha256sum | awk '{print $1}')
jq -n \
  --arg manifest "$manifest" \
  --arg checksum "$checksum" \
  --arg digest "$digest" \
  --arg image "$image" \
  '{
    image: {
      imageReference: $image,
      digest: $digest
    },
    manifest: $manifest,
    manifestSha256: $checksum
  }'
`
			if err := os.WriteFile(filepath.Join(bin, "magelift"), []byte(fakeMagelift), 0o700); err != nil {
				t.Fatal(err)
			}

			runnerTemp := filepath.Join(root, "runner-temp")
			if err := os.Mkdir(runnerTemp, 0o700); err != nil {
				t.Fatal(err)
			}
			githubOutput := filepath.Join(root, "github_output")
			cmd := exec.Command("bash", "-c", script)
			cmd.Dir = checkout
			cmd.Env = append(os.Environ(),
				"BASH_ENV=",
				"PATH="+bin+":"+os.Getenv("PATH"),
				"RUNNER_TEMP="+runnerTemp,
				"MAGELIFT_RELEASE_IMAGE="+tc.image,
				"GITHUB_SHA="+tc.sha,
				"GITHUB_OUTPUT="+githubOutput,
			)
			output, err := cmd.CombinedOutput()
			if tc.fail {
				if err == nil {
					t.Fatal("invalid SHA accepted")
				}
				if _, invokeErr := os.Stat(filepath.Join(runnerTemp, "fake-invoked")); !os.IsNotExist(invokeErr) {
					t.Fatal("invalid tag reached the builder")
				}
				return
			}
			if err != nil {
				t.Fatalf("build step failed: %v output=%s", err, output)
			}
			if _, err := os.Stat(filepath.Join(checkout, "build-result.json")); err == nil {
				t.Fatal("build-result.json must not be created in checkout")
			}
			statusOut, err := exec.Command("git", "-C", checkout, "status", "--porcelain").CombinedOutput()
			if err != nil {
				t.Fatalf("git status: %v output=%s", err, statusOut)
			}
			if len(strings.TrimSpace(string(statusOut))) != 0 {
				t.Fatalf("checkout dirty after build: %s", statusOut)
			}

			handoff := filepath.Join(runnerTemp, "magelift-build")
			for _, name := range []string{"build-result.json", "manifest.json", "manifest.sha256"} {
				if _, err := os.Stat(filepath.Join(handoff, name)); err != nil {
					t.Fatalf("missing handoff file %s: %v", name, err)
				}
			}
			verify := exec.Command("bash", "-c", "set -eu; sha256sum -c manifest.sha256")
			verify.Dir = handoff
			if out, err := verify.CombinedOutput(); err != nil {
				t.Fatalf("handoff checksum verify: %v output=%s", err, out)
			}
			got, err := os.ReadFile(githubOutput)
			if err != nil {
				t.Fatal(err)
			}
			wantDigest := tc.want + "@sha256:" + strings.Repeat("c", 64)
			if !strings.Contains(string(got), "digest="+wantDigest) {
				t.Fatalf("GITHUB_OUTPUT=%q want digest=%s", got, wantDigest)
			}
		})
	}
}

func TestGeneratedPromotionDoesNotInvokeOrDependOnBuild(t *testing.T) {
	var workflow struct {
		Jobs map[string]struct {
			If    string
			Needs any
			Steps []struct {
				Uses string
				Run  string
			}
		}
	}
	rendered := renderWorkflow("magelift.yaml", "v1.2.3", []string{"staging", "production"}, awsCIWorkflowGenerator{}, config.Config{}, "aws/ecs-fargate")
	if err := yaml.Unmarshal(rendered, &workflow); err != nil {
		t.Fatal(err)
	}
	promotion := workflow.Jobs["production"]
	if promotion.Needs != "validate" {
		t.Fatalf("promotion depends on build: %#v", promotion.Needs)
	}
	if !strings.Contains(workflow.Jobs["build"].If, "inputs.environment != 'production'") {
		t.Fatal("production dispatch would rebuild")
	}
	for _, step := range promotion.Steps {
		if strings.Contains(step.Uses, "setup-buildx") || strings.Contains(step.Run, " build --push") {
			t.Fatal("promotion invokes builder")
		}
	}
}

func TestProductionArtifactRunMustBeSuccessfulSameWorkflowAndRepository(t *testing.T) {
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("requires " + tool)
		}
	}
	var w strings.Builder
	w.WriteString("jobs:\n  production:\n    steps:\n")
	writeBuildArtifactDownload(&w, true)
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string
				Run  string
			}
		}
	}
	if err := yaml.Unmarshal([]byte(w.String()), &workflow); err != nil {
		t.Fatal(err)
	}
	script := ""
	for _, step := range workflow.Jobs["production"].Steps {
		if step.Name == "Require an existing release" {
			script = step.Run
		}
	}
	for _, scenario := range []string{"valid", "fork", "PR", "other workflow", "failed run"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			gh := filepath.Join(bin, "gh")
			if err := os.WriteFile(gh, []byte("#!/bin/sh\ncat \"$ML_SOURCE_RUN_FIXTURE\"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			repo, event, path, conclusion := "fixture/shop", "push", ".github/workflows/magelift.yml", "success"
			switch scenario {
			case "fork":
				repo = "fork/shop"
			case "PR":
				event = "pull_request"
			case "other workflow":
				path = ".github/workflows/untrusted.yml"
			case "failed run":
				conclusion = "failure"
			}
			raw := fmt.Sprintf(`{"conclusion":%q,"event":%q,"path":%q,"head_repository":{"full_name":%q}}`, conclusion, event, path, repo)
			fixture := filepath.Join(dir, "run.json")
			if err := os.WriteFile(fixture, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", "-c", script)
			cmd.Env = append(os.Environ(), "BASH_ENV=", "PATH="+bin+":"+os.Getenv("PATH"), "ML_SOURCE_RUN_FIXTURE="+fixture, "DIGEST=registry.example.invalid/shop@sha256:"+strings.Repeat("a", 64), "ARTIFACT_RUN_ID=123", "REPOSITORY=fixture/shop", "WORKFLOW_REF=fixture/shop/.github/workflows/magelift.yml@refs/heads/main", "RUNNER_TEMP="+dir)
			if output, err := cmd.CombinedOutput(); (err == nil) != (scenario == "valid") {
				t.Fatalf("%s err=%v output=%s", scenario, err, output)
			}
		})
	}
}
