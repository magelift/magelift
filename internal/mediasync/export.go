package mediasync

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type DownloadAPI interface {
	s3.ListObjectsV2APIClient
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// Export writes a bucket's Magento media objects beneath the selected directory.
func Export(ctx context.Context, client DownloadAPI, bucket, prefix, directory string) (files, bytes int64, err error) {
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return 0, 0, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return 0, 0, err
	}
	defer root.Close()

	pages := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: &bucket, Prefix: &prefix})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return files, bytes, err
		}
		for _, item := range page.Contents {
			key := aws.ToString(item.Key)
			if strings.HasSuffix(key, "/") {
				continue
			}
			name, ok := strings.CutPrefix(key, prefix)
			if !ok || !fs.ValidPath(name) || name == "." || strings.Contains(name, "\\") || !filepath.IsLocal(filepath.FromSlash(name)) {
				return files, bytes, fmt.Errorf("invalid media object name %q", key)
			}
			n, err := download(ctx, client, root, bucket, key, filepath.FromSlash(name))
			if err != nil {
				return files, bytes, fmt.Errorf("download %q: %w", key, err)
			}
			files++
			bytes += n
		}
	}
	return files, bytes, nil
}

func download(ctx context.Context, client DownloadAPI, root *os.Root, bucket, key, name string) (int64, error) {
	dir := filepath.Dir(name)
	if err := root.MkdirAll(dir, 0o750); err != nil {
		return 0, err
	}
	if info, err := root.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return 0, errors.New("destination is not a regular file")
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	object, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return 0, err
	}
	defer object.Body.Close()
	temp := filepath.Join(dir, ".magelift-media-"+rand.Text())
	file, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Remove(temp) }()
	n, writeErr := io.Copy(file, object.Body)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return 0, err
	}
	if object.ContentLength != nil && n != *object.ContentLength {
		return 0, io.ErrUnexpectedEOF
	}
	// Publish each complete object atomically so a failed retry preserves the existing file.
	if err := root.Rename(temp, name); err != nil {
		return 0, err
	}
	return n, nil
}
