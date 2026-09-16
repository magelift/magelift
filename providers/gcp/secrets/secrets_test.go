package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/secretref"
)

type fakeAPI struct {
	payload map[string][]byte
	err     error
}

func (f *fakeAPI) List(context.Context, string) ([]Meta, error) { return nil, nil }
func (f *fakeAPI) Set(context.Context, string, string, []byte) error {
	return nil
}
func (f *fakeAPI) Remove(context.Context, string, string) error { return nil }
func (f *fakeAPI) GetSecretValue(_ context.Context, name string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	value, ok := f.payload[name]
	if !ok {
		return nil, errors.New("secret not found")
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out, nil
}

func TestGetSecretValueReturnsPayload(t *testing.T) {
	t.Parallel()
	name := "projects/p/secrets/composer/versions/latest"
	want := []byte(`{"http-basic":{"repo.magento.com":{"username":"u","password":"p"}}}`)
	store := NewStoreFromClient(&fakeAPI{payload: map[string][]byte{name: want}})
	got, err := store.GetSecretValue(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("got=%q", got)
	}
}

func TestGetSecretValuePropagatesMissingAndErrors(t *testing.T) {
	t.Parallel()
	store := NewStoreFromClient(&fakeAPI{err: errors.New("access denied")})
	_, err := store.GetSecretValue(context.Background(), "projects/p/secrets/x/versions/1")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("unexpected error: %v", err)
	}
	empty := NewStoreFromClient(&fakeAPI{payload: map[string][]byte{}})
	_, err = empty.GetSecretValue(context.Background(), "projects/p/secrets/missing/versions/latest")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected missing error: %v", err)
	}
	if _, err := NewStoreFromClient(&fakeAPI{}).GetSecretValue(context.Background(), "  "); !errors.Is(err, ErrSecretNameRequired) {
		t.Fatalf("empty name error=%v", err)
	}
}

func TestGCPSecretManagerProviderAdapterResolves(t *testing.T) {
	t.Parallel()
	name := "projects/p/secrets/composer/versions/latest"
	payload := []byte(`{"bearer":{"example.invalid":"token"}}`)
	store := NewStoreFromClient(&fakeAPI{payload: map[string][]byte{name: payload}})
	value, err := (secretref.Resolver{GCPSecretManager: store}).Resolve(context.Background(), secretref.Reference{
		Kind: secretref.GCPSecretManager,
		ID:   name,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != string(payload) {
		t.Fatalf("value=%q", value)
	}
}
