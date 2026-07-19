package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	awsendpoint "github.com/acourtiol/magelift/internal/cloud/aws/endpoint"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const stateBackupPrefix = "backups/"
const backupCompleteObject = ".magelift-complete"

var backupIDPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z$`)

type ArchiveAPI interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObjects(context.Context, *s3.DeleteObjectsInput, ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

type Archive struct {
	client ArchiveAPI
	bucket string
	kmsARN string
	now    func() time.Time
}

type BackupResult struct {
	ID      string `json:"id" yaml:"id"`
	Prefix  string `json:"prefix" yaml:"prefix"`
	Objects int    `json:"objects" yaml:"objects"`
}

type RestoreResult struct {
	ID      string `json:"id" yaml:"id"`
	Prefix  string `json:"prefix" yaml:"prefix"`
	Objects int    `json:"objects" yaml:"objects"`
}

func NewAWSArchive(ctx context.Context, region, bucket, kmsARN string) (*Archive, error) {
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(config, func(options *s3.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
			options.UsePathStyle = true
		}
	})
	return NewArchiveFromClient(client, bucket, kmsARN)
}

func NewArchiveFromClient(client ArchiveAPI, bucket, kmsARN string) (*Archive, error) {
	if client == nil || !bucketName.MatchString(bucket) || !kmsKeyARN.MatchString(kmsARN) {
		return nil, errors.New("state archive requires an S3 client, a state bucket, and a KMS key ARN")
	}
	return &Archive{client: client, bucket: bucket, kmsARN: kmsARN, now: time.Now}, nil
}

func (a *Archive) Backup(ctx context.Context) (BackupResult, error) {
	if a == nil || a.client == nil {
		return BackupResult{}, errors.New("state archive is not configured")
	}
	id := a.now().UTC().Format("20060102T150405.000000000Z")
	prefix := stateBackupPrefix + id + "/"
	keys, err := a.list(ctx, "")
	if err != nil {
		return BackupResult{}, err
	}
	count := 0
	for _, key := range keys {
		if strings.HasPrefix(key, "locks/") || strings.HasPrefix(key, stateBackupPrefix) {
			continue
		}
		if err := a.copy(ctx, key, prefix+key); err != nil {
			return BackupResult{}, fmt.Errorf("backup state object %q: %w", key, err)
		}
		count++
	}
	if count == 0 {
		return BackupResult{}, errors.New("state bucket contains no durable state objects")
	}
	if err := a.complete(ctx, prefix); err != nil {
		return BackupResult{}, err
	}
	return BackupResult{ID: id, Prefix: prefix, Objects: count}, nil
}

func (a *Archive) Restore(ctx context.Context, id string) (RestoreResult, error) {
	if a == nil || a.client == nil {
		return RestoreResult{}, errors.New("state archive is not configured")
	}
	if !backupIDPattern.MatchString(id) {
		return RestoreResult{}, errors.New("state backup ID is invalid")
	}
	prefix := stateBackupPrefix + id + "/"
	backupKeys, err := a.list(ctx, prefix)
	if err != nil {
		return RestoreResult{}, err
	}
	if len(backupKeys) == 0 {
		return RestoreResult{}, errors.New("state backup was not found")
	}
	completeKey := prefix + backupCompleteObject
	complete := false
	backupObjects := make(map[string]struct{}, len(backupKeys))
	for _, key := range backupKeys {
		if key == completeKey {
			complete = true
			continue
		}
		target := strings.TrimPrefix(key, prefix)
		if target == "" || strings.HasPrefix(target, "locks/") || strings.HasPrefix(target, stateBackupPrefix) {
			return RestoreResult{}, errors.New("state backup contains an invalid object")
		}
		backupObjects[target] = struct{}{}
	}
	if !complete {
		return RestoreResult{}, errors.New("state backup is incomplete")
	}
	if len(backupObjects) == 0 {
		return RestoreResult{}, errors.New("state backup contains no durable state objects")
	}
	currentKeys, err := a.list(ctx, "")
	if err != nil {
		return RestoreResult{}, err
	}
	var stale []string
	for _, key := range currentKeys {
		if strings.HasPrefix(key, "locks/") || strings.HasPrefix(key, stateBackupPrefix) {
			continue
		}
		if _, ok := backupObjects[key]; !ok {
			stale = append(stale, key)
		}
	}
	for _, key := range backupKeys {
		if key == completeKey {
			continue
		}
		target := strings.TrimPrefix(key, prefix)
		if err := a.copy(ctx, key, target); err != nil {
			return RestoreResult{}, fmt.Errorf("restore state object %q: %w", target, err)
		}
	}
	// Copy the complete snapshot before deleting stale current state. If a
	// restore copy fails, unrelated current objects remain available for
	// recovery.
	if err := a.delete(ctx, stale); err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{ID: id, Prefix: prefix, Objects: len(backupObjects)}, nil
}

func (a *Archive) complete(ctx context.Context, prefix string) error {
	_, err := a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: awssdk.String(a.bucket), Key: awssdk.String(prefix + backupCompleteObject), Body: bytes.NewReader(nil),
		ServerSideEncryption: s3types.ServerSideEncryptionAwsKms, SSEKMSKeyId: awssdk.String(a.kmsARN),
	})
	if err != nil {
		return fmt.Errorf("complete state backup: %w", err)
	}
	return nil
}

func (a *Archive) list(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	var token *string
	for {
		output, err := a.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: awssdk.String(a.bucket), Prefix: awssdk.String(prefix), ContinuationToken: token})
		if err != nil {
			return nil, fmt.Errorf("list state objects: %w", err)
		}
		if output == nil {
			return nil, errors.New("list state objects returned no response")
		}
		for _, object := range output.Contents {
			if key := awssdk.ToString(object.Key); key != "" {
				keys = append(keys, key)
			}
		}
		if !awssdk.ToBool(output.IsTruncated) {
			return keys, nil
		}
		if output.NextContinuationToken == nil || awssdk.ToString(output.NextContinuationToken) == "" {
			return nil, errors.New("state object listing was truncated without a continuation token")
		}
		token = output.NextContinuationToken
	}
}

func (a *Archive) copy(ctx context.Context, source, target string) error {
	_, err := a.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket: awssdk.String(a.bucket), Key: awssdk.String(target), CopySource: awssdk.String(url.PathEscape(a.bucket + "/" + source)),
		ServerSideEncryption: s3types.ServerSideEncryptionAwsKms, SSEKMSKeyId: awssdk.String(a.kmsARN),
	})
	if err != nil {
		return fmt.Errorf("copy state object: %w", err)
	}
	return nil
}

func (a *Archive) delete(ctx context.Context, keys []string) error {
	for start := 0; start < len(keys); start += 1000 {
		end := start + 1000
		if end > len(keys) {
			end = len(keys)
		}
		objects := make([]s3types.ObjectIdentifier, 0, end-start)
		for _, key := range keys[start:end] {
			objects = append(objects, s3types.ObjectIdentifier{Key: awssdk.String(key)})
		}
		if len(objects) == 0 {
			continue
		}
		output, err := a.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: awssdk.String(a.bucket), Delete: &s3types.Delete{Objects: objects, Quiet: awssdk.Bool(true)}})
		if err != nil {
			return fmt.Errorf("delete stale state objects: %w", err)
		}
		if output != nil && len(output.Errors) > 0 {
			return fmt.Errorf("delete stale state objects: %s", awssdk.ToString(output.Errors[0].Message))
		}
	}
	return nil
}
