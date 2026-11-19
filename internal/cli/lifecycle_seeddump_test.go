package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/dumpimport"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/seeddump"
	"go.yaml.in/yaml/v4"
)

func TestDeployNeverImportsRecordedSeedDump(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	root := filepath.Dir(path)
	dump := filepath.Join(root, "seed.sql")
	if err := os.WriteFile(dump, []byte("-- fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path = rewriteLifecycleSeedDump(t, path, dump)
	if _, err := seeddump.InitRecorded(context.Background(), root, "staging", dump); err != nil {
		t.Fatal(err)
	}
	for _, infraOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "infra-only"}[infraOnly], func(t *testing.T) {
			var order []string
			o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
			o.configPath = path
			o.environment = "staging"
			o.newBackend = func(context.Context, platform.PlannedStack, string) (infrastructureBackend, error) {
				return &fakeInfrastructureBackend{}, nil
			}
			o.newDeploySteps = func(context.Context, infrastructureBackend, platform.PlannedStack, io.Writer) (deployflow.Steps, error) {
				return fakeDeploymentSteps{order: &order}, nil
			}
			o.newLock = func(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
				return func(context.Context) error { return nil }, nil
			}
			o.importSeedDump = func(context.Context, dumpimport.Options) error {
				t.Fatal("deploy started an implicit seed import")
				return nil
			}
			cmd := newCommandWithOptions(o)
			args := []string{"--config", path, "--env", "staging", "--output", "json", "deploy"}
			if infraOnly {
				args = append(args, "--infra-only")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			store, err := seeddump.New(root, "staging")
			if err != nil {
				t.Fatal(err)
			}
			record, err := store.Read(context.Background())
			if err != nil || record == nil || record.Status != seeddump.StatusRecorded {
				t.Fatalf("journal=%#v err=%v", record, err)
			}
		})
	}
}

func rewriteLifecycleSeedDump(t *testing.T, path, dumpPath string) string {
	t.Helper()
	data, err := readTestProjectYAML(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	envs := document["environments"].(map[string]any)
	staging := envs["staging"].(map[string]any)
	staging["seedDump"] = dumpPath
	envs["staging"] = staging
	document["environments"] = envs
	updated, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(path, updated, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
