package bootstrap

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	gcpstate "github.com/magelift/magelift/providers/gcp/state"
)

type gcsBucketLister interface {
	List(ctx context.Context, bucket, prefix string) ([]string, error)
	Close() error
}

func gcsObjectListQuery(prefix string) *storage.Query {
	return &storage.Query{Prefix: prefix, Versions: true}
}

type gcsObjectLister struct {
	client *storage.Client
}

func newGCSObjectLister(ctx context.Context, opts ...option.ClientOption) (*gcsObjectLister, error) {
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	return &gcsObjectLister{client: client}, nil
}

func (g *gcsObjectLister) Close() error {
	if g == nil || g.client == nil {
		return nil
	}
	return g.client.Close()
}

func (g *gcsObjectLister) List(ctx context.Context, bucket, prefix string) ([]string, error) {
	if g == nil || g.client == nil {
		return nil, fmt.Errorf("list GCS objects: client is required")
	}
	it := g.client.Bucket(bucket).Objects(ctx, gcsObjectListQuery(prefix))
	keys := make([]string, 0)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list GCS objects in bucket %q: %w", bucket, err)
		}
		if attrs != nil && strings.TrimSpace(attrs.Name) != "" {
			keys = append(keys, attrs.Name)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

type gcsCheckpointReader struct {
	lister gcsBucketLister
}

func newGCSCheckpointReader(ctx context.Context, opts ...option.ClientOption) (StateCheckpointAPI, error) {
	lister, err := newGCSObjectLister(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &gcsCheckpointReader{lister: lister}, nil
}

func (g *gcsCheckpointReader) Close() error {
	if g == nil || g.lister == nil {
		return nil
	}
	return g.lister.Close()
}

func (g *gcsCheckpointReader) HasPriorCheckpointData(ctx context.Context, bucket, project, environment string) (bool, error) {
	if g == nil || g.lister == nil {
		return false, fmt.Errorf("state checkpoint reader is required")
	}
	lockKey := "locks/" + project + "/" + environment + ".json"
	checkPrefixes := []string{
		".pulumi/",
		"backups/" + project + "/" + environment + "/",
	}
	for _, prefix := range checkPrefixes {
		keys, err := g.lister.List(ctx, bucket, prefix)
		if err != nil {
			return false, err
		}
		for _, key := range keys {
			if key == lockKey {
				continue
			}
			if strings.TrimSpace(key) != "" {
				return true, nil
			}
		}
	}
	keys, err := g.lister.List(ctx, bucket, "")
	if err != nil {
		return false, err
	}
	for _, key := range keys {
		if key == lockKey || strings.HasPrefix(key, "locks/") {
			continue
		}
		if strings.HasPrefix(key, ".pulumi/") || strings.HasPrefix(key, "backups/") {
			return true, nil
		}
	}
	return false, nil
}

type gcsStateLockFactory struct {
	client     *storage.Client
	newManager func(client *storage.Client, bucket, project, environment string) (*gcpstate.Manager, error)
}

func newGCSStateLockFactory(ctx context.Context, opts ...option.ClientOption) (StateLockAPI, error) {
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create GCS client for state locks: %w", err)
	}
	return &gcsStateLockFactory{
		client: client,
		newManager: func(client *storage.Client, bucket, project, environment string) (*gcpstate.Manager, error) {
			return gcpstate.NewGCSFromClient(client, bucket, project, environment)
		},
	}, nil
}

func (f *gcsStateLockFactory) Close() error {
	if f == nil || f.client == nil {
		return nil
	}
	return f.client.Close()
}

func (f *gcsStateLockFactory) Acquire(ctx context.Context, bucket, project, environment, owner string) (func(context.Context) error, error) {
	if f == nil || f.newManager == nil || f.client == nil {
		return nil, fmt.Errorf("state lock factory is required")
	}
	manager, err := f.newManager(f.client, bucket, project, environment)
	if err != nil {
		return nil, err
	}
	handle, err := manager.Acquire(ctx, project, environment, owner)
	if err != nil {
		return nil, err
	}
	return handle.Release, nil
}
