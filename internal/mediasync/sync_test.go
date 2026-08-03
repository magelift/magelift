package mediasync

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type fakeMediaS3 struct {
	objects map[string][]byte
}

func (f *fakeMediaS3) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	f.objects[awssdk.ToString(input.Key)] = body
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeMediaS3) ListObjectsV2(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	contents := make([]s3types.Object, 0, len(f.objects))
	for key := range f.objects {
		contents = append(contents, s3types.Object{Key: awssdk.String(key)})
	}
	return &s3.ListObjectsV2Output{Contents: contents}, nil
}

func TestMediaSyncFixtureListingDiffEmpty(t *testing.T) {
	source := fixtureMediaRoot(t)
	fake := &fakeMediaS3{objects: map[string][]byte{}}
	result, err := Sync(context.Background(), Options{
		Source: source,
		Bucket: "shop-media",
		Client: fake,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Diff.Empty() {
		t.Fatalf("listing diff = %#v, want empty", result.Diff)
	}
	wantKey := "catalog/product/fixture.txt"
	if result.Uploaded != 1 || len(result.Keys) != 1 || result.Keys[0] != wantKey {
		t.Fatalf("result = %#v, want key %q", result, wantKey)
	}
	if string(fake.objects[wantKey]) != "fixture-media\n" {
		t.Fatalf("uploaded body = %q", fake.objects[wantKey])
	}
}

func TestMediaSyncPubMediaKeyMapping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app", "pub", "media", "wysiwyg", "banner.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("banner"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &fakeMediaS3{objects: map[string][]byte{}}
	result, err := Sync(context.Background(), Options{Source: dir, Bucket: "media", Client: fake})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Diff.Empty() || result.Keys[0] != "wysiwyg/banner.txt" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMediaSyncMergeLeavesRemoteExtras(t *testing.T) {
	source := fixtureMediaRoot(t)
	fake := &fakeMediaS3{objects: map[string][]byte{"stale/old.txt": []byte("old")}}
	result, err := Sync(context.Background(), Options{Source: source, Bucket: "media", Client: fake})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diff.Missing) != 0 {
		t.Fatalf("missing = %v", result.Diff.Missing)
	}
	if len(result.Diff.Extra) != 1 || result.Diff.Extra[0] != "stale/old.txt" {
		t.Fatalf("extra = %v", result.Diff.Extra)
	}
	if _, ok := fake.objects["stale/old.txt"]; !ok {
		t.Fatal("merge deleted remote extra key")
	}
}

func TestMediaSyncRejectsDotDotSource(t *testing.T) {
	_, err := Sync(context.Background(), Options{
		Source: "../outside",
		Bucket: "media",
		Client: &fakeMediaS3{},
	})
	if err == nil || !strings.Contains(err.Error(), "..") {
		t.Fatalf("err = %v, want .. rejection", err)
	}
}

func TestListingDiff(t *testing.T) {
	diff := DiffKeys([]string{"a", "b"}, []string{"b", "c"})
	if strings.Join(diff.Missing, ",") != "a" || strings.Join(diff.Extra, ",") != "c" {
		t.Fatalf("diff = %#v", diff)
	}
	if !DiffKeys([]string{"a"}, []string{"a"}).Empty() {
		t.Fatal("expected empty diff")
	}
}

func fixtureMediaRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "fixtures", "migrate", "media")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(abs, "catalog", "product", "fixture.txt")); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	return abs
}

func TestMediaSyncRequiresBucketAndClient(t *testing.T) {
	if _, err := Sync(context.Background(), Options{Source: t.TempDir()}); err == nil {
		t.Fatal("expected error")
	}
}
