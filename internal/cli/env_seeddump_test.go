package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/seeddump"
)

func TestEnvCreateDumpPersistsSeedDumpAndJournalRecorded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dumpPath := filepath.Join(dir, "seed.sql")
	if err := os.WriteFile(dumpPath, []byte("-- fixture dump\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "create", "preview-dump", "--dump", dumpPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode create output: %v\n%s", err, out.String())
	}
	if result["seedDump"] != dumpPath {
		t.Fatalf("seedDump = %#v", result["seedDump"])
	}
	if result["seedDumpStatus"] != seeddump.StatusRecorded {
		t.Fatalf("seedDumpStatus = %#v (want %q, not ADR placeholder)", result["seedDumpStatus"], seeddump.StatusRecorded)
	}
	if status, _ := result["seedDumpStatus"].(string); strings.Contains(status, "ADR") {
		t.Fatalf("create still uses placeholder prose: %q", status)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := config.Load(data)
	if err != nil {
		t.Fatalf("Load after create --dump: %v", err)
	}
	effective, err := file.Resolve("preview-dump", config.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve after create --dump: %v", err)
	}
	if effective.Config.SeedDump != dumpPath {
		t.Fatalf("effective SeedDump = %q", effective.Config.SeedDump)
	}

	store, err := seeddump.New(dir, "preview-dump")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Read(cmd.Context())
	if err != nil {
		t.Fatal(err)
	}
	if record == nil || record.Status != seeddump.StatusRecorded {
		t.Fatalf("journal = %#v", record)
	}
	if record.DumpPath != dumpPath {
		t.Fatalf("journal dumpPath = %q", record.DumpPath)
	}
}
