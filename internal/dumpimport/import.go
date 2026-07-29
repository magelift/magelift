package dumpimport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/acourtiol/magelift/internal/localdev"
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

func (o Options) withDefaults() Options {
	if strings.TrimSpace(o.Database) == "" {
		o.Database = "magento"
	}
	if strings.TrimSpace(o.User) == "" {
		o.User = "root"
	}
	if o.Password == "" && o.User == "root" {
		o.Password = "root"
	}
	if strings.TrimSpace(o.Host) == "" {
		o.Host = "127.0.0.1"
	}
	if o.Port == 0 {
		o.Port = 3306
	}
	if strings.TrimSpace(o.ComposeFile) == "" {
		o.ComposeFile = localdev.ComposeFile
	}
	if strings.TrimSpace(o.WorkDir) == "" {
		o.WorkDir = "."
	}
	return o
}

// Import pipes DumpPath into MySQL. Refuses non-empty targets without Yes;
// with Yes, schema-replaces then imports so interrupted re-runs converge.
func Import(ctx context.Context, opts Options) error {
	opts = opts.withDefaults()
	if strings.TrimSpace(opts.DumpPath) == "" {
		return errors.New("dumpimport: dump path is required")
	}
	if _, err := os.Stat(opts.DumpPath); err != nil {
		return fmt.Errorf("dumpimport: dump path: %w", err)
	}

	runner, err := resolveRunner(opts)
	if err != nil {
		return err
	}

	nonEmpty, err := nonEmptyWith(ctx, runner, opts.Database)
	if err != nil {
		return err
	}
	if nonEmpty && !opts.Yes {
		return ErrNonEmptyRequiresYes
	}
	if nonEmpty && opts.Yes {
		if err := schemaReplace(ctx, runner, opts.Database); err != nil {
			return err
		}
	}

	sqlReader, cleanup, err := openDumpReader(ctx, opts.DumpPath)
	if err != nil {
		return err
	}
	defer cleanup()

	stderr, err := runner.ExecSQL(ctx, opts.Database, sqlReader)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("dumpimport: mysql import failed: %s", msg)
	}
	return nil
}

func openDumpReader(ctx context.Context, path string) (io.Reader, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, func() {}, fmt.Errorf("dumpimport: open dump: %w", err)
	}
	if !strings.HasSuffix(strings.ToLower(path), ".gz") {
		return f, func() { _ = f.Close() }, nil
	}

	cmd := exec.CommandContext(ctx, "gunzip", "-c")
	cmd.Stdin = f
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = f.Close()
		return nil, func() {}, fmt.Errorf("dumpimport: gunzip pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return nil, func() {}, fmt.Errorf("dumpimport: start gunzip: %w", err)
	}
	cleanup := func() {
		_ = stdout.Close()
		waitErr := cmd.Wait()
		_ = f.Close()
		_ = waitErr
		_ = stderr
	}
	return stdout, cleanup, nil
}

type mysqlRunner interface {
	ExecSQL(ctx context.Context, database string, stdin io.Reader) (stderr string, err error)
	Query(ctx context.Context, database, sql string) (string, error)
}

func resolveRunner(opts Options) (mysqlRunner, error) {
	if _, err := exec.LookPath("mysql"); err == nil {
		return &hostMySQL{
			user:     opts.User,
			password: opts.Password,
			host:     opts.Host,
			port:     opts.Port,
		}, nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, errors.New("dumpimport: neither mysql nor docker found on PATH")
	}
	if strings.TrimSpace(opts.ComposeProject) == "" {
		return nil, errors.New("dumpimport: ComposeProject is required when using docker compose mysql")
	}
	composePath := filepath.Join(opts.WorkDir, opts.ComposeFile)
	if _, err := os.Stat(composePath); err != nil {
		return nil, fmt.Errorf("dumpimport: compose file: %w", err)
	}
	return &composeMySQL{
		workDir:     opts.WorkDir,
		composeFile: composePath,
		project:     opts.ComposeProject,
		user:        opts.User,
		password:    opts.Password,
	}, nil
}

type hostMySQL struct {
	user, password, host string
	port                 int
}

func (h *hostMySQL) baseArgs(database string) []string {
	args := []string{
		"-h", h.host,
		"-P", strconv.Itoa(h.port),
		"-u", h.user,
	}
	if h.password != "" {
		args = append(args, "-p"+h.password)
	}
	if database != "" {
		args = append(args, database)
	}
	return args
}

func (h *hostMySQL) ExecSQL(ctx context.Context, database string, stdin io.Reader) (string, error) {
	cmd := exec.CommandContext(ctx, "mysql", h.baseArgs(database)...)
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	err := cmd.Run()
	return stderr.String(), err
}

func (h *hostMySQL) Query(ctx context.Context, database, sql string) (string, error) {
	args := append(h.baseArgs(database), "-N", "-e", sql)
	cmd := exec.CommandContext(ctx, "mysql", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("dumpimport: mysql query failed: %s", msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

type composeMySQL struct {
	workDir, composeFile, project string
	user, password              string
}

func (c *composeMySQL) mysqlCmd(ctx context.Context, database string, extra ...string) *exec.Cmd {
	args := []string{
		"compose", "-f", c.composeFile, "--project-name", c.project,
		"exec", "-T", "database",
		"mysql", "-u", c.user,
	}
	if c.password != "" {
		args = append(args, "-p"+c.password)
	}
	if database != "" {
		args = append(args, database)
	}
	args = append(args, extra...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = c.workDir
	return cmd
}

func (c *composeMySQL) ExecSQL(ctx context.Context, database string, stdin io.Reader) (string, error) {
	cmd := c.mysqlCmd(ctx, database)
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	err := cmd.Run()
	return stderr.String(), err
}

func (c *composeMySQL) Query(ctx context.Context, database, sql string) (string, error) {
	cmd := c.mysqlCmd(ctx, database, "-N", "-e", sql)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("dumpimport: mysql query failed: %s", msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}
