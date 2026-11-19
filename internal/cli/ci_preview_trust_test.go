package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"go.yaml.in/yaml/v4"
)

var ciComparison = regexp.MustCompile(`([a-zA-Z0-9_.]+)\s*(==|!=)\s*('[^']*'|[a-zA-Z0-9_.]+)`)

// Evaluate the small equality/boolean subset emitted for job conditions.
func evaluateCITestCondition(t *testing.T, condition string, claims map[string]string) bool {
	t.Helper()
	expression := ciComparison.ReplaceAllStringFunc(condition, func(part string) string {
		m := ciComparison.FindStringSubmatch(part)
		right := m[3]
		if strings.HasPrefix(right, "'") {
			right = strings.Trim(right, "'")
		} else {
			right = claims[right]
		}
		equal := claims[m[1]] == right
		if m[2] == "!=" {
			equal = !equal
		}
		if equal {
			return "true"
		}
		return "false"
	})
	parsed, err := parser.ParseExpr(expression)
	if err != nil {
		t.Fatalf("condition %q: %v", expression, err)
	}
	var eval func(ast.Expr) bool
	eval = func(node ast.Expr) bool {
		switch n := node.(type) {
		case *ast.ParenExpr:
			return eval(n.X)
		case *ast.Ident:
			if n.Name == "true" {
				return true
			}
			if n.Name == "false" {
				return false
			}
		case *ast.BinaryExpr:
			if n.Op == token.LAND {
				return eval(n.X) && eval(n.Y)
			}
			if n.Op == token.LOR {
				return eval(n.X) || eval(n.Y)
			}
		}
		t.Fatalf("unexpected condition node %T", node)
		return false
	}
	return eval(parsed)
}

func TestPreviewWorkflowUsesReviewedBaseAndImmutableHead(t *testing.T) {
	for _, provider := range []string{"aws", "gcp", "azure"} {
		t.Run(provider, func(t *testing.T) {
			cfg := config.Config{}
			cfg.Target.Provider = provider
			var generator ciWorkflowGenerator = awsCIWorkflowGenerator{}
			if provider == "gcp" {
				generator = gcpCIWorkflowGenerator{}
				cfg.Target.GCP = &config.GCPTarget{Project: "example-gcp-project", Region: "us-central1"}
			}
			if provider == "azure" {
				generator = azureCIWorkflowGenerator{}
			}
			data := renderWorkflow("magelift.php", "v1.2.3", []string{"preview", "staging", "production"}, generator, cfg, provider)
			var workflow struct {
				On   map[string]any `yaml:"on"`
				Jobs map[string]struct {
					If          string            `yaml:"if"`
					Environment string            `yaml:"environment"`
					Permissions map[string]string `yaml:"permissions"`
					Steps       []struct {
						Name string         `yaml:"name"`
						Uses string         `yaml:"uses"`
						Run  string         `yaml:"run"`
						With map[string]any `yaml:"with"`
					} `yaml:"steps"`
				} `yaml:"jobs"`
			}
			if err := yaml.Unmarshal(data, &workflow); err != nil {
				t.Fatal(err)
			}
			if workflow.On["pull_request"] == nil || workflow.On["pull_request_target"] == nil {
				t.Fatal("missing separate PR events")
			}
			if workflow.Jobs["validate"].Permissions["id-token"] != "" || strings.Contains(string(data), "allow-unsafe-pr-checkout") {
				t.Fatal("validation has federated privilege or unsafe checkout")
			}
			for _, name := range []string{"build", "preview"} {
				condition := workflow.Jobs[name].If
				for _, required := range []string{"pull_request_target", "refs/heads/main", "github.base_ref == 'main'", "github.event.action == 'labeled'", "github.event.label.name == 'magelift-preview'", "github.event.pull_request.head.repo.full_name == github.repository"} {
					if !strings.Contains(condition, required) {
						t.Fatalf("%s missing %s: %s", name, required, condition)
					}
				}
				if strings.Contains(condition, "synchronize") {
					t.Fatal("new commits must require a fresh label")
				}
			}
			build := workflow.Jobs["build"]
			baseClaims := map[string]string{"github.event_name": "pull_request_target", "github.ref": "refs/heads/main", "github.base_ref": "main", "github.event.action": "labeled", "github.event.label.name": "magelift-preview", "github.event.pull_request.head.repo.full_name": "agency/shop", "github.repository": "agency/shop"}
			for _, test := range []struct {
				name, key, value string
				allowed          bool
			}{
				{"trusted label", "", "", true},
				{"fork", "github.event.pull_request.head.repo.full_name", "attacker/shop", false},
				{"wrong ref", "github.ref", "refs/heads/other", false},
				{"wrong base", "github.base_ref", "other", false},
				{"different label", "github.event.label.name", "other", false},
				{"retained label after push", "github.event.action", "synchronize", false},
				{"unprivileged PR", "github.event_name", "pull_request", false},
			} {
				claims := make(map[string]string, len(baseClaims))
				for k, v := range baseClaims {
					claims[k] = v
				}
				if test.key != "" {
					claims[test.key] = test.value
				}
				for _, job := range []string{"build", "preview"} {
					if got := evaluateCITestCondition(t, workflow.Jobs[job].If, claims); got != test.allowed {
						t.Fatalf("%s %s allowed=%v", test.name, job, got)
					}
				}
			}
			if !strings.Contains(build.Environment, "preview-build") || !strings.Contains(build.Environment, "'build'") {
				t.Fatalf("build environment = %s", build.Environment)
			}
			if workflow.Jobs["preview"].Environment != "preview" {
				t.Fatal("preview has wrong environment")
			}
			if got := build.Steps[0].With["ref"]; got != "${{ github.event.pull_request.head.sha || github.sha }}" {
				t.Fatalf("shop checkout ref = %v", got)
			}
			if got := build.Steps[1].With["ref"]; got != "${{ github.event.pull_request.base.sha || github.sha }}" {
				t.Fatalf("trusted checkout ref = %v", got)
			}
			if build.Steps[2].Name != "Verify source identity" || !strings.Contains(build.Steps[2].Run, "test ! -L shop/composer.lock") {
				t.Fatal("source verification must precede credentials")
			}
			install, credentials := -1, -1
			for i, step := range build.Steps {
				if step.Name == "Install MageLift" {
					install = i
				}
				if strings.Contains(step.Name, "Configure AWS credentials") || strings.Contains(step.Name, "Authenticate to Google Cloud") || strings.Contains(step.Name, "Sign in to Azure") {
					credentials = i
				}
			}
			if install < 3 || credentials <= install {
				t.Fatalf("source validation and verified install must precede cloud credentials: install=%d credentials=%d", install, credentials)
			}
			for _, step := range workflow.Jobs["preview"].Steps {
				if step.With["path"] == "shop" || strings.Contains(step.Run, "shop/") {
					t.Fatal("deploy job uses PR source")
				}
			}
			for _, name := range []string{"staging", "production"} {
				if strings.Contains(workflow.Jobs[name].If, "pull_request_target") || !strings.Contains(workflow.Jobs[name].If, "refs/heads/main") {
					t.Fatalf("%s condition %s", name, workflow.Jobs[name].If)
				}
			}
		})
	}
}
