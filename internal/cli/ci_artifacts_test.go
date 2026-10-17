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
