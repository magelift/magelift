// Package mediasync copies a local Magento media tree into an S3-compatible bucket.
package mediasync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// API covers the S3 requests used to upload and verify media.
type API interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// Options configures a media sync into Bucket via Client.
// Sync updates source keys, preserves other objects and verifies the resulting listing.
type Options struct {
	Source string
	Bucket string
	Prefix string
	Client API
}

// Result summarizes a completed sync.
type Result struct {
	Uploaded int
	Bytes    int64
	Keys     []string
	Diff     ListingDiff
}

// ListingDiff is the set difference between expected source keys and listed bucket keys.
type ListingDiff struct {
	Missing []string
	Extra   []string
}

// Empty reports whether the listing matches the source.
func (d ListingDiff) Empty() bool {
	return len(d.Missing) == 0 && len(d.Extra) == 0
}

// Sync uploads each regular file and compares the remote listing with the source.
func Sync(ctx context.Context, opts Options) (Result, error) {
	if opts.Client == nil {
		return Result{}, errors.New("mediasync: S3 client is required")
	}
	bucket := strings.TrimSpace(opts.Bucket)
	if bucket == "" {
		return Result{}, errors.New("mediasync: bucket is required")
	}
	source, err := confineSource(opts.Source)
	if err != nil {
		return Result{}, err
	}
	files, err := walkMediaFiles(source)
	if err != nil {
		return Result{}, err
	}
	if len(files) == 0 {
		return Result{}, fmt.Errorf("mediasync: source %q contains no files", source)
	}

	prefix := strings.TrimSpace(opts.Prefix)
	if prefix != "" && (strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "..") || !strings.HasSuffix(prefix, "/")) {
		return Result{}, errors.New("mediasync: prefix must be a relative directory ending in /")
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	var uploadedBytes int64
	keys := make([]string, 0, len(files))
	for _, file := range files {
		key, mapErr := objectKey(source, file)
		if mapErr != nil {
			return Result{}, mapErr
		}
		key = prefix + key
		relative, err := filepath.Rel(source, file)
		if err != nil {
			return Result{}, err
		}
		n, err := putFile(ctx, opts.Client, root, bucket, key, relative)
		if err != nil {
			return Result{}, err
		}
		uploadedBytes += n
		keys = append(keys, key)
	}
	sort.Strings(keys)

	listed, err := listAllKeys(ctx, opts.Client, bucket, prefix)
	if err != nil {
		return Result{}, err
	}
	diff := DiffKeys(keys, listed)
	if len(diff.Missing) > 0 {
		return Result{Uploaded: len(keys), Bytes: uploadedBytes, Keys: keys, Diff: diff}, fmt.Errorf("mediasync: listing drift after sync: missing keys %v", diff.Missing)
	}
	return Result{Uploaded: len(keys), Bytes: uploadedBytes, Keys: keys, Diff: diff}, nil
}

// DiffKeys returns Missing = expected−listed and Extra = listed−expected.
func DiffKeys(expected, listed []string) ListingDiff {
	want := make(map[string]struct{}, len(expected))
	for _, key := range expected {
		want[key] = struct{}{}
	}
	have := make(map[string]struct{}, len(listed))
	for _, key := range listed {
		have[key] = struct{}{}
	}
	var missing, extra []string
	for key := range want {
		if _, ok := have[key]; !ok {
			missing = append(missing, key)
		}
	}
	for key := range have {
		if _, ok := want[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return ListingDiff{Missing: missing, Extra: extra}
}

func confineSource(source string) (string, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return "", errors.New("mediasync: source is required")
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", errors.New("mediasync: source path must not escape via ..")
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("mediasync: resolve source: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("mediasync: source: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("mediasync: source %q is not a directory", abs)
	}
	return abs, nil
}

func walkMediaFiles(source string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("mediasync: path escapes source root: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mediasync: walk source: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

// objectKey maps a filesystem path to an object key.
// If the relative path contains pub/media/, strip through that prefix;
// otherwise keys are relative to the source root (media root).
func objectKey(sourceRoot, absPath string) (string, error) {
	rel, err := filepath.Rel(sourceRoot, absPath)
	if err != nil {
		return "", fmt.Errorf("mediasync: relative key: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("mediasync: path escapes source root: %s", absPath)
	}
	slash := filepath.ToSlash(rel)
	const marker = "pub/media/"
	if idx := strings.Index(slash, marker); idx >= 0 {
		key := slash[idx+len(marker):]
		if key == "" || strings.HasSuffix(key, "/") {
			return "", fmt.Errorf("mediasync: empty object key for %s", absPath)
		}
		return key, nil
	}
	if slash == "" || slash == "." {
		return "", fmt.Errorf("mediasync: empty object key for %s", absPath)
	}
	return slash, nil
}

func putFile(ctx context.Context, client API, root *os.Root, bucket, key, name string) (int64, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return 0, err
	}
	if !before.Mode().IsRegular() {
		return 0, fmt.Errorf("media file %q is not a regular file", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if !os.SameFile(before, info) {
		return 0, fmt.Errorf("media file %q changed during upload", name)
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket, Key: &key, Body: file, ContentLength: awssdk.Int64(info.Size()),
	})
	if err != nil {
		return 0, fmt.Errorf("put s3://%s/%s: %w", bucket, key, err)
	}
	return info.Size(), nil
}

func listAllKeys(ctx context.Context, client API, bucket, prefix string) ([]string, error) {
	var keys []string
	var token *string
	for {
		out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            awssdk.String(bucket),
			Prefix:            awssdk.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("mediasync: list s3://%s: %w", bucket, err)
		}
		for _, object := range out.Contents {
			key := awssdk.ToString(object.Key)
			if key == "" || strings.HasSuffix(key, "/") {
				continue
			}
			keys = append(keys, key)
		}
		if !awssdk.ToBool(out.IsTruncated) {
			break
		}
		token = out.NextContinuationToken
		if token == nil || *token == "" {
			return nil, errors.New("S3 returned a truncated media listing without a continuation token")
		}
	}
	sort.Strings(keys)
	return keys, nil
}
