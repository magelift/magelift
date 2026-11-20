package dumpimport

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/localdev"
)

// ErrNonEmptyRequiresYes is returned when the target schema already has base
// tables and Options.Yes is false (D-04). CLI callers wrap with invalid().
var ErrNonEmptyRequiresYes = errors.New("importing into a non-empty database requires --yes")

// Options configures a dump import against localdev MySQL (or equivalent).
// Yes maps to the persistent root --yes flag; do not invent --force.
type Options struct {
	// DumpPath is a .sql or .sql.gz file (synthetic fixtures under testdata/fixtures/migrate).
	DumpPath string
	// OutputPath is the local destination for Export. Must be a file path, not "-".
	OutputPath string
	// Yes confirms destructive schema-replace when the target is non-empty (D-04).
	// For Export, Yes overwrites an existing OutputPath.
	Yes bool
	// Sanitize rewrites the exported SQL so mailbox addresses are hashed. Off by
	// default. Other personal data stays in the export.
	Sanitize bool

	// Database is the target schema name (default magento).
	Database string
	// User / Password for mysql (default root/root; localdev admin for schema-replace).
	User     string
	Password string
	// Host / Port for host mysql client (defaults 127.0.0.1:3306).
	// In kube mode, Host is the Cloud SQL private IP reachable from the pod VPC.
	Host    string
	Port    int
	TLSMode string
	CAFile  string

	// Runner selects transport. Empty uses host mysql or compose fallback.
	// RunnerKube pipes SQL via kubectl exec into a VPC-adjacent pod.
	Runner string

	// Namespace / Pod / PodSelector / Container configure the kube runner.
	// Pod may be a pod name or a resource such as "deploy/web". PodSelector is
	// used only when Pod is empty (resolved via kubectl get pods -l …).
	Namespace   string
	Pod         string
	PodSelector string
	Container   string
	// Kubeconfig is an optional path passed as kubectl --kubeconfig.
	Kubeconfig string
	// KubeExec overrides kubectl execution (unit tests). Production leaves nil.
	KubeExec KubeExec

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
	if strings.TrimSpace(o.Host) == "" && !isKubeRunner(o.Runner) {
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

func isKubeRunner(runner string) bool {
	return strings.EqualFold(strings.TrimSpace(runner), RunnerKube)
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

	if strings.HasSuffix(strings.ToLower(opts.DumpPath), ".gz") {
		reader, closeReader, err := openDumpReader(ctx, opts.DumpPath)
		if err != nil {
			return err
		}
		_, validationErr := io.Copy(io.Discard, reader)
		closeReader()
		if validationErr != nil {
			return fmt.Errorf("dumpimport: invalid gzip dump: %w", validationErr)
		}
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
	sqlReader, cleanup, err := openDumpReader(ctx, opts.DumpPath)
	if err != nil {
		return err
	}
	defer cleanup()
	if nonEmpty && opts.Yes {
		if err := schemaReplace(ctx, runner, opts.Database); err != nil {
			return fmt.Errorf("dumpimport: schema replacement failed; the destination may be partially modified: %w", err)
		}
	}

	stderr, err := runner.ExecSQL(ctx, opts.Database, sqlReader)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("dumpimport: mysql import failed; the destination may be partially modified: %s", msg)
	}
	return nil
}

func openDumpReader(_ context.Context, path string) (io.Reader, func(), error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, fmt.Errorf("dumpimport: open dump: %w", err)
	}
	if !strings.HasSuffix(strings.ToLower(path), ".gz") {
		return file, func() { _ = file.Close() }, nil
	}
	reader, err := gzip.NewReader(file)
	if err != nil {
		_ = file.Close()
		return nil, func() {}, fmt.Errorf("dumpimport: invalid gzip dump: %w", err)
	}
	return reader, func() { _ = reader.Close(); _ = file.Close() }, nil
}

type mysqlRunner interface {
	ExecSQL(ctx context.Context, database string, stdin io.Reader) (stderr string, err error)
	Query(ctx context.Context, database, sql string) (string, error)
	Dump(ctx context.Context, database string, stdout io.Writer) (stderr string, err error)
}

func resolveRunner(opts Options) (mysqlRunner, error) {
	if opts.Port < 1 || opts.Port > 65535 {
		return nil, errors.New("dumpimport: database port must be between 1 and 65535")
	}
	if opts.TLSMode != "" && opts.TLSMode != "verify" {
		return nil, errors.New("dumpimport: TLS mode must be verify or unset")
	}
	if opts.TLSMode == "verify" {
		info, err := os.Stat(opts.CAFile)
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("dumpimport: verified TLS requires a readable CA file")
		}
		file, err := os.Open(opts.CAFile)
		if err != nil {
			return nil, errors.New("dumpimport: verified TLS requires a readable CA file")
		}
		_ = file.Close()
	} else if opts.CAFile != "" {
		return nil, errors.New("dumpimport: CA file requires verified TLS")
	}
	if opts.TLSMode == "verify" && isKubeRunner(opts.Runner) {
		return nil, errors.New("dumpimport: verified TLS is not supported by the kube runner; use magelift migrate restore")
	}
	if isKubeRunner(opts.Runner) {
		return newKubeRunner(opts)
	}
	if _, err := exec.LookPath("mysql"); err == nil {
		return &hostMySQL{
			user:     opts.User,
			password: opts.Password,
			host:     opts.Host,
			port:     opts.Port,
			tlsMode:  opts.TLSMode,
			caFile:   opts.CAFile,
		}, nil
	}
	if opts.TLSMode == "verify" {
		return nil, errors.New("dumpimport: verified TLS requires a host MySQL or MariaDB client")
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

// Query runs sql against the target schema via the resolved runner (host, compose, or kube).
// Harness cells use this to assert fixture tables after a managed dump import.
func Query(ctx context.Context, opts Options, sql string) (string, error) {
	opts = opts.withDefaults()
	runner, err := resolveRunner(opts)
	if err != nil {
		return "", err
	}
	return runner.Query(ctx, opts.Database, sql)
}

type hostMySQL struct {
	user, password, host string
	tlsMode, caFile      string
	port                 int
}

func (h *hostMySQL) baseArgs(database string) []string {
	args := []string{
		"-h", h.host,
		"-P", strconv.Itoa(h.port),
		"-u", h.user,
	}
	if database != "" {
		args = append(args, database)
	}
	return args
}

func (h *hostMySQL) command(ctx context.Context, binary, database string, extra ...string) (*exec.Cmd, error) {
	args := h.baseArgs("")
	if h.tlsMode == "verify" {
		version, err := exec.CommandContext(ctx, binary, "--version").Output()
		if err != nil {
			return nil, fmt.Errorf("dumpimport: identify %s client: %w", binary, err)
		}
		kind := strings.ToLower(string(version))
		switch {
		case strings.Contains(kind, "mariadb"):
			args = append(args, "--ssl", "--ssl-verify-server-cert", "--ssl-ca="+h.caFile)
		case strings.Contains(kind, "mysql"):
			args = append(args, "--ssl-mode=VERIFY_IDENTITY", "--ssl-ca="+h.caFile)
		default:
			return nil, fmt.Errorf("dumpimport: cannot determine %s TLS options from its version", binary)
		}
	}
	if database != "" {
		args = append(args, database)
	}
	args = append(args, extra...)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = mysqlPasswordEnv(h.password)
	return cmd, nil
}

func mysqlPasswordEnv(password string) []string {
	var result []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "MYSQL_PWD=") {
			result = append(result, entry)
		}
	}
	return append(result, "MYSQL_PWD="+password)
}

