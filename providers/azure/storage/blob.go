// Package storage implements Azure Blob operations used by MageLift.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// BlobStore operates on one existing Azure Blob container. The container is
// provisioned separately, so data operations do not acquire ownership of it.
type BlobStore struct {
	client    *azblob.Client
	container string
}

// NewBlobStore connects using the caller's Azure identity. endpoint must be
// the storage account endpoint, and container is the existing container name.
func NewBlobStore(endpoint, container string, credential azcore.TokenCredential) (*BlobStore, error) {
	if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(container) == "" {
		return nil, errors.New("Azure Blob endpoint and container are required")
	}
	if credential == nil {
		return nil, errors.New("Azure Blob credential is required")
	}
	client, err := azblob.NewClient(endpoint, credential, nil)
	if err != nil {
		return nil, fmt.Errorf("create Azure Blob client: %w", err)
	}
	return &BlobStore{client: client, container: container}, nil
}

// NewBlobStoreWithConnectionString is useful for a storage emulator or an
// explicitly supplied shared-key connection string.
func NewBlobStoreWithConnectionString(connectionString, container string) (*BlobStore, error) {
	if strings.TrimSpace(connectionString) == "" || strings.TrimSpace(container) == "" {
		return nil, errors.New("Azure Blob connection string and container are required")
	}
	client, err := azblob.NewClientFromConnectionString(connectionString, nil)
	if err != nil {
		return nil, fmt.Errorf("create Azure Blob client: %w", err)
	}
	return &BlobStore{client: client, container: container}, nil
}

func (s *BlobStore) Put(ctx context.Context, name string, body io.Reader) error {
	if s == nil || s.client == nil || strings.TrimSpace(name) == "" || body == nil {
		return errors.New("Azure Blob store, name, and content are required")
	}
	if _, err := s.client.UploadStream(ctx, s.container, name, body, nil); err != nil {
		return fmt.Errorf("upload Azure blob %q: %w", name, err)
	}
	return nil
}

// Open returns a stream which the caller must close.
func (s *BlobStore) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	if s == nil || s.client == nil || strings.TrimSpace(name) == "" {
		return nil, errors.New("Azure Blob store and name are required")
	}
	response, err := s.client.DownloadStream(ctx, s.container, name, nil)
	if err != nil {
		return nil, fmt.Errorf("open Azure blob %q: %w", name, err)
	}
	return response.Body, nil
}

func (s *BlobStore) Delete(ctx context.Context, name string) error {
	if s == nil || s.client == nil || strings.TrimSpace(name) == "" {
		return errors.New("Azure Blob store and name are required")
	}
	if _, err := s.client.DeleteBlob(ctx, s.container, name, nil); err != nil {
		return fmt.Errorf("delete Azure blob %q: %w", name, err)
	}
	return nil
}
