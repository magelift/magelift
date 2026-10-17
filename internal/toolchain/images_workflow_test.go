package toolchain

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

type publicationWorkflow struct {
	On struct {
		WorkflowDispatch struct {
			Inputs map[string]struct {
				Default  any
				Required bool
			}
		} `yaml:"workflow_dispatch"`
		Push struct {
			Branches []string
			Tags     []string
		}
	}
	Jobs map[string]struct {
		Strategy struct {
			Matrix struct {
				Include []struct {
					Image string
					PHP   string `yaml:"php"`
				}
			}
		}
		Steps []struct {
			Name string
			Run  string
			With map[string]any
		}
	}
}

func readPublicationWorkflow(t *testing.T) publicationWorkflow {
	t.Helper()
	bytes, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "images.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow publicationWorkflow
	if err := yaml.Unmarshal(bytes, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

func TestCommunityImagePublicationIsExplicitAndScoped(t *testing.T) {
	workflow := readPublicationWorkflow(t)
	publish, ok := workflow.On.WorkflowDispatch.Inputs["publish"]
	if !ok || publish.Default != false {
		t.Fatal("manual image publication must default to false")
	}
	if !workflow.On.WorkflowDispatch.Inputs["version"].Required {
		t.Fatal("manual image publication requires an explicit version")
	}
	if len(workflow.On.Push.Branches) != 0 {
		t.Fatal("branch pushes must not publish images")
	}
	job, ok := workflow.Jobs["community-alpha"]
	if !ok {
		t.Fatal("missing bounded alpha job")
	}
	kinds := map[string]bool{}
	for _, cell := range job.Strategy.Matrix.Include {
		if cell.PHP != "8.5" {
			t.Fatalf("unexpected alpha PHP: %s", cell.PHP)
		}
		kinds[cell.Image] = true
	}
	if len(job.Strategy.Matrix.Include) != 2 || len(kinds) != 2 || !kinds["builder"] || !kinds["nginx"] {
		t.Fatalf("unexpected alpha image matrix: %+v", job.Strategy.Matrix.Include)
	}
}

func TestCommunityImageVersionGuardExecutesBeforePublication(t *testing.T) {
	workflow := readPublicationWorkflow(t)
	var guard string
	for _, step := range workflow.Jobs["community-alpha"].Steps {
		if step.Name == "Validate image version" {
			guard = step.Run
		}
		if _, publishes := step.With["push"]; publishes && guard == "" {
			t.Fatal("image publisher precedes source/version validation")
		}
	}
	if guard == "" {
		t.Fatal("missing shared image version guard")
	}
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name, version, ref string
		allowed            bool
	}{
		{"manual explicit", "v0.1.0-alpha.1", "refs/heads/review", true},
		{"mutable latest", "latest", "refs/heads/review", false},
		{"version injection", "v1.0.0\nlatest", "refs/heads/review", false},
		{"approved exact commit", "v0.0.0-community." + sha, "refs/tags/images/community-alpha." + sha, true},
		{"wrong commit", "v0.0.0-community." + sha, "refs/tags/images/community-alpha." + strings.Repeat("b", 40), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "-c", guard)
			cmd.Env = append(os.Environ(), "BASH_ENV=", "VERSION="+tc.version, "GITHUB_REF="+tc.ref, "GITHUB_SHA="+sha)
			output, err := cmd.CombinedOutput()
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v, error=%v, output=%s", tc.allowed, err, output)
			}
		})
	}
}
