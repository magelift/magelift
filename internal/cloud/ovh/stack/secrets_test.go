package stack

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cloud/ovh/resilience"
	"github.com/magelift/magelift/internal/platform"
)

func TestSecretsCRUDScopesOwnedPathsAndProtectsForeignSecrets(t *testing.T) {
	planned := applicationSecretsTestPlan()
	prefix, err := applicationSecretPrefix(planned)
	if err != nil {
		t.Fatal(err)
	}
	const value = "do-not-return-this-value"
	client := newFakeApplicationSecretClient()
	client.secrets[prefix+"/existing"] = fakeApplicationSecret{
		metadata: resilience.SecretMetadata{
			Path: prefix + "/existing", State: "active", CurrentVersion: 1,
			CustomMetadata: applicationSecretMetadata(prefix),
		},
		value: []byte("old-value"),
	}
	client.secrets[prefix+"/foreign"] = fakeApplicationSecret{
		metadata: resilience.SecretMetadata{
			Path: prefix + "/foreign", State: "active", CurrentVersion: 1,
			CustomMetadata: map[string]string{secretManagedByKey: "someone-else"},
		},
		value: []byte("foreign-value"),
	}
	client.secrets["magelift/application-secrets/another-stack/other"] = fakeApplicationSecret{
		metadata: resilience.SecretMetadata{
			Path: "magelift/application-secrets/another-stack/other", State: "active", CurrentVersion: 1,
			CustomMetadata: applicationSecretMetadata("magelift/application-secrets/another-stack"),
		},
		value: []byte("other-value"),
	}
	store := NewSecrets(func(context.Context, platform.PlannedStack) (resilience.SecretAPI, error) {
		return client, nil
	})

	listed, err := store.List(context.Background(), planned)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := secretNames(listed); strings.Join(got, ",") != "existing" {
		t.Fatalf("List() names = %v, want only owned existing secret", got)
	}

	if err := store.Set(context.Background(), planned, "existing", []byte(value)); err != nil {
		t.Fatalf("Set(existing) error = %v", err)
	}
	if client.versionCalls != 1 || client.createCalls != 0 {
		t.Fatalf("Set(existing) calls = create %d, version %d, want 0, 1", client.createCalls, client.versionCalls)
	}
	if got := string(client.secrets[prefix+"/existing"].value); got != value {
		t.Fatalf("updated value = %q, want test value", got)
	}

	if err := store.Set(context.Background(), planned, "new/nested", []byte(value)); err != nil {
		t.Fatalf("Set(new) error = %v", err)
	}
	if client.createCalls != 1 {
		t.Fatalf("Set(new) create calls = %d, want 1", client.createCalls)
	}
	created := client.secrets[prefix+"/new/nested"]
	if created.metadata.CustomMetadata[secretOwnerKey] != prefix || strings.Contains(strings.Join(metadataValues(created.metadata.CustomMetadata), " "), value) {
		t.Fatalf("created metadata leaked the secret value: %#v", created.metadata.CustomMetadata)
	}

	if err := store.Remove(context.Background(), planned, "existing"); err != nil {
		t.Fatalf("Remove(existing) error = %v", err)
	}
	if client.deleteCalls != 1 {
		t.Fatalf("Remove(existing) delete calls = %d, want 1", client.deleteCalls)
	}
	if err := store.Remove(context.Background(), planned, "foreign"); err == nil {
		t.Fatal("Remove(foreign) succeeded; foreign secret must not be touched")
	}
	if client.deleteCalls != 1 {
		t.Fatalf("Remove(foreign) delete calls = %d, want unchanged 1", client.deleteCalls)
	}
}

