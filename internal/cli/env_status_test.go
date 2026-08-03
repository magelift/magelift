package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/acourtiol/magelift/internal/seeddump"
)

func TestEnvStatusMergesSeedDumpPathAndImportedJournal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dumpPath := filepath.Join(dir, "seed.sql")
	if err := os.WriteFile(dumpPath, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var createOut bytes.Buffer
	create := newCommandWithOptions(testOptions(&createOut, &fakeTerminal{interactive: false}))
	create.SetArgs([]string{"--config", path, "--output", "json", "env", "create", "preview-status", "--dump", dumpPath})
	if err := create.Execute(); err != nil {
		t.Fatal(err)
	}

	store, err := seeddump.New(dir, "preview-status")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImported(context.Background()); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "status", "preview-status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode status: %v\n%s", err, out.String())
	}
	if result["environment"] != "preview-status" {
		t.Fatalf("environment = %#v", result["environment"])
	}
	if result["seedDump"] != dumpPath {
		t.Fatalf("seedDump = %#v", result["seedDump"])
	}
	if result["seedDumpStatus"] != seeddump.StatusImported {
		t.Fatalf("seedDumpStatus = %#v", result["seedDumpStatus"])
	}
	if reason, ok := result["seedDumpReason"]; ok && reason != nil && reason != "" {
		t.Fatalf("imported should not surface reason: %#v", reason)
	}
}

func TestEnvStatusMergesFailedReasonFromJournal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	dumpPath := filepath.Join(dir, "seed.sql")
	if err := os.WriteFile(dumpPath, []byte("-- fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var createOut bytes.Buffer
	create := newCommandWithOptions(testOptions(&createOut, &fakeTerminal{interactive: false}))
	create.SetArgs([]string{"--config", path, "--output", "json", "env", "create", "preview-fail", "--dump", dumpPath})
	if err := create.Execute(); err != nil {
		t.Fatal(err)
	}

	store, err := seeddump.New(dir, "preview-fail")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkFailed(context.Background(), "mysql refused connection"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommandWithOptions(testOptions(&out, &fakeTerminal{interactive: false}))
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "status", "preview-fail"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode status: %v\n%s", err, out.String())
	}
	if result["seedDump"] != dumpPath {
		t.Fatalf("seedDump = %#v", result["seedDump"])
	}
	if result["seedDumpStatus"] != seeddump.StatusFailed {
		t.Fatalf("seedDumpStatus = %#v", result["seedDumpStatus"])
	}
	if result["seedDumpReason"] != "mysql refused connection" {
		t.Fatalf("seedDumpReason = %#v", result["seedDumpReason"])
	}
}

func TestEnvStatusOmitsSeedFieldsWithoutSeedDump(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommandWithOptions(testOptions(&out, &fakeTerminal{interactive: false}))
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "status", "staging"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode status: %v\n%s", err, out.String())
	}
	if result["environment"] != "staging" {
		t.Fatalf("environment = %#v", result["environment"])
	}
	if _, ok := result["seedDump"]; ok {
		t.Fatalf("seedDump present without config path: %#v", result["seedDump"])
	}
	if _, ok := result["seedDumpStatus"]; ok {
		t.Fatalf("seedDumpStatus present without config path: %#v", result["seedDumpStatus"])
	}
}

func TestEnvStatusReportsPathWhenJournalMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	contents := `schemaVersion: 1
project:
  name: example-shop
application:
  edition: open-source
  version: 2.4.9
  mode: integrated
build:
  php: "8.5"
target:
  provider: aws
  runtime: ecs-fargate
defaults:
  region: eu-west-3
  preset: preview
environments:
  orphan-dump:
    account: "123456789012"
    seedDump: /tmp/orphan.sql
extensions: {}
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommandWithOptions(testOptions(&out, &fakeTerminal{interactive: false}))
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "status", "orphan-dump"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode status: %v\n%s", err, out.String())
	}
	if result["seedDump"] != "/tmp/orphan.sql" {
		t.Fatalf("seedDump = %#v", result["seedDump"])
	}
	if result["seedDumpStatus"] != "unavailable" {
		t.Fatalf("seedDumpStatus without journal = %#v (want unavailable)", result["seedDumpStatus"])
	}
}
