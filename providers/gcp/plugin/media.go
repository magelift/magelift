package plugin

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/magelift/magelift/internal/platform"
	gcpstorage "github.com/magelift/magelift/providers/gcp/storage"
	"github.com/magelift/magelift/sdk"
)

// objectStore moves bytes between a bucket prefix and operator disk. The
// production implementation speaks GCS; tests substitute fakes.
type objectStore interface {
	List(ctx context.Context, bucket, prefix string) ([]string, error)
	Download(ctx context.Context, bucket, object, dest string) (int64, error)
	Upload(ctx context.Context, bucket, object, src string) (int64, error)
}

type gcsObjectStore struct {
	newClient func(ctx context.Context) (*storage.Client, error)
}

func (s gcsObjectStore) client(ctx context.Context) (*storage.Client, error) {
	if s.newClient != nil {
		return s.newClient(ctx)
	}
	return storage.NewClient(ctx)
}

func (s gcsObjectStore) List(ctx context.Context, bucket, prefix string) ([]string, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	var names []string
	query := &storage.Query{Prefix: prefix}
	objects := client.Bucket(bucket).Objects(ctx, query)
	for {
		attrs, err := objects.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(attrs.Name, "/") {
			continue
		}
		names = append(names, attrs.Name)
	}
	return names, nil
}

func (s gcsObjectStore) Download(ctx context.Context, bucket, object, dest string) (int64, error) {
	client, err := s.client(ctx)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	reader, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	writer, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(writer, reader)
	closeErr := writer.Close()
	if copyErr != nil {
		return written, copyErr
	}
	return written, closeErr
}

func (s gcsObjectStore) Upload(ctx context.Context, bucket, object, src string) (int64, error) {
	client, err := s.client(ctx)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	reader, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	writer := client.Bucket(bucket).Object(object).NewWriter(ctx)
	written, copyErr := io.Copy(writer, reader)
	closeErr := writer.Close()
	if copyErr != nil {
		return written, copyErr
	}
	return written, closeErr
}

// MediaExport downloads the media prefix to operator disk.
func (s *Server) MediaExport(ctx context.Context, req *sdk.MediaTransferCall) (*sdk.MediaTransferResult, *sdk.OperationError) {
	return s.mediaTransfer(ctx, req, true)
}

// MediaImport uploads an operator-local tree into the media prefix.
func (s *Server) MediaImport(ctx context.Context, req *sdk.MediaTransferCall) (*sdk.MediaTransferResult, *sdk.OperationError) {
	return s.mediaTransfer(ctx, req, false)
}

func (s *Server) mediaTransfer(ctx context.Context, req *sdk.MediaTransferCall, outward bool) (*sdk.MediaTransferResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("media transfer call is required")
	}
	if _, operr := loadSpecCall(req.Envelope, req.Plan); operr != nil {
		return nil, operr
	}
	outputs, operr := decodeOutputs(req.OutputsJSON)
	if operr != nil {
		return nil, operr
	}
	bucket, err := platform.RequireStringOutput(outputs, platform.OutputMediaBucket)
	if err != nil {
		return nil, InvalidError("media bucket output is required: " + err.Error())
	}
	localDir := strings.TrimSpace(req.LocalDir)
	if localDir == "" {
		return nil, InvalidError("local directory is required")
	}
	if !filepath.IsAbs(localDir) {
		return nil, InvalidError("local directory must be absolute")
	}
	store := s.mediaStore()
	var files, bytes int64
	if outward {
		if err := os.MkdirAll(localDir, 0o755); err != nil {
			return nil, mapError(fmt.Errorf("create export directory: %w", err))
		}
		files, bytes, err = exportMediaTree(ctx, store, bucket, localDir)
	} else {
		if info, err := os.Stat(localDir); err != nil || !info.IsDir() {
			return nil, InvalidError("local directory does not exist or is not a directory")
		}
		files, bytes, err = importMediaTree(ctx, store, bucket, localDir)
	}
	if err != nil {
		return nil, mapError(err)
	}
	return &sdk.MediaTransferResult{FileCount: files, ByteCount: bytes, LocalDir: localDir}, nil
}

func (s *Server) mediaStore() objectStore {
	if s != nil && s.NewMediaStore != nil {
		return s.NewMediaStore()
	}
	return gcsObjectStore{}
}

func exportMediaTree(ctx context.Context, store objectStore, bucket, localDir string) (int64, int64, error) {
	objects, err := store.List(ctx, bucket, gcpstorage.MediaPrefix)
	if err != nil {
		return 0, 0, fmt.Errorf("list media objects: %w", err)
	}
	var files, bytes int64
	for _, object := range objects {
		relative := strings.TrimPrefix(object, gcpstorage.MediaPrefix)
		if relative == "" || strings.Contains(relative, "..") {
			continue
		}
		dest := filepath.Join(localDir, filepath.FromSlash(relative))
		written, err := store.Download(ctx, bucket, object, dest)
		if err != nil {
			return files, bytes, fmt.Errorf("download %s: %w", object, err)
		}
		files++
		bytes += written
	}
	return files, bytes, nil
}

func importMediaTree(ctx context.Context, store objectStore, bucket, localDir string) (int64, int64, error) {
	var files, bytes int64
	err := filepath.WalkDir(localDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		object := gcpstorage.MediaPrefix + filepath.ToSlash(relative)
		written, err := store.Upload(ctx, bucket, object, path)
		if err != nil {
			return fmt.Errorf("upload %s: %w", relative, err)
		}
		files++
		bytes += written
		return nil
	})
	return files, bytes, err
}
