// Package secrets adapts GCP Secret Manager to the Magento Secrets port.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
)

type API interface {
	List(ctx context.Context, project string) ([]Meta, error)
	Set(ctx context.Context, project, name string, value []byte) error
	Remove(ctx context.Context, project, name string) error
}

type Meta struct {
	Name string
}

type Store struct {
	client API
}

func NewStoreFromClient(client API) *Store {
	return &Store{client: client}
}

func NewStore(ctx context.Context) (*Store, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create Secret Manager client: %w", err)
	}
	return NewStoreFromClient(smClient{client: client}), nil
}

func (s *Store) List(ctx context.Context, project string) ([]Meta, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("secret store is required")
	}
	return s.client.List(ctx, project)
}

func (s *Store) Set(ctx context.Context, project, name string, value []byte) error {
	if s == nil || s.client == nil {
		return errors.New("secret store is required")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("secret name is required")
	}
	return s.client.Set(ctx, project, name, value)
}

func (s *Store) Remove(ctx context.Context, project, name string) error {
	if s == nil || s.client == nil {
		return errors.New("secret store is required")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("secret name is required")
	}
	return s.client.Remove(ctx, project, name)
}

type smClient struct {
	client *secretmanager.Client
}

func (s smClient) List(ctx context.Context, project string) ([]Meta, error) {
	it := s.client.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{
		Parent: "projects/" + project,
	})
	var out []Meta
	for {
		secret, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list secrets: %w", err)
		}
		name := secret.GetName()
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		out = append(out, Meta{Name: name})
	}
	return out, nil
}

func (s smClient) Set(ctx context.Context, project, name string, value []byte) error {
	parent := "projects/" + project
	secretName := parent + "/secrets/" + name
	_, err := s.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: secretName})
	if err != nil {
		_, createErr := s.client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
			Parent:   parent,
			SecretId: name,
			Secret: &secretmanagerpb.Secret{
				Replication: &secretmanagerpb.Replication{
					Replication: &secretmanagerpb.Replication_Automatic_{
						Automatic: &secretmanagerpb.Replication_Automatic{},
					},
				},
			},
		})
		if createErr != nil {
			return fmt.Errorf("create secret: %w", createErr)
		}
	}
	_, err = s.client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent: secretName,
		Payload: &secretmanagerpb.SecretPayload{Data: value},
	})
	if err != nil {
		return fmt.Errorf("add secret version: %w", err)
	}
	return nil
}

func (s smClient) Remove(ctx context.Context, project, name string) error {
	err := s.client.DeleteSecret(ctx, &secretmanagerpb.DeleteSecretRequest{
		Name: "projects/" + project + "/secrets/" + name,
	})
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}
