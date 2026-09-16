package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
	archivecore "github.com/magelift/magelift/internal/shared/statearchive"
)

const stateBackupPrefix = "backups/"

type ArchiveAPI interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObjects(context.Context, *s3.DeleteObjectsInput, ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

type Archive struct {
	client     ArchiveAPI
	bucket     string
	encryption ObjectEncryption
	core       *archivecore.Archive
	now        func() time.Time
}

type BackupResult = archivecore.BackupResult
type RestoreResult = archivecore.RestoreResult

func NewAWSArchive(ctx context.Context, region, bucket string, encryption ObjectEncryption) (*Archive, error) {
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	return NewAWSArchiveWithEndpoint(ctx, region, bucket, encryption, endpoint)
}

// NewAWSArchiveWithEndpoint mirrors NewAWSWithEndpoint for backup/restore against
// AWS-compatible emulators (Floci) and S3-compatible providers.
func NewAWSArchiveWithEndpoint(ctx context.Context, region, bucket string, encryption ObjectEncryption, endpoint string) (*Archive, error) {
	validatedEndpoint, err := awsendpoint.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(config, func(options *s3.Options) {
		if validatedEndpoint != "" {
			options.BaseEndpoint = awssdk.String(validatedEndpoint)
			options.UsePathStyle = true
		}
	})
	return NewArchiveFromClient(client, bucket, encryption)
}

func NewArchiveFromClient(client ArchiveAPI, bucket string, encryption ObjectEncryption) (*Archive, error) {
	if client == nil || !bucketName.MatchString(bucket) {
		return nil, errors.New("state archive requires an S3 client and a state bucket")
	}
	if err := encryption.validate(); err != nil {
		return nil, err
	}
	core, err := archivecore.New(archiveStore{client: client, bucket: bucket, encryption: encryption}, stateBackupPrefix, "")
	if err != nil {
		return nil, err
	}
	return &Archive{client: client, bucket: bucket, encryption: encryption, core: core, now: time.Now}, nil
}

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

type archiveStore struct {
	client     ArchiveAPI
	bucket     string
	encryption ObjectEncryption
}

func (s archiveStore) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	var token *string
	for {
		output, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: awssdk.String(s.bucket), Prefix: awssdk.String(prefix), ContinuationToken: token})
		if err != nil {
			return nil, err
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

func (s archiveStore) Copy(ctx context.Context, source, target string) error {
	copyInput := &s3.CopyObjectInput{
		Bucket: awssdk.String(s.bucket), Key: awssdk.String(target), CopySource: awssdk.String(url.PathEscape(s.bucket + "/" + source)),
	}
	s.encryption.applyCopy(copyInput)
	_, err := s.client.CopyObject(ctx, copyInput)
	if err != nil {
		return err
	}
	return nil
}

func (s archiveStore) Read(ctx context.Context, key string) ([]byte, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: awssdk.String(s.bucket), Key: awssdk.String(key)})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Body == nil {
		return nil, errors.New("read state object returned no body")
	}
	defer output.Body.Close()
	body, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (s archiveStore) Put(ctx context.Context, key string, body []byte) error {
	putInput := &s3.PutObjectInput{Bucket: awssdk.String(s.bucket), Key: awssdk.String(key), Body: bytes.NewReader(body)}
	s.encryption.applyPut(putInput)
	_, err := s.client.PutObject(ctx, putInput)
	return err
}

func (s archiveStore) Delete(ctx context.Context, key string) error {
	return s.DeleteMany(ctx, []string{key})
}

func (s archiveStore) DeleteMany(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	const batchSize = 1000
	for start := 0; start < len(keys); start += batchSize {
		end := start + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		identifiers := make([]s3types.ObjectIdentifier, 0, end-start)
		for _, key := range keys[start:end] {
			identifiers = append(identifiers, s3types.ObjectIdentifier{Key: awssdk.String(key)})
		}
		output, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: awssdk.String(s.bucket),
			Delete: &s3types.Delete{Objects: identifiers, Quiet: awssdk.Bool(true)},
		})
		if err != nil {
			return err
		}
		if output != nil && len(output.Errors) > 0 {
			return fmt.Errorf("delete object: %s", awssdk.ToString(output.Errors[0].Message))
		}
	}
	return nil
}
