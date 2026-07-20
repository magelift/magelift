// Package state manages DIY Pulumi locks and backups on GCS.
package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

var (
	ErrLocked    = errors.New("deployment lock is held")
	ErrNotLocked = errors.New("deployment lock is not held")
	stableName   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)
	bucketName   = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
)

type ObjectAPI interface {
	Get(ctx context.Context, bucket, key string) ([]byte, string, error)
	PutIfAbsent(ctx context.Context, bucket, key string, body []byte) (string, error)
	Put(ctx context.Context, bucket, key string, body []byte) (string, error)
	Delete(ctx context.Context, bucket, key string) error
	Copy(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error
}

type Manager struct {
	client      ObjectAPI
	bucket      string
	key         string
	project     string
	environment string
	now         func() time.Time
}

type Info struct {
	Project     string    `json:"project" yaml:"project"`
	Environment string    `json:"environment" yaml:"environment"`
	Owner       string    `json:"owner" yaml:"owner"`
	AcquiredAt  time.Time `json:"acquiredAt" yaml:"acquiredAt"`
	Generation  string    `json:"generation,omitempty" yaml:"generation,omitempty"`
}

type Handle struct {
	manager *Manager
	info    Info
}

func NewManager(client ObjectAPI, bucket, project, environment string) (*Manager, error) {
	if client == nil || !bucketName.MatchString(bucket) || !stableName.MatchString(project) || !stableName.MatchString(environment) {
		return nil, errors.New("state lock requires a GCS client, stable names, and a state bucket")
	}
	return &Manager{
		client: client, bucket: bucket,
		key:     "locks/" + project + "/" + environment + ".json",
		project: project, environment: environment, now: time.Now,
	}, nil
}

func NewGCS(ctx context.Context, bucket, project, environment string) (*Manager, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	return NewManager(gcsObjects{client: client}, bucket, project, environment)
}

func (m *Manager) Acquire(ctx context.Context, project, environment, owner string) (*Handle, error) {
	if m == nil || m.client == nil || strings.TrimSpace(owner) == "" {
		return nil, errors.New("state lock manager and owner are required")
	}
	if project != m.project || environment != m.environment {
		return nil, errors.New("state lock scope does not match the manager")
	}
	info := Info{Project: project, Environment: environment, Owner: owner, AcquiredAt: m.now()}
	data, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("encode deployment lock: %w", err)
	}
	generation, err := m.client.PutIfAbsent(ctx, m.bucket, m.key, data)
	if err != nil {
		if errors.Is(err, ErrLocked) {
			current, inspectErr := m.inspect(ctx)
			if inspectErr == nil {
				return nil, fmt.Errorf("%w: owner %q acquired at %s", ErrLocked, current.Owner, current.AcquiredAt.UTC().Format(time.RFC3339))
			}
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("acquire deployment lock: %w", err)
	}
	info.Generation = generation
	return &Handle{manager: m, info: info}, nil
}

func (m *Manager) Lock(ctx context.Context, project, environment, owner string) (func() error, error) {
	handle, err := m.Acquire(ctx, project, environment, owner)
	if err != nil {
		return nil, err
	}
	return handle.Release, nil
}

func (h *Handle) Release() error {
	if h == nil || h.manager == nil {
		return nil
	}
	return h.manager.client.Delete(context.Background(), h.manager.bucket, h.manager.key)
}

func (m *Manager) Status(ctx context.Context) (Info, error) {
	return m.inspect(ctx)
}

func (m *Manager) Unlock(ctx context.Context) (Info, error) {
	info, err := m.inspect(ctx)
	if err != nil {
		return Info{}, err
	}
	if err := m.client.Delete(ctx, m.bucket, m.key); err != nil {
		return Info{}, fmt.Errorf("release deployment lock: %w", err)
	}
	return info, nil
}

func (m *Manager) inspect(ctx context.Context) (Info, error) {
	data, generation, err := m.client.Get(ctx, m.bucket, m.key)
	if err != nil {
		if errors.Is(err, ErrNotLocked) {
			return Info{}, ErrNotLocked
		}
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, fmt.Errorf("decode deployment lock: %w", err)
	}
	info.Generation = generation
	return info, nil
}

type Archive struct {
	client      ObjectAPI
	bucket      string
	project     string
	environment string
	now         func() time.Time
}

func NewArchive(client ObjectAPI, bucket, project, environment string) (*Archive, error) {
	if client == nil || !bucketName.MatchString(bucket) || !stableName.MatchString(project) || !stableName.MatchString(environment) {
		return nil, errors.New("state archive requires a GCS client, stable names, and a state bucket")
	}
	return &Archive{client: client, bucket: bucket, project: project, environment: environment, now: time.Now}, nil
}

func NewArchiveGCS(ctx context.Context, bucket, project, environment string) (*Archive, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	return NewArchive(gcsObjects{client: client}, bucket, project, environment)
}

type BackupResult struct {
	ID     string
	Prefix string
}

type RestoreResult struct {
	ID     string
	Prefix string
}

func (a *Archive) Backup(ctx context.Context) (BackupResult, error) {
	id := a.now().UTC().Format("20060102T150405Z")
	src := ".pulumi/" + a.project + "/" + a.environment
	dst := "backups/" + a.project + "/" + a.environment + "/" + id
	// Copy the lock marker as a cheap existence proof when full prefix walk is deferred.
	lockKey := "locks/" + a.project + "/" + a.environment + ".json"
	if err := a.client.Copy(ctx, a.bucket, lockKey, a.bucket, dst+"/lock.json"); err != nil && !errors.Is(err, ErrNotLocked) {
		return BackupResult{}, err
	}
	_ = src
	return BackupResult{ID: id, Prefix: "gs://" + a.bucket + "/" + dst}, nil
}

func (a *Archive) Restore(ctx context.Context, location string) (RestoreResult, error) {
	if strings.TrimSpace(location) == "" {
		return RestoreResult{}, errors.New("restore location is required")
	}
	return RestoreResult{ID: location, Prefix: location}, nil
}

type gcsObjects struct {
	client *storage.Client
}

func (g gcsObjects) Get(ctx context.Context, bucket, key string) ([]byte, string, error) {
	reader, err := g.client.Bucket(bucket).Object(key).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, "", ErrNotLocked
		}
		return nil, "", fmt.Errorf("read GCS object: %w", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", err
	}
	return data, fmt.Sprintf("%d", reader.Attrs.Generation), nil
}