func (h *hostMySQL) ExecSQL(ctx context.Context, database string, stdin io.Reader) (string, error) {
	cmd, err := h.command(ctx, "mysql", database)
	if err != nil {
		return "", err
	}
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr, cmd.Stdout = &stderr, io.Discard
	err = cmd.Run()
	return stderr.String(), err
}

func (h *hostMySQL) Query(ctx context.Context, database, sql string) (string, error) {
	cmd, err := h.command(ctx, "mysql", database, "-N", "-e", sql)
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("dumpimport: mysql query failed: %s", msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (h *hostMySQL) Dump(ctx context.Context, database string, stdout io.Writer) (string, error) {
	if _, err := exec.LookPath("mysqldump"); err != nil {
		return "", errors.New("dumpimport: mysqldump not found on PATH")
	}
	cmd, err := h.command(ctx, "mysqldump", database, "--single-transaction", "--quick")
	if err != nil {
		return "", err
	}
	cmd.Stdout = stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stderr.String(), err
}

type composeMySQL struct {
	workDir, composeFile, project string
	user, password                string
}

func (c *composeMySQL) mysqlCmd(ctx context.Context, database string, extra ...string) *exec.Cmd {
	args := []string{
		"compose", "-f", c.composeFile, "--project-name", c.project,
		"exec", "-T", "-e", "MYSQL_PWD", "database",
		"mysql", "-u", c.user,
	}
	if database != "" {
		args = append(args, database)
	}
	args = append(args, extra...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = c.workDir
	cmd.Env = mysqlPasswordEnv(c.password)
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

func (c *composeMySQL) Dump(ctx context.Context, database string, stdout io.Writer) (string, error) {
	args := []string{
		"compose", "-f", c.composeFile, "--project-name", c.project,
		"exec", "-T", "database",
		"mysqldump", "-u", c.user,
	}
	if c.password != "" {
		args = append(args, "-p"+c.password)
	}
	args = append(args, "--single-transaction", "--quick")
	if database != "" {
		args = append(args, database)
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = c.workDir
	cmd.Stdout = stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
}
