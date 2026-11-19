// Package secrets implements access to Azure Key Vault secrets.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

type Store struct{ client *azsecrets.Client }

// NewStore uses Azure's challenge authentication and verifies the vault URL.
func NewStore(vaultURL string, credential azcore.TokenCredential) (*Store, error) {
	if strings.TrimSpace(vaultURL) == "" || credential == nil {
		return nil, errors.New("Azure Key Vault URL and credential are required")
	}
	client, err := azsecrets.NewClient(vaultURL, credential, nil)
	if err != nil {
		return nil, fmt.Errorf("create Azure Key Vault client: %w", err)
	}
	return NewStoreWithClient(client)
}

// NewStoreWithClient accepts a preconfigured SDK client, for example one
// configured with a transport for an isolated test emulator.
func NewStoreWithClient(client *azsecrets.Client) (*Store, error) {
	if client == nil {
		return nil, errors.New("Azure Key Vault client is required")
	}
	return &Store{client: client}, nil
}

func (s *Store) Get(ctx context.Context, name string) ([]byte, error) {
	if s == nil || s.client == nil || strings.TrimSpace(name) == "" {
		return nil, errors.New("Azure Key Vault store and secret name are required")
	}
	response, err := s.client.GetSecret(ctx, name, "", nil)
	if err != nil {
		return nil, fmt.Errorf("read Azure Key Vault secret %q: %w", name, err)
	}
	if response.Value == nil {
		return nil, fmt.Errorf("Azure Key Vault secret %q has no value", name)
	}
	return []byte(*response.Value), nil
}

func (s *Store) Set(ctx context.Context, name string, value []byte) error {
	if s == nil || s.client == nil || strings.TrimSpace(name) == "" {
		return errors.New("Azure Key Vault store and secret name are required")
	}
	if !utf8.Valid(value) {
		return errors.New("Azure Key Vault secret must be UTF-8")
	}
	text := string(value)
	if _, err := s.client.SetSecret(ctx, name, azsecrets.SetSecretParameters{Value: &text}, nil); err != nil {
		return fmt.Errorf("set Azure Key Vault secret %q: %w", name, err)
	}
	return nil
}

// Delete starts Key Vault's soft-delete lifecycle. Purge is a separate Azure
// operation and is intentionally not implied by a normal removal.
func (s *Store) Delete(ctx context.Context, name string) error {
	if s == nil || s.client == nil || strings.TrimSpace(name) == "" {
		return errors.New("Azure Key Vault store and secret name are required")
	}
	if _, err := s.client.DeleteSecret(ctx, name, nil); err != nil {
		return fmt.Errorf("delete Azure Key Vault secret %q: %w", name, err)
	}
	return nil
}