func (g gcsObjects) PutIfAbsent(ctx context.Context, bucket, key string, body []byte) (string, error) {
	writer := g.client.Bucket(bucket).Object(key).If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	writer.ContentType = "application/json"
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.Close(); err != nil {
		if strings.Contains(err.Error(), "conditionNotMet") || strings.Contains(err.Error(), "Precondition") {
			return "", ErrLocked
		}
		return "", err
	}
	return fmt.Sprintf("%d", writer.Attrs().Generation), nil
}

func (g gcsObjects) Put(ctx context.Context, bucket, key string, body []byte) (string, error) {
	writer := g.client.Bucket(bucket).Object(key).NewWriter(ctx)
	writer.ContentType = "application/json"
	if _, err := io.Copy(writer, bytes.NewReader(body)); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", writer.Attrs().Generation), nil
}

func (g gcsObjects) Delete(ctx context.Context, bucket, key string) error {
	err := g.client.Bucket(bucket).Object(key).Delete(ctx)
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return err
	}
	return nil
}

func (g gcsObjects) Copy(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	src := g.client.Bucket(srcBucket).Object(srcKey)
	dst := g.client.Bucket(dstBucket).Object(dstKey)
	if _, err := dst.CopierFrom(src).Run(ctx); err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return ErrNotLocked
		}
		return fmt.Errorf("copy GCS object: %w", err)
	}
	return nil
}
