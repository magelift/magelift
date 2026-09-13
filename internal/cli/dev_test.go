package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/localdev"
)

func TestDevInitCreatesComposeTemplate(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "local", "init"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(directory, localdev.ComposeFile)
	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "local.php.ini") || !strings.Contains(string(data), "MAGENTO_DC__OVERRIDE") || !strings.Contains(output.String(), "composeFile") {
		t.Fatalf("compose output = %s", output.String())
	}
	phpIniPath := filepath.Join(directory, ".magelift", localdev.LocalPHPIniFile)
	phpIni, err := os.ReadFile(phpIniPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(phpIni) != "" {
		t.Fatalf("local PHP ini = %q", phpIni)
	}
	envPath := filepath.Join(directory, ".magelift", localdev.LocalEnvFile)
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("local credentials mode = %o", info.Mode().Perm())
	}
}

func TestDevInitWritesConfiguredPHPSettings(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	contents := strings.Replace(starterConfig, "target:\n", "local:\n  phpSettings:\n    memory_limit: 1G\n    max_execution_time: \"180\"\ntarget:\n", 1)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "local", "init"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	phpIni, err := os.ReadFile(filepath.Join(directory, ".magelift", localdev.LocalPHPIniFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(phpIni) != "max_execution_time = 180\nmemory_limit = 1G\n" {
		t.Fatalf("local PHP ini = %q", phpIni)
	}
}

func TestDevSeedStartsAppAndKeepsPasswordOutOfCommandArguments(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "bin", "magento"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, ".magelift"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, localdev.ComposeFile), []byte(localdev.ComposeTemplate), 0o600); err != nil {
		t.Fatal(err)
	}
	password := "Alocaladminpassword123"
	var calls [][]string
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.getenv = func(key string) string {
		if key == "MAGELIFT_LOCAL_ADMIN_PASSWORD" {
			return password
		}
		return ""
	}
	o.runCompose = func(_ context.Context, _ string, _ []string, args []string, _, _ io.Writer) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "local", "seed"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0][len(calls[0])-2] != "-d" || calls[0][len(calls[0])-1] != "app" {
		t.Fatalf("Compose calls = %#v", calls)
	}
	for _, argument := range calls[1] {
		if strings.Contains(argument, password) {
			t.Fatalf("password leaked into Compose arguments: %#v", calls[1])
		}
	}
	credentials, err := os.ReadFile(filepath.Join(directory, ".magelift", localdev.LocalEnvFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(credentials) != "MAGELIFT_LOCAL_ADMIN_PASSWORD="+password+"\n" {
		t.Fatalf("credentials = %q", credentials)
	}
	if strings.Contains(output.String(), password) {
		t.Fatalf("password leaked into output: %s", output.String())
	}
}

func TestDevSeedIsIdempotentWhenMagentoIsInstalled(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"bin/magento", ".magelift/compose.local.yml", "app/etc/env.php"} {
		full := filepath.Join(directory, relative)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("installed"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	called := false
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.runCompose = func(context.Context, string, []string, []string, io.Writer, io.Writer) error {
		called = true
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "local", "seed"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("seed started Compose for an installed Magento project")
	}
}

func TestDevSeedRequiresExplicitPassword(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"bin/magento", localdev.ComposeFile} {
		full := filepath.Join(directory, relative)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(localdev.ComposeTemplate), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "local", "seed"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "MAGELIFT_LOCAL_ADMIN_PASSWORD") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
}

func TestDevUpUsesProjectRootAndArgumentVector(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(directory, localdev.ComposeFile)
	if err := os.MkdirAll(filepath.Dir(composePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composePath, []byte(localdev.ComposeTemplate), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotDir string
	var gotEnv, gotArgs []string
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.runCompose = func(_ context.Context, dir string, env, args []string, _, _ io.Writer) error {
		gotDir, gotEnv, gotArgs = dir, append([]string(nil), env...), append([]string(nil), args...)
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "local", "up", "--service", "database"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotDir != directory || !reflect.DeepEqual(gotEnv, []string{"MAGELIFT_PROJECT_ROOT=" + directory}) || !reflect.DeepEqual(gotArgs, []string{"compose", "-f", ".magelift/compose.local.yml", "--project-name", "magelift-example-shop", "up", "-d", "database"}) {
		t.Fatalf("dir=%q env=%#v args=%#v", gotDir, gotEnv, gotArgs)
	}
}

func TestDevResetRequiresApproval(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "local", "reset"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
}

func TestHelpListsLocalNotDev(t *testing.T) {
	var output bytes.Buffer
	command := newCommand(&output, &output, nil)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	if !strings.Contains(help, "  local ") {
		t.Fatalf("help missing local command:\n%s", help)
	}
	if strings.Contains(help, "  dev ") {
		t.Fatalf("help still lists dev:\n%s", help)
	}
}

func TestDevIsNotACommand(t *testing.T) {
	var output bytes.Buffer
	command := newCommand(&output, io.Discard, nil)
	command.SetArgs([]string{"dev"})
	err := command.Execute()
	if err == nil {
		t.Fatal("expected unknown command")
	}
	message := err.Error() + output.String()
	if !strings.Contains(message, "unknown command") {
		t.Fatalf("error=%v output=%s", err, output.String())
	}
}
