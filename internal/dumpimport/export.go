package dumpimport

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// UnsanitizedLabel is the operator-facing dump classification when --sanitize
// is not requested.
const UnsanitizedLabel = "unsanitized Magento data"

// ErrOutputExists is returned when Export would overwrite OutputPath without Yes.
var ErrOutputExists = errors.New("dumpimport: output path already exists; pass --yes to overwrite")

// Export writes a mysqldump of the target schema to OutputPath. Credentials
// follow the same transport as Import. The file is labeled unsanitized unless
// Sanitize is set.
func Export(ctx context.Context, opts Options) error {
	opts = opts.withDefaults()
	dest := strings.TrimSpace(opts.OutputPath)
	if dest == "" {
		return errors.New("dumpimport: output path is required")
	}
	if dest == "-" {
		return errors.New("dumpimport: output path must be a local file")
	}

	runner, err := resolveRunner(opts)
	if err != nil {
		return err
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if opts.Yes {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(dest, flags, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrOutputExists
		}
		return fmt.Errorf("dumpimport: create output: %w", err)
	}

	var (
		w      io.Writer = f
		closer io.Closer
	)
	if strings.HasSuffix(strings.ToLower(dest), ".gz") {
		gz := gzip.NewWriter(f)
		w = gz
		closer = gz
	}

	stderr, err := runner.Dump(ctx, opts.Database, w)
	if closer != nil {
		if closeErr := closer.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(dest)
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("dumpimport: mysqldump failed: %s", msg)
	}
	if opts.Sanitize {
		if err := sanitizeOutputFile(dest); err != nil {
			_ = os.Remove(dest)
			return err
		}
	}
	return nil
}
