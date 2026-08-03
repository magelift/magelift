package state

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type fakeS3 struct {
	mu      sync.Mutex
	body    []byte
	etag    string
	putErr  error
	getErr  error
	delete  bool
	lastPut *s3.PutObjectInput
}

func (f *fakeS3) HeadObject(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return nil, nil
}
func (f *fakeS3) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	if len(f.body) == 0 {
		return nil, &smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(f.body)), ETag: awssdk.String(f.etag)}, nil
}
func (f *fakeS3) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastPut = input
	if f.putErr != nil {
		return nil, f.putErr
	}
	if len(f.body) != 0 && input.IfNoneMatch != nil && *input.IfNoneMatch == "*" {
		return nil, &smithy.GenericAPIError{Code: "PreconditionFailed", Message: "exists"}
	}
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	f.body, f.etag = data, "etag-1"
	return &s3.PutObjectOutput{ETag: awssdk.String(f.etag)}, nil
}
func (f *fakeS3) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if input.IfMatch != nil && *input.IfMatch != f.etag {
		return nil, &smithy.GenericAPIError{Code: "PreconditionFailed", Message: "etag changed"}
	}
	f.body, f.delete = nil, true
	return &s3.DeleteObjectOutput{}, nil
}

func TestAcquireReportsOwnerAndReleaseIsConditional(t *testing.T) {
	fake := &fakeS3{}
	manager, err := NewManager(fake, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionKMS, KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000"})
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return time.Unix(100, 0).UTC() }
	handle, err := manager.Acquire(context.Background(), "shop", "staging", "ci-run-1")
	if err != nil {
		t.Fatal(err)
	}
	if handle.Info().Owner != "ci-run-1" || handle.Info().AcquiredAt.Unix() != 100 {
		t.Fatalf("lock info = %#v", handle.Info())
	}
	if _, err := manager.Acquire(context.Background(), "shop", "staging", "ci-run-2"); err == nil || !errors.Is(err, ErrLocked) {
		t.Fatalf("second acquire error = %v", err)
	}
	if err := handle.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Status(context.Background()); !errors.Is(err, ErrNotLocked) {
		t.Fatalf("status error = %v", err)
	}
}

func TestRejectsInvalidManagerAndOwner(t *testing.T) {
	if _, err := NewManager(nil, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionKMS, KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000"}); err == nil {
		t.Fatal("nil client accepted")
	}
	fake := &fakeS3{}
	manager, err := NewManager(fake, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionKMS, KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acquire(context.Background(), "shop", "staging", ""); err == nil {
		t.Fatal("empty owner accepted")
	}
}

func TestReleaseAcceptsEquivalentTimestampLocations(t *testing.T) {
	fake := &fakeS3{}
	manager, err := NewManager(fake, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionKMS, KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000"})
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time {
		return time.Date(2026, time.July, 18, 2, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	}
	handle, err := manager.Acquire(context.Background(), "shop", "staging", "ci-run-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUnlockRemovesCurrentLockWithConditionalDelete(t *testing.T) {
	fake := &fakeS3{}
	manager, err := NewManager(fake, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionKMS, KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acquire(context.Background(), "shop", "staging", "ci-run-1"); err != nil {
		t.Fatal(err)
	}
	info, err := manager.Unlock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Owner != "ci-run-1" || !fake.delete {
		t.Fatalf("unlock info=%#v delete=%v", info, fake.delete)
	}
	if _, err := manager.Status(context.Background()); !errors.Is(err, ErrNotLocked) {
		t.Fatalf("status after unlock = %v", err)
	}
}

func TestEncryptionAES256LockSucceedsWithoutARN(t *testing.T) {
	fake := &fakeS3{}
	manager, err := NewManager(fake, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionAES256})
	if err != nil {
		t.Fatal(err)
	}
	if manager.key != "locks/shop/staging.json" {
		t.Fatalf("lock key path = %q", manager.key)
	}
	handle, err := manager.Acquire(context.Background(), "shop", "staging", "aes-owner")
	if err != nil {
		t.Fatal(err)
	}
	if handle.Info().Owner != "aes-owner" {
		t.Fatalf("owner = %q", handle.Info().Owner)
	}
	if fake.lastPut == nil || fake.lastPut.ServerSideEncryption != "AES256" {
		t.Fatalf("expected AES256 SSE, got %#v", fake.lastPut)
	}
	if fake.lastPut.SSEKMSKeyId != nil {
		t.Fatalf("AES256 must not set SSEKMSKeyId, got %v", awssdk.ToString(fake.lastPut.SSEKMSKeyId))
	}
}

func TestEncryptionKMSRejectsEmptyAndNonARN(t *testing.T) {
	fake := &fakeS3{}
	if _, err := NewManager(fake, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionKMS}); err == nil {
		t.Fatal("empty KMS ARN accepted")
	}
	if _, err := NewManager(fake, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionKMS, KMSKeyARN: "not-an-arn"}); err == nil {
		t.Fatal("non-ARN KMS key accepted")
	}
	if _, err := NewManager(fake, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionAES256, KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000"}); err == nil {
		t.Fatal("AES256 with KMS ARN accepted")
	}
}

func TestEncryptionKMSPutObjectSetsAwsKms(t *testing.T) {
	fake := &fakeS3{}
	arn := "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000"
	manager, err := NewManager(fake, "state-bucket", "shop", "staging", ObjectEncryption{Mode: EncryptionKMS, KMSKeyARN: arn})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acquire(context.Background(), "shop", "staging", "kms-owner"); err != nil {
		t.Fatal(err)
	}
	if fake.lastPut == nil || fake.lastPut.ServerSideEncryption != "aws:kms" {
		t.Fatalf("expected aws:kms SSE, got %#v", fake.lastPut)
	}
	if got := awssdk.ToString(fake.lastPut.SSEKMSKeyId); got != arn {
		t.Fatalf("SSEKMSKeyId = %q", got)
	}
}
