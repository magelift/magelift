package dumpimport

import (
	"context"
	"errors"
)

// ErrNonEmptyRequiresYes is returned when the target schema already has base
// tables and Options.Yes is false (D-04). CLI callers wrap with invalid().
var ErrNonEmptyRequiresYes = errors.New("importing into a non-empty database requires --yes")

// Options configures a dump import against localdev MySQL (or equivalent).
// Yes maps to the persistent root --yes flag; do not invent --force.
type Options struct {
	// DumpPath is a .sql or .sql.gz file (synthetic fixtures under testdata/fixtures/migrate).
	DumpPath string
	// Yes confirms destructive schema-replace when the target is non-empty (D-04).
	Yes bool

	// Database is the target schema name (default magento).
	Database string
	// User / Password for mysql (default root/root — localdev admin for schema-replace).
	User     string
	Password string
	// Host / Port for host mysql client (defaults 127.0.0.1:3306).
	Host string
	Port int

	// ComposeFile is relative to WorkDir (default .magelift/compose.local.yml).
	ComposeFile string
	// ComposeProject is the docker compose --project-name (required for compose fallback).
	ComposeProject string
	// WorkDir is the directory containing ComposeFile (default ".").
	WorkDir string
}

// NonEmpty reports whether the target schema has ≥1 BASE TABLE.
func NonEmpty(ctx context.Context, opts Options) (bool, error) {
	return false, errors.New("dumpimport: NonEmpty not implemented")
}

// Import pipes DumpPath into MySQL. Refuses non-empty targets without Yes;
// with Yes, schema-replaces then imports.
func Import(ctx context.Context, opts Options) error {
	return errors.New("dumpimport: Import not implemented")
}
