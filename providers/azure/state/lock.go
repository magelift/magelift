// Package state coordinates Azure Blob backed environment operations.
package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/streaming"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/magelift/magelift/internal/platform"
)

var (
	ErrLocked       = errors.New("deployment lock is held")
	ErrNotLocked    = errors.New("deployment lock is not held")
	ErrLockReplaced = errors.New("deployment lock was replaced")
	stableName      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

type Manager struct {
	client                               *azblob.Client
	container, key, project, environment string
}

type Handle struct {
	manager *Manager
	etag    azcore.ETag
	info    platform.LockInfo
}

// ETag returns the object version captured by Acquire for a stateless release
// receipt. The value is only useful with the manager that owns the Blob.
func (h *Handle) ETag() azcore.ETag {
	if h == nil {
		return ""
	}
	return h.etag
}

func NewManager(client *azblob.Client, container, project, environment string) (*Manager, error) {
	if client == nil || strings.TrimSpace(container) == "" || !stableName.MatchString(project) || !stableName.MatchString(environment) {
		return nil, errors.New("Azure state lock requires a Blob client, container, and stable project and environment names")
	}
	return &Manager{client: client, container: container, key: "locks/azure/" + project + "/" + environment + ".json", project: project, environment: environment}, nil
}

func (m *Manager) Acquire(ctx context.Context, owner string) (*Handle, error) {
	if m == nil || m.client == nil || strings.TrimSpace(owner) == "" {
		return nil, errors.New("Azure state lock manager and owner are required")
	}
	info := platform.LockInfo{Project: m.project, Environment: m.environment, Owner: owner, AcquiredAt: time.Now().UTC()}
	data, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	star := azcore.ETag("*")
	object := m.client.ServiceClient().NewContainerClient(m.container).NewBlockBlobClient(m.key)
	response, err := object.Upload(ctx, streaming.NopCloser(bytes.NewReader(data)), &blockblob.UploadOptions{
		AccessConditions: &blob.AccessConditions{ModifiedAccessConditions: &blob.ModifiedAccessConditions{IfNoneMatch: &star}},
	})
	if err != nil {
		var responseError *azcore.ResponseError
		if errors.As(err, &responseError) && (responseError.StatusCode == 409 || responseError.StatusCode == 412) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("acquire Azure deployment lock: %w", err)
	}
	if response.ETag == nil {
		return nil, errors.New("Azure did not return a lock ETag; inspect the lock before retrying")
	}
	return &Handle{manager: m, etag: *response.ETag, info: info}, nil
}

func (m *Manager) inspect(ctx context.Context) (platform.LockInfo, azcore.ETag, error) {
	if m == nil || m.client == nil {
		return platform.LockInfo{}, "", errors.New("Azure state lock manager is required")
	}
	response, err := m.client.DownloadStream(ctx, m.container, m.key, nil)
	if err != nil {
		var responseError *azcore.ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == 404 && responseError.ErrorCode == "BlobNotFound" {
			return platform.LockInfo{}, "", ErrNotLocked
		}
		return platform.LockInfo{}, "", fmt.Errorf("read Azure deployment lock: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return platform.LockInfo{}, "", err
	}
	var info platform.LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return platform.LockInfo{}, "", errors.New("Azure deployment lock contains invalid metadata")
	}
	if info.Project != m.project || info.Environment != m.environment || response.ETag == nil {
		return platform.LockInfo{}, "", errors.New("Azure deployment lock identity or ETag is invalid")
	}
	return info, *response.ETag, nil
}

func (m *Manager) Status(ctx context.Context) (platform.LockInfo, error) {
	info, _, err := m.inspect(ctx)
	return info, err
}

// BackendIdentity binds receipts to the account endpoint and container while
// omitting URL credentials such as SAS query parameters.
func (m *Manager) BackendIdentity() (string, error) {
	if m == nil || m.client == nil {
		return "", errors.New("Azure state lock manager is required")
	}
	endpoint, err := url.Parse(m.client.URL())
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return "", errors.New("Azure Blob service endpoint is invalid")
	}
	endpoint.User = nil
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return strings.TrimRight(endpoint.String(), "/") + "/" + m.container, nil
}

// ReleaseReceipt conditionally deletes the Blob version captured by Acquire.
// It does not depend on the process that created the receipt remaining alive.
func (m *Manager) ReleaseReceipt(ctx context.Context, etag azcore.ETag) error {
	if etag == "" {
		return ErrLockReplaced
	}
	return m.delete(ctx, etag)
}

func (m *Manager) delete(ctx context.Context, etag azcore.ETag) error {
	_, err := m.client.DeleteBlob(ctx, m.container, m.key, &azblob.DeleteBlobOptions{
		AccessConditions: &blob.AccessConditions{ModifiedAccessConditions: &blob.ModifiedAccessConditions{IfMatch: &etag}},
	})
	if err != nil {
		var responseError *azcore.ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode == 404 && responseError.ErrorCode == "BlobNotFound" {
				return ErrNotLocked
			}
			if responseError.StatusCode == 412 {
				return ErrLockReplaced
			}
		}
		return fmt.Errorf("release Azure deployment lock: %w", err)
	}
	return nil
}

func (h *Handle) Release(ctx context.Context) error {
	if h == nil || h.manager == nil {
		return errors.New("Azure state lock handle is required")
	}
	if err := h.manager.delete(ctx, h.etag); err != nil {
		return err
	}
	h.manager = nil
	return nil
}

// Unlock releases the currently observed lock using its ETag. This is intended
// for an explicit operator recovery after an interrupted operation.
func (m *Manager) Unlock(ctx context.Context) (platform.LockInfo, error) {
	info, etag, err := m.inspect(ctx)
	if err != nil {
		return platform.LockInfo{}, err
	}
	if err := m.delete(ctx, etag); err != nil {
		return platform.LockInfo{}, err
	}
	return info, nil
}
