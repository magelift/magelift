package mediasync

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type downloadStore struct {
	fakeMediaS3
	length int64
}

func (s *downloadStore) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(s.objects[*in.Key])), ContentLength: aws.Int64(s.length)}, nil
}

func TestExportPreservesExistingFileOnTruncatedDownload(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "product.jpg")
	if err := os.WriteFile(name, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &downloadStore{fakeMediaS3: fakeMediaS3{objects: map[string][]byte{"media/product.jpg": []byte("part")}}, length: 9}
	if _, _, err := Export(t.Context(), store, "shop", "media/", dir); err == nil {
		t.Fatal("truncated object accepted")
	}
	if got, err := os.ReadFile(name); err != nil || string(got) != "original" {
		t.Fatalf("original file: %q %v", got, err)
	}
	names, err := os.ReadDir(dir)
	if err != nil || len(names) != 1 {
		t.Fatalf("temporary file left behind: %v %v", names, err)
	}
}

func TestExportConfinesObjectNamesAndSymlinks(t *testing.T) {
	for _, key := range []string{"../outside", "a/../outside", "nested\\escape", "symlink/outside"} {
		t.Run(strings.ReplaceAll(key, "/", "_"), func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			if err := os.Symlink(outside, filepath.Join(dir, "symlink")); err != nil {
				t.Skip(err)
			}
			store := &downloadStore{fakeMediaS3: fakeMediaS3{objects: map[string][]byte{"media/" + key: []byte("x")}}, length: 1}
			if _, _, err := Export(t.Context(), store, "shop", "media/", dir); err == nil {
				t.Fatalf("accepted %q", key)
			}
			files, err := os.ReadDir(outside)
			if err != nil || len(files) != 0 {
				t.Fatalf("outside files: %v %v", files, err)
			}
		})
	}
}
