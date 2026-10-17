package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestMageLiftImageWorkflowCannotPublish(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "images.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		On          map[string]any
		Permissions map[string]string
		Jobs        map[string]struct {
			Steps []struct {
				Uses string
				Run  string
				With map[string]any
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.On) != 1 {
		t.Fatal("recipe validation must be manually triggered")
	}
	if _, ok := workflow.On["workflow_dispatch"]; !ok {
		t.Fatal("missing manual validation")
	}
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Fatal("recipe validation must be read-only")
	}
	builds := 0
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.Uses == "" {
				continue
			}
			if _, ok := step.With["push"]; ok {
				builds++
				if step.With["push"] != false || step.With["load"] != true {
					t.Fatal("recipe CI must load without publishing")
				}
			}
		}
	}
	if builds != 1 {
		t.Fatalf("expected one matrix build, got %d", builds)
	}
}