func TestSecretsRejectInvalidInputsBeforeConstructingClient(t *testing.T) {
	planned := applicationSecretsTestPlan()
	factoryCalls := 0
	store := NewSecrets(func(context.Context, platform.PlannedStack) (resilience.SecretAPI, error) {
		factoryCalls++
		return newFakeApplicationSecretClient(), nil
	})

	for _, test := range []struct {
		name string
		call func() error
	}{
		{name: "empty name", call: func() error { return store.Set(context.Background(), planned, "", []byte("value")) }},
		{name: "path traversal", call: func() error { return store.Remove(context.Background(), planned, "../foreign") }},
		{name: "empty value", call: func() error { return store.Set(context.Background(), planned, "name", nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err == nil {
				t.Fatal("operation succeeded for invalid input")
			}
		})
	}
	if factoryCalls != 0 {
		t.Fatalf("client factory calls = %d, want 0 for invalid input", factoryCalls)
	}
}

func TestModuleSecretsWiresOnlyTheProviderLocalFactory(t *testing.T) {
	client := newFakeApplicationSecretClient()
	calls := 0
	module := Module{NewApplicationSecretClient: func(_ context.Context, _ platform.PlannedStack) (resilience.SecretAPI, error) {
		calls++
		return client, nil
	}}
	store := module.Secrets()
	if store == nil {
		t.Fatal("Module.Secrets() returned nil")
	}
	if _, err := store.List(context.Background(), applicationSecretsTestPlan()); err != nil {
		t.Fatalf("wired Module.Secrets().List() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("factory calls = %d, want 1", calls)
	}
}

func TestModuleSecretsWithoutFactoryRemainsExplicitlyUnsupported(t *testing.T) {
	if _, err := (Module{}).Secrets().List(context.Background(), applicationSecretsTestPlan()); !errors.Is(err, platform.ErrNotSupported) {
		t.Fatalf("zero-value Module.Secrets().List() error = %v, want ErrNotSupported", err)
	}
}

func applicationSecretsTestPlan() Planned {
	return Planned{Spec: Spec{Identity: Identity{Project: "shop", Environment: "staging", Region: "EU-WEST-PAR"}}}
}

func secretNames(values []platform.SecretMeta) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Name)
	}
	sort.Strings(result)
	return result
}

func metadataValues(metadata map[string]string) []string {
	values := make([]string, 0, len(metadata))
	for _, value := range metadata {
		values = append(values, value)
	}
	return values
}

type fakeApplicationSecret struct {
	metadata resilience.SecretMetadata
	value    []byte
}

type fakeApplicationSecretClient struct {
	secrets      map[string]fakeApplicationSecret
	createCalls  int
	versionCalls int
	deleteCalls  int
}

func newFakeApplicationSecretClient() *fakeApplicationSecretClient {
	return &fakeApplicationSecretClient{secrets: make(map[string]fakeApplicationSecret)}
}

func (f *fakeApplicationSecretClient) Get(_ context.Context, path string) (resilience.SecretMetadata, error) {
	secret, ok := f.secrets[path]
	if !ok {
		return resilience.SecretMetadata{}, errors.New("not found")
	}
	return secret.metadata, nil
}

func (f *fakeApplicationSecretClient) List(_ context.Context) ([]resilience.SecretMetadata, error) {
	paths := make([]string, 0, len(f.secrets))
	for path := range f.secrets {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]resilience.SecretMetadata, 0, len(paths))
	for _, path := range paths {
		result = append(result, f.secrets[path].metadata)
	}
	return result, nil
}

func (f *fakeApplicationSecretClient) Access(_ context.Context, path string, _ uint32) ([]byte, error) {
	secret, ok := f.secrets[path]
	if !ok {
		return nil, errors.New("not found")
	}
	return append([]byte(nil), secret.value...), nil
}

func (f *fakeApplicationSecretClient) Delete(_ context.Context, path string) error {
	if _, ok := f.secrets[path]; !ok {
		return errors.New("not found")
	}
	f.deleteCalls++
	delete(f.secrets, path)
	return nil
}

func (f *fakeApplicationSecretClient) Create(_ context.Context, path string, metadata map[string]string, value []byte) (resilience.SecretMetadata, error) {
	if _, ok := f.secrets[path]; ok {
		return resilience.SecretMetadata{}, errors.New("already exists")
	}
	f.createCalls++
	created := resilience.SecretMetadata{Path: path, State: "active", CurrentVersion: 1, CustomMetadata: cloneMetadata(metadata)}
	f.secrets[path] = fakeApplicationSecret{metadata: created, value: append([]byte(nil), value...)}
	return created, nil
}

func (f *fakeApplicationSecretClient) CreateVersion(_ context.Context, path string, value []byte) (resilience.SecretMetadata, error) {
	secret, ok := f.secrets[path]
	if !ok {
		return resilience.SecretMetadata{}, errors.New("not found")
	}
	f.versionCalls++
	secret.metadata.CurrentVersion++
	secret.value = append([]byte(nil), value...)
	f.secrets[path] = secret
	return secret.metadata, nil
}

func cloneMetadata(metadata map[string]string) map[string]string {
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}
	return result
}

var _ resilience.SecretAPI = (*fakeApplicationSecretClient)(nil)
