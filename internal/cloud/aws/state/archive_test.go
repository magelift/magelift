package state

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type fakeArchiveS3 struct {
	objects      map[string]string
	failCopyFrom string
}

func (f *fakeArchiveS3) ListObjectsV2(_ context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	keys := make([]string, 0)
	prefix := awssdk.ToString(input.Prefix)
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return &s3.ListObjectsV2Output{Contents: func() []s3types.Object {
		objects := make([]s3types.Object, 0, len(keys))
		for _, key := range keys {
			objects = append(objects, s3types.Object{Key: awssdk.String(key)})
		}
		return objects
	}()}, nil
}

func (f *fakeArchiveS3) CopyObject(_ context.Context, input *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	source, err := url.PathUnescape(awssdk.ToString(input.CopySource))
	if err != nil {
		return nil, err
	}
	separator := strings.IndexByte(source, '/')
	if separator < 0 {
		return nil, nil
	}
	value, ok := f.objects[source[separator+1:]]
	if source[separator+1:] == f.failCopyFrom {
		return nil, errors.New("copy failed")
	}
	if !ok {
		return nil, nil
	}
	f.objects[awssdk.ToString(input.Key)] = value
	return &s3.CopyObjectOutput{}, nil
}

func (f *fakeArchiveS3) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.objects[awssdk.ToString(input.Key)] = ""
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeArchiveS3) DeleteObjects(_ context.Context, input *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	for _, object := range input.Delete.Objects {
		delete(f.objects, awssdk.ToString(object.Key))
	}
	return &s3.DeleteObjectsOutput{}, nil
}

func TestArchiveBackupExcludesLocksAndPreviousBackups(t *testing.T) {
	fake := &fakeArchiveS3{objects: map[string]string{
		"stacks/shop.json":             "state",
		"locks/shop/staging.json":      "lock",
		"backups/old/stacks/shop.json": "old",
	}}
	archive, err := NewArchiveFromClient(fake, "state-bucket", "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	archive.now = func() time.Time { return time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC) }
	result, err := archive.Backup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Objects != 1 || fake.objects[result.Prefix+"stacks/shop.json"] != "state" {
		t.Fatalf("backup result=%#v objects=%#v", result, fake.objects)
	}
	if _, ok := fake.objects[result.Prefix+"locks/shop/staging.json"]; ok {
		t.Fatal("lock was copied into backup")
	}
}

func TestArchiveRestoreDeletesStaleObjectsAndCopiesSnapshot(t *testing.T) {
	fake := &fakeArchiveS3{objects: map[string]string{"stacks/shop.json": "state", "stacks/old.json": "old"}}
	archive, err := NewArchiveFromClient(fake, "state-bucket", "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	archive.now = func() time.Time { return time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC) }
	backup, err := archive.Backup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fake.objects["stacks/shop.json"] = "changed"
	fake.objects["stacks/stale.json"] = "stale"
	result, err := archive.Restore(context.Background(), backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Objects != 2 || fake.objects["stacks/shop.json"] != "state" || fake.objects["stacks/old.json"] != "old" {
		t.Fatalf("restore result=%#v objects=%#v", result, fake.objects)
	}
	if _, ok := fake.objects["stacks/stale.json"]; ok {
		t.Fatal("stale state object was not removed")
	}
}

func TestArchiveRestoreRejectsPartialBackupBeforeMutation(t *testing.T) {
	id := "20260718T120000.000000000Z"
	prefix := stateBackupPrefix + id + "/"
	fake := &fakeArchiveS3{objects: map[string]string{
		"stacks/shop.json":          "current",
		prefix + "stacks/shop.json": "partial",
	}}
	archive, err := NewArchiveFromClient(fake, "state-bucket", "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Restore(context.Background(), id); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("error = %v", err)
	}
	if fake.objects["stacks/shop.json"] != "current" {
		t.Fatal("restore mutated state before checking backup completeness")
	}
}

func TestArchiveRestoreCopiesBeforeDeletingStaleObjects(t *testing.T) {
	fake := &fakeArchiveS3{objects: map[string]string{
		"stacks/shop.json":  "current",
		"stacks/stale.json": "stale",
	}}
	archive, err := NewArchiveFromClient(fake, "state-bucket", "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	archive.now = func() time.Time { return time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC) }
	backup, err := archive.Backup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fake.objects["stacks/shop.json"] = "changed"
	fake.objects["stacks/stale.json"] = "stale"
	fake.failCopyFrom = backup.Prefix + "stacks/shop.json"
	if _, err := archive.Restore(context.Background(), backup.ID); err == nil {
		t.Fatal("restore succeeded after a snapshot copy failure")
	}
	if fake.objects["stacks/stale.json"] != "stale" {
		t.Fatal("restore deleted stale state before copying the snapshot")
	}
}

func TestArchiveFailedBackupCannotBeRestored(t *testing.T) {
	fake := &fakeArchiveS3{objects: map[string]string{
		"stacks/shop.json": "state",
		"stacks/next.json": "next",
	}, failCopyFrom: "stacks/next.json"}
	archive, err := NewArchiveFromClient(fake, "state-bucket", "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	archive.now = func() time.Time { return time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC) }
	if _, err := archive.Backup(context.Background()); err == nil {
		t.Fatal("backup succeeded after a copy failure")
	}
	fake.failCopyFrom = ""
	if _, err := archive.Restore(context.Background(), "20260718T120000.000000000Z"); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("error = %v", err)
	}
}
