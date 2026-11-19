package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/localdev"
)

func TestDevReconcilesGeneratedFilesAndRestoresMissingFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	write := func(body string) {
		t.Helper()
		if err := writeTestProjectFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(starterConfig)
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	if _, err := o.prepareLocalFiles(true); err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(starterConfig, "target:\n", "local:\n  phpSettings:\n    memory_limit: 1G\nservices:\n  search:\n    family: opensearch\n    version: '3.9.0'\n    image: example.com/search:custom\ntarget:\n", 1)
	write(updated)
	plan, err := o.prepareLocalFiles(true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Search.Image != "example.com/search:custom" {
		t.Fatalf("search image = %q", plan.Search.Image)
	}
	compose, err := os.ReadFile(filepath.Join(dir, localdev.ComposeFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "example.com/search:custom") {
		t.Fatal("compose did not refresh")
	}
	iniPath := filepath.Join(dir, ".magelift", localdev.LocalPHPIniFile)
	if ini, err := os.ReadFile(iniPath); err != nil || !strings.Contains(string(ini), "memory_limit = 1G") {
		t.Fatalf("ini = %q, %v", ini, err)
	}
	for _, file := range []string{iniPath, filepath.Join(dir, ".magelift", localdev.LocalEnvFile)} {
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := o.prepareLocalFiles(true); err != nil {
		t.Fatal(err)
	}
	if ini, err := os.ReadFile(iniPath); err != nil || !strings.Contains(string(ini), "memory_limit = 1G") {
		t.Fatalf("restored ini = %q, %v", ini, err)
	}
	info, err := os.Stat(filepath.Join(dir, ".magelift", localdev.LocalEnvFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials = %v, %v", info, err)
	}
}

func TestDevPreservesManualGeneratedFileEdits(t *testing.T) {
	for _, name := range []string{"compose.local.yml", localdev.LocalPHPIniFile} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "magelift.php")
			if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
				t.Fatal(err)
			}
			o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
			o.configPath, o.environment = path, "staging"
			if _, err := o.prepareLocalFiles(true); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, ".magelift", name)
			if err := os.WriteFile(file, []byte("manual edit"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := o.prepareLocalFiles(true); err == nil || !strings.Contains(err.Error(), "manual changes") {
				t.Fatalf("error = %v", err)
			}
			if got, _ := os.ReadFile(file); string(got) != "manual edit" {
				t.Fatalf("manual changes lost: %q", got)
			}
		})
	}
}

func TestDevGeneratedFileJournalRecoversPartialUpdate(t *testing.T) {
	dir := t.TempDir()
	old := map[string][]byte{"compose": []byte("old compose"), "ini": []byte("old ini")}
	if err := reconcileLocalGenerated(dir, old); err != nil {
		t.Fatal(err)
	}
	next := map[string][]byte{"compose": []byte("new compose"), "ini": []byte("new ini")}
	journal := map[string][]string{}
	for name, value := range next {
		journal[name] = []string{localContentHash(old[name]), localContentHash(value)}
	}
	data, _ := json.Marshal(journal)
	if err := os.WriteFile(filepath.Join(dir, "local-generated.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose"), next["compose"], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := reconcileLocalGenerated(dir, next); err != nil {
		t.Fatal(err)
	}
	for name, want := range next {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != string(want) {
			t.Fatalf("%s = %q, %v", name, got, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "compose"), old["compose"], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := reconcileLocalGenerated(dir, next); err == nil {
		t.Fatal("manual reversion accepted after completed update")
	}
}

func TestDevRetriesAfterCredentialsCreationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials := filepath.Join(dir, ".magelift", localdev.LocalEnvFile)
	if err := os.MkdirAll(credentials, 0o755); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	if _, err := o.prepareLocalFiles(true); err == nil || !strings.Contains(err.Error(), "regular credentials file") {
		t.Fatalf("error = %v", err)
	}
	if err := os.Remove(credentials); err != nil {
		t.Fatal(err)
	}
	if _, err := o.prepareLocalFiles(true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(credentials)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials = %v, %v", info, err)
	}
}

func TestLocalURLUsesConfiguredHTTPPort(t *testing.T) {
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	env := map[string]string{"MAGELIFT_LOCAL_APP_HTTP_PORT": "9080"}
	o.getenv = func(k string) string { return env[k] }
	if got := localURL(o); got != "http://localhost:9080/" {
		t.Fatal(got)
	}
	env["MAGELIFT_LOCAL_BASE_URL"] = "https://shop.example/"
	if got := localURL(o); got != "https://shop.example/" {
		t.Fatal(got)
	}
}

func TestDevUsesSelectedEnvironmentServiceImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	contents := strings.Replace(starterConfig, "target:\n", "services:\n  search:\n    family: opensearch\n    image: example.com/search:preview\ntarget:\n", 1)
	contents = strings.Replace(contents, "  staging:\n    inherits:", "  staging:\n    services:\n      search:\n        image: example.com/search:staging\n    inherits:", 1)
	if err := writeTestProjectFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "preview"
	plan, err := o.prepareLocalFiles(true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Search.Image != "example.com/search:preview" {
		t.Fatalf("preview search image = %q", plan.Search.Image)
	}
	o.environment = "staging"
	plan, err = o.prepareLocalFiles(true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Search.Image != "example.com/search:staging" {
		t.Fatalf("staging search image = %q", plan.Search.Image)
	}
	compose, err := os.ReadFile(filepath.Join(dir, localdev.ComposeFile))
	if err != nil || !strings.Contains(string(compose), "example.com/search:staging") {
		t.Fatalf("staging compose = %q, %v", compose, err)
	}
}

func TestDevInitCreatesComposeTemplate(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "local", "init"})
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
	path := filepath.Join(directory, "magelift.php")
	contents := strings.Replace(starterConfig, "target:\n", "local:\n  phpSettings:\n    memory_limit: 1G\n    max_execution_time: \"180\"\ntarget:\n", 1)
	if err := writeTestProjectFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "local", "init"})
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
	path := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(filepath.Join(directory, "bin", "magento"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, ".magelift"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(filepath.Join(directory, localdev.ComposeFile), []byte(localdev.ComposeTemplate), 0o600); err != nil {
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
	path := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"bin/magento", ".magelift/compose.local.yml", "app/etc/env.php"} {
		full := filepath.Join(directory, relative)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeTestProjectFile(full, []byte("installed"), 0o600); err != nil {
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
	path := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"bin/magento", localdev.ComposeFile} {
		full := filepath.Join(directory, relative)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeTestProjectFile(full, []byte(localdev.ComposeTemplate), 0o600); err != nil {
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
	path := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(directory, localdev.ComposeFile)
	if err := os.MkdirAll(filepath.Dir(composePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(composePath, []byte(localdev.ComposeTemplate), 0o600); err != nil {
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
	path := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
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

func TestHelpListsLocalAndDev(t *testing.T) {
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
	if !strings.Contains(help, "  dev ") {
		t.Fatalf("help omits dev:\n%s", help)
	}
}

func TestDevReportsMissingShop(t *testing.T) {
	var output bytes.Buffer
	command := newCommand(&output, io.Discard, nil)
	command.SetArgs([]string{"dev"})
	err := command.Execute()
	if err == nil {
		t.Fatal("expected a missing Magento shop error")
	}
	message := err.Error() + output.String()
	if !strings.Contains(message, "bin/magento") {
		t.Fatalf("error=%v output=%s", err, output.String())
	}
}
