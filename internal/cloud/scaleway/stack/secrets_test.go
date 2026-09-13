package stack

import (
	"context"
	"errors"
	"strings"
	"testing"

	scalewayresilience "github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	"github.com/magelift/magelift/internal/platform"
)

func TestSecretsListReturnsSortedMetadataOnly(t *testing.T) {
	t.Parallel()

	client := &fakeScalewaySecretAPI{items: []scalewayresilience.SecretMetadata{
		ownedScalewaySecret("id-b", "shop/z"),
		ownedScalewaySecret("id-a", "shop/a"),
		{ID: "foreign", Name: "shop/foreign", Tags: []string{"other-owner"}},
	}}
	got, err := (Secrets{client: client}).List(context.Background(), scalewayPlannedForSecrets())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "shop/a" || got[1].Name != "shop/z" {
		t.Fatalf("metadata = %#v", got)
	}
	if client.listCalls != 1 {
		t.Fatalf("List calls = %d, want 1", client.listCalls)
	}
}

func TestSecretsSetUpdatesExistingAndCreatesMissing(t *testing.T) {
	t.Parallel()

	t.Run("existing secret creates a new version", func(t *testing.T) {
		client := &fakeScalewaySecretAPI{items: []scalewayresilience.SecretMetadata{ownedScalewaySecret("existing", "shop/api")}}
		value := []byte("secret-value")
		if err := (Secrets{client: client}).Set(context.Background(), scalewayPlannedForSecrets(), "shop/api", value); err != nil {
			t.Fatal(err)
		}
		value[0] = 'X'
		if client.createdName != "" || len(client.versionIDs) != 1 || client.versionIDs[0] != "existing" || string(client.versionData[0]) != "secret-value" {
			t.Fatalf("existing update calls: created=%q versions=%v version_count=%d", client.createdName, client.versionIDs, len(client.versionData))
		}
	})

	t.Run("missing secret is created unprotected then versioned", func(t *testing.T) {
		client := &fakeScalewaySecretAPI{created: scalewayresilience.SecretMetadata{ID: "created", Name: "shop/api"}}
		if err := (Secrets{client: client}).Set(context.Background(), scalewayPlannedForSecrets(), "shop/api", []byte("secret-value")); err != nil {
			t.Fatal(err)
		}
		if client.createdName != "shop/api" || client.createdProtected || len(client.versionIDs) != 1 || client.versionIDs[0] != "created" {
			t.Fatalf("created secret = %#v", client)
		}
	})
}

func TestSecretsSetCleansUpIncompleteCreateWithoutLeakingValue(t *testing.T) {
	t.Parallel()

	client := &fakeScalewaySecretAPI{
		created:    scalewayresilience.SecretMetadata{ID: "created", Name: "shop/api"},
		versionErr: errors.New("request body included secret-value"),
	}
	err := (Secrets{client: client}).Set(context.Background(), scalewayPlannedForSecrets(), "shop/api", []byte("secret-value"))
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("Set error = %v; secret value must not cross the adapter error boundary", err)
	}
	if len(client.deleteIDs) != 1 || client.deleteIDs[0] != "created" {
		t.Fatalf("incomplete create cleanup = %#v", client.deleteIDs)
	}
}

func TestSecretsRemoveRefusesProtectedSecretAndDeletesUnprotectedSecret(t *testing.T) {
	t.Parallel()

	t.Run("protected secret fails closed", func(t *testing.T) {
		client := &fakeScalewaySecretAPI{items: []scalewayresilience.SecretMetadata{{ID: "protected", Name: "shop/api", Protected: true, Tags: ownedScalewaySecret("protected", "shop/api").Tags}}}
		err := (Secrets{client: client}).Remove(context.Background(), scalewayPlannedForSecrets(), "shop/api")
		if err == nil || !strings.Contains(err.Error(), "protected") || len(client.deleteIDs) != 0 {
			t.Fatalf("protected removal err=%v deletes=%v", err, client.deleteIDs)
		}
	})

	t.Run("unprotected secret is deleted", func(t *testing.T) {
		client := &fakeScalewaySecretAPI{items: []scalewayresilience.SecretMetadata{ownedScalewaySecret("owned", "shop/api")}}
		if err := (Secrets{client: client}).Remove(context.Background(), scalewayPlannedForSecrets(), "shop/api"); err != nil {
			t.Fatal(err)
		}
		if len(client.deleteIDs) != 1 || client.deleteIDs[0] != "owned" {
			t.Fatalf("deletes = %v", client.deleteIDs)
		}
	})
}

