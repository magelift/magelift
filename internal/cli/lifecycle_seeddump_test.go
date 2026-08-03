package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/dumpimport"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/seeddump"
	"go.yaml.in/yaml/v4"
)

func TestAutoImportOnceFromRecorded(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	projectRoot := filepath.Dir(path)
	dumpInProject := filepath.Join(projectRoot, "seed.sql")
	if err := os.WriteFile(dumpInProject, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path = rewriteLifecycleSeedDump(t, path, dumpInProject)
	if _, err := seeddump.InitRecorded(context.Background(), projectRoot, "staging", dumpInProject); err != nil {
		t.Fatal(err)
	}

	var importCalls int
	var order []string
	backend := &fakeInfrastructureBackend{}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newDeploySteps = func(context.Context, infrastructureBackend, platform.PlannedStack, io.Writer) (deployflow.Steps, error) {
		return fakeDeploymentSteps{order: &order}, nil
	}
	o.newLock = func(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
		order = append(order, "lock.acquire")
		return func(context.Context) error { order = append(order, "lock.release"); return nil }, nil
	}
	o.importSeedDump = func(context.Context, dumpimport.Options) error {
		importCalls++
		order = append(order, "seed.import")
		return nil
	}

	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "deploy"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if importCalls != 1 {
		t.Fatalf("first deploy importCalls = %d, want 1", importCalls)
	}
	if idx := indexOf(order, "seed.import"); idx < 0 {
		t.Fatalf("seed.import missing from order: %v", order)
	} else if release := indexOf(order, "lock.release"); release < 0 || idx < release {
		t.Fatalf("seed.import must run after lock.release; order=%v", order)
	}

	store, err := seeddump.New(projectRoot, "staging")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Read(context.Background())
	if err != nil || record == nil || record.Status != seeddump.StatusImported {
		t.Fatalf("journal after first deploy = %#v err=%v", record, err)
	}

	order = nil
	cmd = newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "deploy"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if importCalls != 1 {
		t.Fatalf("second deploy re-imported: importCalls = %d", importCalls)
	}
}

func TestAutoImportRecordedOnly(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	projectRoot := filepath.Dir(path)
	dumpInProject := filepath.Join(projectRoot, "seed.sql")
	if err := os.WriteFile(dumpInProject, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path = rewriteLifecycleSeedDump(t, path, dumpInProject)
	if _, err := seeddump.InitRecorded(context.Background(), projectRoot, "staging", dumpInProject); err != nil {
		t.Fatal(err)
	}
	store, err := seeddump.New(projectRoot, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImported(context.Background()); err != nil {
		t.Fatal(err)
	}

	var importCalls int
	order := []string{}
	backend := &fakeInfrastructureBackend{}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newDeploySteps = func(context.Context, infrastructureBackend, platform.PlannedStack, io.Writer) (deployflow.Steps, error) {
		return fakeDeploymentSteps{order: &order}, nil
	}
	o.newLock = func(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	o.importSeedDump = func(context.Context, dumpimport.Options) error {
		importCalls++
		return nil
	}

	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "deploy"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if importCalls != 0 {
		t.Fatalf("imported status must not auto-import: calls=%d", importCalls)
	}
}

func TestAutoImportFailureFailsDeploy(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	projectRoot := filepath.Dir(path)
	dumpInProject := filepath.Join(projectRoot, "seed.sql")
	if err := os.WriteFile(dumpInProject, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path = rewriteLifecycleSeedDump(t, path, dumpInProject)
	if _, err := seeddump.InitRecorded(context.Background(), projectRoot, "staging", dumpInProject); err != nil {
		t.Fatal(err)
	}

	order := []string{}
	backend := &fakeInfrastructureBackend{}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newDeploySteps = func(context.Context, infrastructureBackend, platform.PlannedStack, io.Writer) (deployflow.Steps, error) {
		return fakeDeploymentSteps{order: &order}, nil
	}
	o.newLock = func(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	o.importSeedDump = func(context.Context, dumpimport.Options) error {
		return errors.New("dump pipe broken")
	}

	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "deploy"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected deploy to fail when auto-import fails")
	}
	if !strings.Contains(err.Error(), "dump pipe broken") {
		t.Fatalf("error = %v", err)
	}

	store, errStore := seeddump.New(projectRoot, "staging")
	if errStore != nil {
		t.Fatal(errStore)
	}
	record, errRead := store.Read(context.Background())
	if errRead != nil || record == nil {
		t.Fatalf("journal: %v %#v", errRead, record)
	}
	if record.Status != seeddump.StatusFailed {
		t.Fatalf("status = %q", record.Status)
	}
	if !strings.Contains(record.Reason, "dump pipe broken") {
		t.Fatalf("reason = %q", record.Reason)
	}
}

func rewriteLifecycleSeedDump(t *testing.T, path, dumpPath string) string {
	t.Helper()
	data, err := os.ReadFile(path)
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
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func indexOf(order []string, needle string) int {
	for i, s := range order {
		if s == needle {
			return i
		}
	}
	return -1
}
