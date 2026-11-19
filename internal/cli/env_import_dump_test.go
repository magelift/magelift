package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/dumpimport"
	"github.com/magelift/magelift/internal/seeddump"
)

func TestEnvImportDumpSuccessMovesJournalImported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dumpPath := filepath.Join(dir, "seed.sql")
	if err := writeTestProjectFile(dumpPath, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	create := newCommandWithOptions(testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false}))
	create.SetArgs([]string{"--config", path, "--output", "json", "env", "create", "preview-import", "--dump", dumpPath})
	if err := create.Execute(); err != nil {
		t.Fatal(err)
	}

	var calls int
	var sawYes bool
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.importSeedDump = func(_ context.Context, opts dumpimport.Options) error {
		calls++
		sawYes = opts.Yes
		if opts.DumpPath != dumpPath {
			t.Fatalf("DumpPath = %q", opts.DumpPath)
		}
		return nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "import-dump", "preview-import"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("importer calls = %d, want 1", calls)
	}
	if sawYes {
		t.Fatal("recording a dump must not authorize replacing a nonempty database")
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	if result["seedDumpStatus"] != seeddump.StatusImported {
		t.Fatalf("result status = %#v", result["seedDumpStatus"])
	}

	store, err := seeddump.New(dir, "preview-import")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Read(context.Background())
	if err != nil || record == nil {
		t.Fatalf("journal read: %v %#v", err, record)
	}
	if record.Status != seeddump.StatusImported {
		t.Fatalf("journal status = %q", record.Status)
	}
}

func TestEnvImportDumpPreflightsTransportBeforeJournalMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dumpPath := filepath.Join(dir, "seed.sql")
	if err := writeTestProjectFile(dumpPath, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	create := newCommandWithOptions(testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false}))
	create.SetArgs([]string{"--config", path, "--output", "json", "env", "create", "preview-preflight", "--dump", dumpPath})
	if err := create.Execute(); err != nil {
		t.Fatal(err)
	}

	runner := &fakeDependencyRunner{missing: map[string]bool{"mysql": true, "docker": true}}
	called := false
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.dependencyRunner = path, runner
	o.importSeedDump = func(context.Context, dumpimport.Options) error {
		called = true
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "env", "import-dump", "preview-preflight"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "database dump import") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
	if called {
		t.Fatal("dump importer ran after dependency preflight failed")
	}
	store, err := seeddump.New(dir, "preview-preflight")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Read(context.Background())
	if err != nil || record == nil {
		t.Fatalf("journal: %v %#v", err, record)
	}
	if record.Status != seeddump.StatusRecorded {
		t.Fatalf("journal status = %q, want recorded", record.Status)
	}
}

func TestEnvImportDumpFailureMarksFailed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dumpPath := filepath.Join(dir, "seed.sql")
	if err := writeTestProjectFile(dumpPath, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	create := newCommandWithOptions(testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false}))
	create.SetArgs([]string{"--config", path, "--output", "json", "env", "create", "preview-fail", "--dump", dumpPath})
	if err := create.Execute(); err != nil {
		t.Fatal(err)
	}

	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.importSeedDump = func(context.Context, dumpimport.Options) error {
		return errors.New("mysql refused connection")
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "import-dump", "preview-fail"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected nonzero exit")
	}
	if ExitCode(err) == 0 {
		t.Fatalf("exit code = %d", ExitCode(err))
	}

	store, errStore := seeddump.New(dir, "preview-fail")
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
	if !strings.Contains(record.Reason, "mysql refused connection") {
		t.Fatalf("reason = %q", record.Reason)
	}
}

func TestEnvImportDumpNonEmptyRetryRequiresYes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dumpPath := filepath.Join(dir, "seed.sql")
	if err := writeTestProjectFile(dumpPath, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	create := newCommandWithOptions(testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false}))
	create.SetArgs([]string{"--config", path, "--output", "json", "env", "create", "preview-retry", "--dump", dumpPath})
	if err := create.Execute(); err != nil {
		t.Fatal(err)
	}

	store, err := seeddump.New(dir, "preview-retry")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkFailed(context.Background(), "previous import interrupted"); err != nil {
		t.Fatal(err)
	}

	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.importSeedDump = func(_ context.Context, opts dumpimport.Options) error {
		if !opts.Yes {
			return dumpimport.ErrNonEmptyRequiresYes
		}
		return nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "import-dump", "preview-retry"})
	err = cmd.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error/code = %v/%d, want invalid(2)", err, ExitCode(err))
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v", err)
	}

	record, errRead := store.Read(context.Background())
	if errRead != nil || record == nil {
		t.Fatalf("journal: %v %#v", errRead, record)
	}
	if record.Status != seeddump.StatusFailed {
		t.Fatalf("status after refuse = %q", record.Status)
	}
}

