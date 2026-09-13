package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/dumpimport"
)

func TestEnvDumpWritesLocalFileAndLabelsUnsanitized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "staging.sql")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.getenv = func(key string) string {
		if key == "MAGELIFT_DUMPIMPORT_PASSWORD" {
			return "super-secret-db"
		}
		return ""
	}
	var sawOpts dumpimport.Options
	o.exportDump = func(_ context.Context, opts dumpimport.Options) error {
		sawOpts = opts
		if opts.Password != "super-secret-db" {
			t.Fatalf("password not passed to exporter")
		}
		if opts.OutputPath != dest {
			t.Fatalf("OutputPath = %q, want %q", opts.OutputPath, dest)
		}
		return os.WriteFile(opts.OutputPath, []byte("-- fixture dump\n"), 0o600)
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "dump", "staging", "--to", dest})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if sawOpts.OutputPath == "" {
		t.Fatal("exporter was not called")
	}
	got := out.String()
	if strings.Contains(got, "super-secret-db") {
		t.Fatalf("password leaked into CLI output: %s", got)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v\n%s", err, got)
	}
	if result["environment"] != "staging" || result["sanitized"] != false || result["label"] != dumpimport.UnsanitizedLabel {
		t.Fatalf("result = %#v", result)
	}
	if result["path"] != dest {
		t.Fatalf("path = %#v", result["path"])
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "-- fixture dump\n" {
		t.Fatalf("dump file = %q", body)
	}
}

func TestEnvDumpRequiresTo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	called := false
	o.exportDump = func(context.Context, dumpimport.Options) error {
		called = true
		return nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "env", "dump", "staging"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) == 0 {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	if called {
		t.Fatal("exporter ran without --to")
	}
}

func TestEnvDumpRefusesOverwriteWithoutYes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "staging.sql")
	if err := os.WriteFile(dest, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.exportDump = func(_ context.Context, opts dumpimport.Options) error {
		if !opts.Yes {
			return dumpimport.ErrOutputExists
		}
		return nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "dump", "staging", "--to", dest})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error/code = %v/%d, want invalid 2", err, ExitCode(err))
	}
	body, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(body) != "keep" {
		t.Fatalf("file mutated: %q", body)
	}
}

func TestEnvDumpIsOnCommandTree(t *testing.T) {
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	root := newCommandWithOptions(o)
	found := false
	for _, c := range root.Commands() {
		if c.Name() != "env" {
			continue
		}
		for _, sub := range c.Commands() {
			if sub.Name() == "dump" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("env dump command missing from tree")
	}
}

func TestEnvDumpSanitizePassesOptInToExporter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "staging.sql")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	var sawOpts dumpimport.Options
	o.exportDump = func(_ context.Context, opts dumpimport.Options) error {
		sawOpts = opts
		return os.WriteFile(opts.OutputPath, []byte("-- hashed\n"), 0o600)
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "dump", "staging", "--to", dest, "--sanitize"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !sawOpts.Sanitize {
		t.Fatal("exporter did not receive Sanitize")
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	if result["sanitized"] != true || result["label"] != dumpimport.SanitizedLabel {
		t.Fatalf("result = %#v", result)
	}
}
