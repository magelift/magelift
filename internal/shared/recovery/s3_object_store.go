package recovery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// S3ObjectAPI is the provider-neutral subset of the S3-compatible protocol
// used by the archive engine. Provider packages translate their official SDK
// request and response models to this port; the archive algorithm never
// depends on an SDK or on a provider's S3 dialect.
type S3ObjectAPI interface {
	List(context.Context, string, string, string) (S3ObjectPage, error)
	Head(context.Context, string, string) (ObjectMetadata, error)
	Read(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, []byte, map[string]string, S3ObjectWriteOptions) error
	Copy(context.Context, string, string, string, string, map[string]string, S3ObjectWriteOptions) error
}

// S3ObjectPage is one provider-neutral page of object metadata. The core
// adapter owns pagination so all S3-compatible providers have identical
// bounded-listing behavior.
type S3ObjectPage struct {
	Objects               []ObjectInfo
	NextContinuationToken string
	Truncated             bool
}

// S3ObjectWriteOptions contains portable write intent that an S3-compatible
// provider can translate to its official SDK. Empty values mean that the
// provider's configured bucket or service default applies.
type S3ObjectWriteOptions struct {
	Encryption     string
	EncryptionKey  string
	ObjectLockMode string
	RetainUntil    time.Time
}

// S3ObjectStore adapts the neutral S3-compatible port to ObjectStore, which
// is consumed by ObjectArchiveEngine. The configured write options are copied
// at construction and cannot be changed by an archive operation.
type S3ObjectStore struct {
	api     S3ObjectAPI
	options S3ObjectWriteOptions
}

var _ ObjectStore = S3ObjectStore{}

// NewS3ObjectStore constructs the reusable S3-compatible ObjectStore adapter.
func NewS3ObjectStore(api S3ObjectAPI, options S3ObjectWriteOptions) (ObjectStore, error) {
	if api == nil {
		return nil, errors.New("S3-compatible object API is required")
	}
	if strings.ContainsAny(options.Encryption+options.EncryptionKey+options.ObjectLockMode, "\r\n\x00") {
		return nil, errors.New("S3-compatible object write options contain control data")
	}
	return S3ObjectStore{api: api, options: options}, nil
}

func (store S3ObjectStore) List(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	objects := make([]ObjectInfo, 0)
	token := ""
	for {
		page, err := store.api.List(ctx, bucket, prefix, token)
		if err != nil {
			return nil, err
		}
		for _, object := range page.Objects {
			if strings.TrimSpace(object.Key) == "" || object.Size < 0 {
				return nil, errors.New("S3-compatible object listing returned an invalid object")
			}
			objects = append(objects, object)
		}
		if !page.Truncated {
			return objects, nil
		}
		if strings.TrimSpace(page.NextContinuationToken) == "" || page.NextContinuationToken == token {
			return nil, errors.New("S3-compatible object listing was truncated without a new continuation token")
		}
		token = page.NextContinuationToken
	}
}

func (store S3ObjectStore) Head(ctx context.Context, bucket, key string) (ObjectMetadata, error) {
	return store.api.Head(ctx, bucket, key)
}

func (store S3ObjectStore) Read(ctx context.Context, bucket, key string) ([]byte, error) {
	return store.api.Read(ctx, bucket, key)
}

func (store S3ObjectStore) Put(ctx context.Context, bucket, key string, body []byte, metadata map[string]string) error {
	if err := store.api.Put(ctx, bucket, key, body, cloneMetadata(metadata), store.options); err != nil {
		return fmt.Errorf("put S3-compatible object: %w", err)
	}
	return nil
}

func (store S3ObjectStore) Copy(ctx context.Context, sourceBucket, sourceKey, targetBucket, targetKey string, metadata map[string]string) error {
	if err := store.api.Copy(ctx, sourceBucket, sourceKey, targetBucket, targetKey, cloneMetadata(metadata), store.options); err != nil {
		return fmt.Errorf("copy S3-compatible object: %w", err)
	}
	return nil
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}
	return result
}