func TestEnvImportDumpWithYesAllowsFailedRetry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dumpPath := filepath.Join(dir, "seed.sql")
	if err := writeTestProjectFile(dumpPath, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	create := newCommandWithOptions(testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false}))
	create.SetArgs([]string{"--config", path, "--output", "json", "env", "create", "preview-yes", "--dump", dumpPath})
	if err := create.Execute(); err != nil {
		t.Fatal(err)
	}

	store, err := seeddump.New(dir, "preview-yes")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkFailed(context.Background(), "previous import interrupted"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.yes = true
	o.importSeedDump = func(_ context.Context, opts dumpimport.Options) error {
		if !opts.Yes {
			return dumpimport.ErrNonEmptyRequiresYes
		}
		return nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--yes", "--output", "json", "env", "import-dump", "preview-yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	record, errRead := store.Read(context.Background())
	if errRead != nil || record == nil {
		t.Fatalf("journal: %v %#v", errRead, record)
	}
	if record.Status != seeddump.StatusImported {
		t.Fatalf("status = %q", record.Status)
	}
}

func TestEnvImportDumpCommandNamedImportDumpNotSeed(t *testing.T) {
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	cmd := envImportDumpCommand(o)
	if cmd.Use != "import-dump <environment>" && !strings.HasPrefix(cmd.Use, "import-dump") {
		t.Fatalf("Use = %q, want import-dump", cmd.Use)
	}
	if strings.Contains(strings.ToLower(cmd.Use), "seed") {
		t.Fatalf("Use must not contain seed: %q", cmd.Use)
	}

	root := newCommandWithOptions(o)
	found := false
	for _, c := range root.Commands() {
		if c.Name() != "env" {
			continue
		}
		for _, sub := range c.Commands() {
			if sub.Name() == "seed" {
				t.Fatal("env must not register a seed subcommand")
			}
			if sub.Name() == "import-dump" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("env import-dump command missing from tree")
	}
}

func TestEnvImportDumpFirstAttemptRefusesNonEmptyWithoutYes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	dump := filepath.Join(dir, "seed.sql")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(dump, []byte("-- fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	create := newCommandWithOptions(testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false}))
	create.SetArgs([]string{"--config", path, "env", "create", "preview-seed", "--dump", dump})
	if err := create.Execute(); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.importSeedDump = func(_ context.Context, opts dumpimport.Options) error {
		if opts.Yes {
			t.Fatal("implicit authorization")
		}
		return dumpimport.ErrNonEmptyRequiresYes
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "env", "import-dump", "preview-seed"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error=%v", err)
	}
}

func TestEnvImportDumpFromExistingEnvironment(t *testing.T) {
	for _, nonempty := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "nonempty"}[nonempty], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "magelift.php")
			if err := writeTestProjectFile(path, []byte(starterConfig), 0600); err != nil {
				t.Fatal(err)
			}
			dump := filepath.Join(dir, "export.sql.gz")
			var called bool
			o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
			o.importSeedDump = func(_ context.Context, opts dumpimport.Options) error {
				called = true
				if opts.DumpPath != dump || opts.Yes {
					t.Fatalf("unexpected options: %#v", opts)
				}
				if nonempty {
					return dumpimport.ErrNonEmptyRequiresYes
				}
				return nil
			}
			cmd := newCommandWithOptions(o)
			cmd.SetArgs([]string{"--config", path, "env", "import-dump", "staging", "--from", dump})
			err := cmd.Execute()
			if !called || (nonempty && !errors.Is(err, dumpimport.ErrNonEmptyRequiresYes)) || (!nonempty && err != nil) {
				t.Fatalf("called=%v err=%v", called, err)
			}
			store, _ := seeddump.New(dir, "staging")
			record, err := store.Read(context.Background())
			if err != nil || record == nil || record.DumpPath != dump {
				t.Fatalf("record=%#v err=%v", record, err)
			}
			want := seeddump.StatusImported
			if nonempty {
				want = seeddump.StatusFailed
			}
			if record.Status != want {
				t.Fatalf("status=%s", record.Status)
			}
		})
	}
}

func TestEnvImportDumpFromPreservesInFlightJournal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := seeddump.InitRecorded(context.Background(), dir, "staging", "original.sql"); err != nil {
		t.Fatal(err)
	}
	store, _ := seeddump.New(dir, "staging")
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.importSeedDump = func(context.Context, dumpimport.Options) error { t.Fatal("import unexpectedly started"); return nil }
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "env", "import-dump", "staging", "--from", "other.sql"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "in-flight") {
		t.Fatalf("error=%v", err)
	}
	record, err := store.Read(context.Background())
	if err != nil || record.Status != seeddump.StatusImporting || record.DumpPath != "original.sql" {
		t.Fatalf("record=%#v err=%v", record, err)
	}
}
