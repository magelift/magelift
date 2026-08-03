package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/config"
)

func TestForeignMagentoAppRejectedAsConfig(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported", ".magento.app.yaml")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".magento.app.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, loadErr := config.Load(data)
	if loadErr == nil {
		t.Fatal("config.Load accepted .magento.app.yaml")
	}
	if !foreignSchemaError(loadErr) {
		t.Fatalf("Load error not clearly foreign-schema: %v", loadErr)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", path, "config", "validate"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("config validate accepted .magento.app.yaml as --config")
	}
	if ExitCode(err) == 0 {
		t.Fatalf("exit code = 0")
	}
	if !foreignSchemaError(err) {
		t.Fatalf("validate error not clearly foreign-schema: %v", err)
	}
}

func TestForeignPlatformAppRejectedAsConfig(t *testing.T) {
	platform := []byte("name: app\ntype: php:8.5\nrelationships:\n  database: \"mysql:mysql\"\n")
	_, loadErr := config.Load(platform)
	if loadErr == nil {
		t.Fatal("config.Load accepted .platform.app.yaml shape")
	}
	if !foreignSchemaError(loadErr) {
		t.Fatalf("Load error not clearly foreign-schema: %v", loadErr)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, ".platform.app.yaml")
	if err := os.WriteFile(path, platform, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", path, "config", "validate"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("config validate accepted .platform.app.yaml")
	}
	if !foreignSchemaError(err) {
		t.Fatalf("validate error not clearly foreign-schema: %v", err)
	}
}

func TestStarterConfigStillLoadsAfterForeignGuard(t *testing.T) {
	file, err := config.Load([]byte(starterConfig))
	if err != nil {
		t.Fatalf("starter Load: %v", err)
	}
	if len(file.Environments()) == 0 {
		t.Fatal("expected environments")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", path, "config", "validate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("starter validate: %v", err)
	}
}

func foreignSchemaError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "schemaversion") ||
		strings.Contains(msg, "magento.app") ||
		strings.Contains(msg, "platform.app") ||
		strings.Contains(msg, "paas") ||
		strings.Contains(msg, "not a magelift")
}