func TestSecretsRefusesForeignSameNameBeforeMutation(t *testing.T) {
	t.Parallel()

	client := &fakeScalewaySecretAPI{items: []scalewayresilience.SecretMetadata{{ID: "foreign", Name: "shop/api", Tags: []string{"other-owner"}}}}
	err := (Secrets{client: client}).Set(context.Background(), scalewayPlannedForSecrets(), "shop/api", []byte("secret-value"))
	if !errors.Is(err, ErrSecretOwnershipConflict) {
		t.Fatalf("Set error = %v, want ownership conflict", err)
	}
	if client.createCalls != 0 || len(client.versionIDs) != 0 || len(client.deleteIDs) != 0 {
		t.Fatalf("foreign secret caused provider mutation: %#v", client)
	}
}

func TestSecretsValidationAvoidsProviderCalls(t *testing.T) {
	t.Parallel()

	client := &fakeScalewaySecretAPI{}
	adapter := Secrets{client: client}
	if !errors.Is(adapter.Set(context.Background(), scalewayPlannedForSecrets(), "  ", []byte("value")), ErrSecretNameRequired) {
		t.Fatal("empty name must be rejected")
	}
	if !errors.Is(adapter.Set(context.Background(), scalewayPlannedForSecrets(), "name", nil), ErrSecretValueMissing) {
		t.Fatal("empty value must be rejected")
	}
	if client.listCalls != 0 || client.createCalls != 0 || len(client.versionIDs) != 0 {
		t.Fatalf("validation made provider calls: %#v", client)
	}
}

func TestModuleSecretsWiresTheActualPlatformPort(t *testing.T) {
	t.Parallel()

	port := Module{}.Secrets()
	if _, ok := port.(Secrets); !ok {
		t.Fatalf("Module.Secrets() = %T, want stack.Secrets", port)
	}
	if _, err := port.List(context.Background(), nil); err == nil || errors.Is(err, platform.ErrNotSupported) {
		t.Fatalf("invalid plan must be rejected by the provider adapter, got %v", err)
	}
}

func scalewayPlannedForSecrets() Planned {
	return Planned{Spec: scwDeploySpec()}
}

func ownedScalewaySecret(id, name string) scalewayresilience.SecretMetadata {
	planned := scalewayPlannedForSecrets()
	ownerTag, err := applicationSecretOwnerTag(planned)
	if err != nil {
		panic(err)
	}
	return scalewayresilience.SecretMetadata{ID: id, Name: name, Status: "ready", Tags: applicationSecretTags(ownerTag)}
}

type fakeScalewaySecretAPI struct {
	items            []scalewayresilience.SecretMetadata
	created          scalewayresilience.SecretMetadata
	versionErr       error
	createErr        error
	deleteErr        error
	listErr          error
	createdName      string
	createdProtected bool
	versionIDs       []string
	versionData      [][]byte
	deleteIDs        []string
	listCalls        int
	createCalls      int
}

func (f *fakeScalewaySecretAPI) Get(context.Context, string) (scalewayresilience.SecretMetadata, error) {
	return scalewayresilience.SecretMetadata{}, errors.New("not implemented in fake")
}

func (f *fakeScalewaySecretAPI) List(context.Context) ([]scalewayresilience.SecretMetadata, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	items := make([]scalewayresilience.SecretMetadata, len(f.items))
	copy(items, f.items)
	return items, nil
}

func (f *fakeScalewaySecretAPI) Access(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("not implemented in fake")
}

func (f *fakeScalewaySecretAPI) Delete(_ context.Context, id string) error {
	f.deleteIDs = append(f.deleteIDs, id)
	return f.deleteErr
}

func (f *fakeScalewaySecretAPI) Create(_ context.Context, name string, _ []string, protected bool) (scalewayresilience.SecretMetadata, error) {
	f.createCalls++
	f.createdName = name
	f.createdProtected = protected
	if f.createErr != nil {
		return scalewayresilience.SecretMetadata{}, f.createErr
	}
	created := f.created
	if created.ID == "" {
		created.ID = "created-secret"
	}
	if created.Name == "" {
		created.Name = name
	}
	if len(created.Tags) == 0 {
		created.Tags = applicationSecretTags(mustScalewayOwnerTagForTest())
	}
	return created, nil
}

func mustScalewayOwnerTagForTest() string {
	ownerTag, err := applicationSecretOwnerTag(scalewayPlannedForSecrets())
	if err != nil {
		panic(err)
	}
	return ownerTag
}

func (f *fakeScalewaySecretAPI) CreateVersion(_ context.Context, id string, data []byte) error {
	f.versionIDs = append(f.versionIDs, id)
	f.versionData = append(f.versionData, append([]byte(nil), data...))
	return f.versionErr
}

func (f *fakeScalewaySecretAPI) Protect(context.Context, string) (scalewayresilience.SecretMetadata, error) {
	return scalewayresilience.SecretMetadata{}, errors.New("not implemented in fake")
}

func (f *fakeScalewaySecretAPI) Unprotect(context.Context, string) (scalewayresilience.SecretMetadata, error) {
	return scalewayresilience.SecretMetadata{}, errors.New("not implemented in fake")
}
