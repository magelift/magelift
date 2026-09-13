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
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	archivecore "github.com/magelift/magelift/internal/cloud/statearchive"
	"google.golang.org/api/iterator"
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
	DeleteGeneration(ctx context.Context, bucket, key, generation string) error
	Copy(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error
}

// ArchiveAPI extends the lock object API with the prefix listing required to
// create and verify a complete state snapshot. Keeping this boundary small
// lets tests and community GCS-compatible implementations provide only the
// object operations MageLift actually needs.
type ArchiveAPI interface {
	ObjectAPI
	List(ctx context.Context, bucket, prefix string) ([]string, error)
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
	return func() error { return handle.Release(ctx) }, nil
}

func (h *Handle) Info() Info {
	if h == nil {
		return Info{}
	}
	return h.info
}

func (h *Handle) Release(ctx context.Context) error {
	if h == nil || h.manager == nil {
		return errors.New("deployment lock handle is required")
	}
	current, err := h.manager.inspect(ctx)
	if err != nil {
		return err
	}
	if current.Owner != h.info.Owner || !current.AcquiredAt.Equal(h.info.AcquiredAt) {
		return errors.New("deployment lock ownership changed")
	}
	if err := h.manager.client.DeleteGeneration(ctx, h.manager.bucket, h.manager.key, current.Generation); err != nil {
		return fmt.Errorf("release deployment lock: %w", err)
	}
	h.manager = nil
	return nil
}

func (m *Manager) Status(ctx context.Context) (Info, error) {
	return m.inspect(ctx)
}

func (m *Manager) Unlock(ctx context.Context) (Info, error) {
	info, err := m.inspect(ctx)
	if err != nil {
		return Info{}, err
	}
	if err := m.client.DeleteGeneration(ctx, m.bucket, m.key, info.Generation); err != nil {
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
	core *archivecore.Archive
	now  func() time.Time
}

func NewArchive(client ArchiveAPI, bucket, project, environment string) (*Archive, error) {
	if client == nil || !bucketName.MatchString(bucket) || !stableName.MatchString(project) || !stableName.MatchString(environment) {
		return nil, errors.New("state archive requires a GCS client, stable names, and a state bucket")
	}
	core, err := archivecore.New(gcsArchiveStore{client: client, bucket: bucket}, "backups/"+project+"/"+environment+"/", "gs://"+bucket+"/")
	if err != nil {
		return nil, err
	}
	return &Archive{core: core, now: time.Now}, nil
}

func NewArchiveGCS(ctx context.Context, bucket, project, environment string) (*Archive, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	return NewArchive(gcsObjects{client: client}, bucket, project, environment)
}

type BackupResult = archivecore.BackupResult
type RestoreResult = archivecore.RestoreResult

func (a *Archive) Backup(ctx context.Context) (BackupResult, error) {
	if a == nil || a.core == nil {
		return BackupResult{}, errors.New("state archive is not configured")
	}
	a.core.SetClock(a.now)
	return a.core.Backup(ctx)
}

func (a *Archive) Restore(ctx context.Context, id string) (RestoreResult, error) {
	if a == nil || a.core == nil {
		return RestoreResult{}, errors.New("state archive is not configured")
	}
	return a.core.Restore(ctx, id)
}

type gcsArchiveStore struct {
	client ArchiveAPI
	bucket string
}

func (s gcsArchiveStore) List(ctx context.Context, prefix string) ([]string, error) {
	return s.client.List(ctx, s.bucket, prefix)
}

func (s gcsArchiveStore) Copy(ctx context.Context, source, target string) error {
	return s.client.Copy(ctx, s.bucket, source, s.bucket, target)
}

func (s gcsArchiveStore) Read(ctx context.Context, key string) ([]byte, error) {
	body, _, err := s.client.Get(ctx, s.bucket, key)
	return body, err
}

func (s gcsArchiveStore) Put(ctx context.Context, key string, body []byte) error {
	_, err := s.client.Put(ctx, s.bucket, key, body)
	return err
}

func (s gcsArchiveStore) Delete(ctx context.Context, key string) error {
	return s.client.Delete(ctx, s.bucket, key)
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
	return g.DeleteGeneration(ctx, bucket, key, "")
}

func (g gcsObjects) DeleteGeneration(ctx context.Context, bucket, key, generation string) error {
	obj := g.client.Bucket(bucket).Object(key)
	if generation != "" {
		gen, err := strconv.ParseInt(generation, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid lock object generation: %w", err)
		}
		obj = obj.If(storage.Conditions{GenerationMatch: gen})
	}
	err := obj.Delete(ctx)
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

func (g gcsObjects) List(ctx context.Context, bucket, prefix string) ([]string, error) {
	it := g.client.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix})
	keys := make([]string, 0)
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list GCS objects: %w", err)
		}
		if attrs != nil && attrs.Name != "" {
			keys = append(keys, attrs.Name)
		}
	}
	sort.Strings(keys)
	return keys, nil
}
