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
// production implementation speaks GCS; tests substitute fakes. Names
// are bucket-relative and untrusted: Download validates and contains
// them at open time. Upload takes a file the caller already opened
// without following links.
type objectStore interface {
	List(ctx context.Context, bucket, prefix string) ([]string, error)
	Download(ctx context.Context, bucket, object, root, name string) (int64, error)
	Upload(ctx context.Context, bucket, object string, src *os.File) (int64, error)
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

func (s gcsObjectStore) Download(ctx context.Context, bucket, object, root, name string) (int64, error) {
	if err := checkContainedName(name); err != nil {
		return 0, fmt.Errorf("object %q: %w", object, err)
	}
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return 0, err
	}
	defer rootDir.Close()
	if parent := parentDir(name); parent != "" {
		if err := rootDir.MkdirAll(parent, 0o755); err != nil {
			return 0, err
		}
	}
	if info, err := rootDir.Lstat(name); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("refusing to write through symlink at %q", name)
	}
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
	writer, err := rootDir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
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

func (s gcsObjectStore) Upload(ctx context.Context, bucket, object string, src *os.File) (int64, error) {
	if src == nil {
		return 0, fmt.Errorf("upload %s: no open file", object)
	}
	client, err := s.client(ctx)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	writer := client.Bucket(bucket).Object(object).NewWriter(ctx)
	written, copyErr := io.Copy(writer, src)
	closeErr := writer.Close()
	if copyErr != nil {
		return written, copyErr
	}
	return written, closeErr
}

// checkContainedName rejects bucket-relative names that could escape the
// destination root. Matching is per path element: `photo..jpg` is a
// legitimate name while `..` is not.
func checkContainedName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("empty object name")
	}
	if strings.HasPrefix(trimmed, "/") || filepath.IsAbs(filepath.FromSlash(trimmed)) {
		return fmt.Errorf("absolute object name")
	}
	for _, element := range strings.Split(trimmed, "/") {
		if element == "." || element == ".." {
			return fmt.Errorf("object name escapes its directory")
		}
	}
	return nil
}

// parentDir returns the slash-separated parent of a validated name, or
// "" when the name sits at the root.
func parentDir(name string) string {
	if index := strings.LastIndex(name, "/"); index >= 0 {
		return name[:index]
	}
	return ""
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
		if err := checkContainedName(relative); err != nil {
			return files, bytes, fmt.Errorf("object %q: %w", object, err)
		}
		written, err := store.Download(ctx, bucket, object, localDir, relative)
		if err != nil {
			return files, bytes, fmt.Errorf("download %s: %w", object, err)
		}
		files++
		bytes += written
	}
	return files, bytes, nil
}

func importMediaTree(ctx context.Context, store objectStore, bucket, localDir string) (int64, int64, error) {
	root, err := os.OpenRoot(localDir)
	if err != nil {
		return 0, 0, err
	}
	defer root.Close()
	var files, bytes int64
	err = filepath.WalkDir(localDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		// Re-stat through the root handle: WalkDir's entry reflects
		// enumeration time, but parents may have changed since. The
		// rooted open below can only fail closed on escape.
		info, err := root.Lstat(relative)
		if err != nil {
			return err
		}
		if info.IsDir() != entry.IsDir() {
			return fmt.Errorf("refusing %q: changed during traversal", relative)
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular file %q", relative)
		}
		file, err := openRootedRegularFile(root, relative)
		if err != nil {
			return err
		}
		object := gcpstorage.MediaObjectKey(filepath.ToSlash(relative))
		written, err := store.Upload(ctx, bucket, object, file)
		closeErr := file.Close()
		if err != nil {
			return fmt.Errorf("upload %s: %w", relative, err)
		}
		if closeErr != nil {
			return fmt.Errorf("upload %s: %w", relative, closeErr)
		}
		files++
		bytes += written
		return nil
	})
	return files, bytes, err
}

// openRootedRegularFile opens a root-relative path for reading. The
// root handle refuses escapes; the identity comparison refuses files
// swapped between the stat and the open.
func openRootedRegularFile(root *os.Root, name string) (*os.File, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing non-regular file %q", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !os.SameFile(info, opened) {
		file.Close()
		return nil, fmt.Errorf("refusing unstable file %q", name)
	}
	return file, nil
}
