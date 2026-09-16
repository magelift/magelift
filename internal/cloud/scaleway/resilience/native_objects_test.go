package resilience

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	cloudresilience "github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

func TestNativeAPIObjectRecoveryIsIdempotentResumableAndOwned(t *testing.T) {
	storage := newFakeScalewayS3()
	storage.putSource("source", "media/a.json", []byte(`{"id":"a"}`))
	storage.putSource("source", "media/b.json", []byte(`{"id":"b"}`))
	native, err := NewNativeAPI(storage, NativeAPIConfig{
		ArchiveBucket: "archive", RestoreBucket: "restore", ArchivePrefix: "snapshots", RestorePrefix: "isolated",
		RequireObjectLock: true, RetentionDays: 7, Now: func() time.Time { return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"media"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/known-content", OwnershipMarker: "magelift/test/scaleway", IdempotencyKey: "media/backup/1",
		ResourceReferences: map[string]string{"media": "scaleway-object://source/media"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 || !backup.Evidence[0].ProtectionVerified {
		t.Fatalf("backup observation = %#v", backup)
	}
	copies := storage.copies
	second, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("idempotent backup: %v", err)
	}
	if second.Status != sdk.ResilienceOperationSucceeded || storage.copies != copies {
		t.Fatalf("idempotent backup changed archive: %#v copies=%d want=%d", second, storage.copies, copies)
	}

	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "media/restore/1"
	restore.BackupReferences = map[string]string{"media": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || !strings.HasPrefix(restored.Evidence[0].RestoreID, "scaleway-object://restore/isolated/media/") {
		t.Fatalf("restore observation = %#v", restored)
	}
	resumed, err := client.Poll(context.Background(), restored.OperationID)
	if err != nil {
		t.Fatalf("resume restore: %v", err)
	}
	if resumed.Status != sdk.ResilienceOperationSucceeded || resumed.OperationID != restored.OperationID {
		t.Fatalf("resumed restore = %#v", resumed)
	}

	integrity := restore
	integrity.Action = sdk.ResilienceIntegrityCheck
	integrity.IdempotencyKey = "media/integrity/1"
	checked, err := client.Start(context.Background(), integrity)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if checked.Status != sdk.ResilienceOperationSucceeded || !checked.Evidence[0].ManifestVerified || !checked.Evidence[0].ProtectionVerified {
		t.Fatalf("integrity observation = %#v", checked)
	}

	inventory, err := client.Inventory(context.Background(), request.OwnershipMarker)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	if len(inventory) == 0 || !inventory[0].Owned {
		t.Fatalf("inventory = %#v", inventory)
	}
}

func TestNativeAPIRejectsUnsupportedQueueBeforeMutation(t *testing.T) {
	native, err := NewNativeAPI(newFakeScalewayS3(), NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudresilience.NativeOperationRequest{
		Provider: "scaleway", Operation: "scaleway.queue.backup.queue", Request: sdk.ResilienceOperationRequest{
			Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, FixtureID: "fixture", OwnershipMarker: "owner", IdempotencyKey: "queue/1",
			ResourceReferences: map[string]string{"queue": "scaleway-queue://queue"},
		},
	})
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("error = %v, want unsupported capability", err)
	}
}

type fakeScalewayObject struct {
	body       []byte
	metadata   map[string]string
	encryption s3types.ServerSideEncryption
	lockMode   s3types.ObjectLockMode
	retain     *time.Time
}

type fakeScalewayS3 struct {
	objects map[string]fakeScalewayObject
	copies  int
}

func newFakeScalewayS3() *fakeScalewayS3 {
	return &fakeScalewayS3{objects: make(map[string]fakeScalewayObject)}
}

func (fake *fakeScalewayS3) putSource(bucket, key string, body []byte) {
	fake.objects[bucket+"\x00"+key] = fakeScalewayObject{body: append([]byte(nil), body...), encryption: s3types.ServerSideEncryptionAes256}
}

func (fake *fakeScalewayS3) ListObjectsV2(_ context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := aws.ToString(input.Prefix)
	keys := make([]string, 0)
	for composite := range fake.objects {
		parts := strings.SplitN(composite, "\x00", 2)
		if len(parts) == 2 && parts[0] == aws.ToString(input.Bucket) && strings.HasPrefix(parts[1], prefix) {
			keys = append(keys, parts[1])
		}
	}
	sort.Strings(keys)
	contents := make([]s3types.Object, 0, len(keys))
	for _, key := range keys {
		object := fake.objects[aws.ToString(input.Bucket)+"\x00"+key]
		contents = append(contents, s3types.Object{Key: aws.String(key), Size: aws.Int64(int64(len(object.body))), ETag: aws.String("etag-" + key)})
	}
	return &s3.ListObjectsV2Output{Contents: contents, IsTruncated: aws.Bool(false)}, nil
}

func (fake *fakeScalewayS3) HeadObject(_ context.Context, input *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	object, ok := fake.objects[aws.ToString(input.Bucket)+"\x00"+aws.ToString(input.Key)]
	if !ok {
		return nil, errors.New("NotFound")
	}
	return &s3.HeadObjectOutput{ContentLength: aws.Int64(int64(len(object.body))), ETag: aws.String("etag-" + aws.ToString(input.Key)), Metadata: cloneScalewayMetadata(object.metadata), ServerSideEncryption: object.encryption, ObjectLockMode: object.lockMode, ObjectLockRetainUntilDate: object.retain}, nil
}

func (fake *fakeScalewayS3) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	object, ok := fake.objects[aws.ToString(input.Bucket)+"\x00"+aws.ToString(input.Key)]
	if !ok {
		return nil, errors.New("NoSuchKey")
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(object.body))}, nil
}

func (fake *fakeScalewayS3) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	encryption := input.ServerSideEncryption
	if encryption == "" {
		encryption = s3types.ServerSideEncryptionAes256
	}
	fake.objects[aws.ToString(input.Bucket)+"\x00"+aws.ToString(input.Key)] = fakeScalewayObject{body: body, metadata: cloneScalewayMetadata(input.Metadata), encryption: encryption, lockMode: input.ObjectLockMode, retain: input.ObjectLockRetainUntilDate}
	return &s3.PutObjectOutput{}, nil
}

func (fake *fakeScalewayS3) CopyObject(_ context.Context, input *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	source, err := url.PathUnescape(aws.ToString(input.CopySource))
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid copy source")
	}
	object, ok := fake.objects[parts[0]+"\x00"+parts[1]]
	if !ok {
		return nil, errors.New("NoSuchKey")
	}
	encryption := input.ServerSideEncryption
	if encryption == "" {
		encryption = s3types.ServerSideEncryptionAes256
	}
	fake.objects[aws.ToString(input.Bucket)+"\x00"+aws.ToString(input.Key)] = fakeScalewayObject{body: append([]byte(nil), object.body...), metadata: cloneScalewayMetadata(input.Metadata), encryption: encryption, lockMode: input.ObjectLockMode, retain: input.ObjectLockRetainUntilDate}
	fake.copies++
	return &s3.CopyObjectOutput{}, nil
}

func (fake *fakeScalewayS3) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	key := aws.ToString(input.Bucket) + "\x00" + aws.ToString(input.Key)
	if _, ok := fake.objects[key]; !ok {
		return nil, errors.New("NotFound")
	}
	delete(fake.objects, key)
	return &s3.DeleteObjectOutput{}, nil
}
