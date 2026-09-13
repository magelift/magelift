package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func previewTestConfig() string {
	// The default starter already carries a preview env with a PR-scoped
	// domain; no surgery needed.
	return starterConfig
}

func TestResolveWithEnvironmentUsesStablePreviewIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	if err := os.WriteFile(path, []byte(previewTestConfig()), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "preview"
	o.previewRepository = "acme/magento"
	o.previewPullRequest = 41
	o.previewBranch = "feature/cart"
	o.previewCommit = strings.Repeat("a", 40)
	o.previewGeneration = 2

	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if environment != effective.Config.PreviewIdentity.Environment {
		t.Fatalf("resolved environment = %q, identity = %#v", environment, effective.Config.PreviewIdentity)
	}
	identity := effective.Config.PreviewIdentity
	if identity == nil || identity.PullRequest != 41 || identity.Branch != "feature/cart" || identity.Generation != 2 {
		t.Fatalf("preview identity = %#v", identity)
	}
	if identity.Environment == "preview" || !strings.HasPrefix(identity.StackKey, "preview-") {
		t.Fatalf("preview identity did not replace the shared environment name: %#v", identity)
	}
	if identity.Domain != "pr-41.preview.example.com" {
		t.Fatalf("preview domain = %q, want PR-scoped domain", identity.Domain)
	}
}

func TestPreviewIdentityInputsCannotTargetFixedEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.previewRepository = "acme/magento"
	o.previewPullRequest = 41
	if _, _, err := o.resolveWithEnvironment(); err == nil || !strings.Contains(err.Error(), "class preview") {
		t.Fatalf("fixed environment accepted preview identity: %v", err)
	}
}

func TestConfigEffectiveCLIPrintsPreviewIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	if err := os.WriteFile(path, []byte(previewTestConfig()), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{
		"--config", path, "--env", "preview", "--output", "json",
		"--preview-repository", "acme/magento", "--preview-number", "41",
		"config", "effective",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"pullRequest": 41`) || !strings.Contains(out.String(), `"ownershipMarker": "magelift-preview-`) {
		t.Fatalf("effective output omitted preview identity: %s", out.String())
	}
}
