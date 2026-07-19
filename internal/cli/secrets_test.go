package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	awssecrets "github.com/acourtiol/magelift/internal/cloud/aws/secrets"
)

type fakeSecretStore struct {
	items       []awssecrets.Secret
	setName     string
	setValue    []byte
	removed     string
	setCalls    int
	listCalls   int
	removeCalls int
}

func (s *fakeSecretStore) List(context.Context) ([]awssecrets.Secret, error) {
	s.listCalls++
	return append([]awssecrets.Secret(nil), s.items...), nil
}

func (s *fakeSecretStore) Set(_ context.Context, name string, value []byte) error {
	s.setCalls++
	s.setName = name
	s.setValue = append([]byte(nil), value...)
	return nil
}

func (s *fakeSecretStore) Remove(_ context.Context, name string) error {
	s.removeCalls++
	s.removed = name
	return nil
}

func secretTestOptions(out *bytes.Buffer, store *fakeSecretStore, configPath string) *options {
	o := testOptions(out, &fakeTerminal{interactive: false})
	o.configPath = configPath
	o.newSecrets = func(context.Context, string) (secretStore, error) { return store, nil }
	return o
}

func writeTestConfig(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSecretSetReadsOnlyFromExplicitStdin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	writeTestConfig(t, path)
	store := &fakeSecretStore{}
	var out bytes.Buffer
	o := secretTestOptions(&out, store, path)
	cmd := newCommandWithOptions(o)
	cmd.SetIn(strings.NewReader("value-from-stdin\n"))
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "secret", "set", "shop/api", "--value-stdin"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if store.setCalls != 1 || store.setName != "shop/api" || string(store.setValue) != "value-from-stdin\n" {
		t.Fatalf("unexpected set call: %+v", store)
	}
	if strings.Contains(out.String(), "value-from-stdin") {
		t.Fatalf("secret value leaked in output: %s", out.String())
	}
	if !strings.Contains(out.String(), `"updated": true`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestSecretSetRequiresValueStdin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	writeTestConfig(t, path)
	store := &fakeSecretStore{}
	var out bytes.Buffer
	o := secretTestOptions(&out, store, path)
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "secret", "set", "shop/api"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "value-stdin") {
		t.Fatalf("unexpected error/code: %v/%d", err, ExitCode(err))
	}
	if store.setCalls != 0 {
		t.Fatal("secret store was called")
	}
}

func TestSecretListUsesStructuredMetadataOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	writeTestConfig(t, path)
	store := &fakeSecretStore{items: []awssecrets.Secret{{Name: "shop/api", ARN: "arn:secret"}}}
	var out bytes.Buffer
	o := secretTestOptions(&out, store, path)
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "secret", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "shop/api"`) || !strings.Contains(out.String(), `"arn": "arn:secret"`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestSecretRemoveRequiresConfirmation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	writeTestConfig(t, path)
	store := &fakeSecretStore{}
	var out bytes.Buffer
	o := secretTestOptions(&out, store, path)
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "secret", "remove", "shop/api"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("unexpected error/code: %v/%d", err, ExitCode(err))
	}

	out.Reset()
	cmd = newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--yes", "--output", "json", "secret", "remove", "shop/api"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if store.removeCalls != 1 || store.removed != "shop/api" || !strings.Contains(out.String(), `"recoveryWindowDays": 30`) {
		t.Fatalf("unexpected removal: calls=%d name=%q output=%s", store.removeCalls, store.removed, out.String())
	}
}
