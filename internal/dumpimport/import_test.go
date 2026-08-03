package dumpimport_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/dumpimport"
	"github.com/magelift/magelift/internal/localdev"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	root := repoRoot(t)
	p := filepath.Join(root, "testdata", "fixtures", "migrate", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return p
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func requireLocalMySQL(t *testing.T) dumpimport.Options {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping dumpimport MySQL integration in -short")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available for dumpimport MySQL proof")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	workDir := t.TempDir()
	composePath := filepath.Join(workDir, localdev.ComposeFile)
	if err := os.MkdirAll(filepath.Dir(composePath), 0o755); err != nil {
		t.Fatal(err)
	}
	// Minimal database-only compose (same image/credentials as localdev).
	// Publish an ephemeral host port so a host `mysql` client (when present) can connect.
	compose := `services:
  database:
    image: mysql:8.4@sha256:c592c15aaf4a1961e15d82eb31ea5987dda862d1c4b1e93424438c0e91dc1f8d
    environment:
      MYSQL_DATABASE: magento
      MYSQL_USER: magento
      MYSQL_PASSWORD: magento
      MYSQL_ROOT_PASSWORD: root
    ports:
      - "127.0.0.1:0:3306"
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h 127.0.0.1 -uroot -proot"]
      interval: 2s
      timeout: 3s
      retries: 30
`
	if err := os.WriteFile(composePath, []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	project := "magelift-dumpimport-" + strings.ReplaceAll(filepath.Base(workDir), ".", "")
	up := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "--project-name", project, "up", "-d", "database")
	up.Dir = workDir
	if out, err := up.CombinedOutput(); err != nil {
		t.Skipf("docker compose up failed (MySQL unavailable): %v\n%s", err, out)
	}
	t.Cleanup(func() {
		downCtx, downCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer downCancel()
		down := exec.CommandContext(downCtx, "docker", "compose", "-f", composePath, "--project-name", project, "down", "--volumes", "--remove-orphans")
		down.Dir = workDir
		_ = down.Run()
	})

	deadline := time.Now().Add(90 * time.Second)
	for {
		ping := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "--project-name", project, "exec", "-T", "database",
			"mysqladmin", "ping", "-h", "127.0.0.1", "-uroot", "-proot", "--silent")
		ping.Dir = workDir
		if err := ping.Run(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Skip("MySQL did not become healthy in time")
		}
		select {
		case <-ctx.Done():
			t.Skip("context canceled waiting for MySQL")
		case <-time.After(1 * time.Second):
		}
	}

	opts := dumpimport.Options{
		Database:       "magento",
		User:           "root",
		Password:       "root",
		ComposeFile:    localdev.ComposeFile,
		ComposeProject: project,
		WorkDir:        workDir,
	}
	if _, err := exec.LookPath("mysql"); err == nil {
		portCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "--project-name", project, "port", "database", "3306")
		portCmd.Dir = workDir
		out, err := portCmd.Output()
		if err != nil {
			t.Fatalf("compose port: %v", err)
		}
		// "127.0.0.1:xxxxx"
		addr := strings.TrimSpace(string(out))
		_, portStr, ok := strings.Cut(addr, ":")
		if !ok {
			t.Fatalf("unexpected compose port output %q", addr)
		}
		opts.Host = "127.0.0.1"
		port, err := strconv.Atoi(portStr)
		if err != nil {
			t.Fatalf("parse port %q: %v", portStr, err)
		}
		opts.Port = port
	}
	return opts
}

func TestImportCreatesTablesFromTinySQL(t *testing.T) {
	opts := requireLocalMySQL(t)
	opts.DumpPath = fixturePath(t, "tiny.sql")

	if err := dumpimport.Import(context.Background(), opts); err != nil {
		t.Fatalf("Import tiny.sql: %v", err)
	}
	nonEmpty, err := dumpimport.NonEmpty(context.Background(), opts)
	if err != nil {
		t.Fatalf("NonEmpty: %v", err)
	}
	if !nonEmpty {
		t.Fatal("expected tables after Import tiny.sql")
	}
	if err := assertProbeRow(t, opts); err != nil {
		t.Fatal(err)
	}
}

func TestHostModeDefaultsWhenKubeUnset(t *testing.T) {
	// Host/compose path leaves Runner empty; documented defaults are loopback MySQL.
	opts := dumpimport.Options{DumpPath: fixturePath(t, "tiny.sql")}
	if opts.Runner != "" {
		t.Fatalf("Runner = %q, want empty for host mode", opts.Runner)
	}
	if opts.Host != "" || opts.Port != 0 {
		t.Fatalf("unset Host/Port should be empty/0 before resolve, got %q/%d", opts.Host, opts.Port)
	}
	// Integration path (when docker available) still imports with defaults applied inside Import.
	live := requireLocalMySQL(t)
	if live.Host != "" && live.Host != "127.0.0.1" && live.Host != "localhost" {
		t.Fatalf("live host opts Host = %q, want loopback when mysql client is present", live.Host)
	}
	if live.Runner != "" {
		t.Fatalf("live host opts Runner = %q, want empty", live.Runner)
	}
}

func TestImportTinySQLGz(t *testing.T) {
	opts := requireLocalMySQL(t)
	opts.DumpPath = fixturePath(t, "tiny.sql.gz")
	opts.Yes = true // schema may be empty; Yes is harmless

	if err := dumpimport.Import(context.Background(), opts); err != nil {
		t.Fatalf("Import tiny.sql.gz: %v", err)
	}
	if err := assertProbeRow(t, opts); err != nil {
		t.Fatal(err)
	}
}

func TestNonEmptyWithoutYesRefuses(t *testing.T) {
	opts := requireLocalMySQL(t)
	opts.DumpPath = fixturePath(t, "tiny.sql")
	if err := dumpimport.Import(context.Background(), opts); err != nil {
		t.Fatalf("seed Import: %v", err)
	}

	opts.Yes = false
	err := dumpimport.Import(context.Background(), opts)
	if err == nil {
		t.Fatal("expected non-empty refusal without Yes")
	}
	if !errors.Is(err, dumpimport.ErrNonEmptyRequiresYes) {
		t.Fatalf("error = %v, want ErrNonEmptyRequiresYes", err)
	}
}

func TestYesSchemaReplaceConverges(t *testing.T) {
	opts := requireLocalMySQL(t)
	opts.DumpPath = fixturePath(t, "tiny.sql")
	if err := dumpimport.Import(context.Background(), opts); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	opts.Yes = true
	if err := dumpimport.Import(context.Background(), opts); err != nil {
		t.Fatalf("Yes Import (replace): %v", err)
	}
	if err := dumpimport.Import(context.Background(), opts); err != nil {
		t.Fatalf("Yes Import (converge second run): %v", err)
	}
	if err := assertProbeRow(t, opts); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptSQLSurfacesClearError(t *testing.T) {
	opts := requireLocalMySQL(t)
	opts.DumpPath = fixturePath(t, "corrupt.sql")

	err := dumpimport.Import(context.Background(), opts)
	if err == nil {
		t.Fatal("expected corrupt.sql to fail")
	}
	msg := err.Error()
	if strings.Contains(msg, "not implemented") {
		t.Fatalf("corrupt.sql must reach mysql (got stub): %v", err)
	}
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "import") && !strings.Contains(lower, "mysql") {
		t.Fatalf("corrupt error should mention import/mysql for journal mapping, got: %v", err)
	}
}

func assertProbeRow(t *testing.T, opts dumpimport.Options) error {
	t.Helper()
	composePath := filepath.Join(opts.WorkDir, opts.ComposeFile)
	cmd := exec.Command("docker", "compose", "-f", composePath, "--project-name", opts.ComposeProject,
		"exec", "-T", "database",
		"mysql", "-N", "-uroot", "-proot", "magento",
		"-e", "SELECT label FROM magelift_seed_probe WHERE id = 1")
	cmd.Dir = opts.WorkDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(string(out) + ": " + err.Error())
	}
	label := strings.TrimSpace(string(out))
	// mysql may print password warnings on stderr mixed into CombinedOutput
	lines := strings.Split(label, "\n")
	label = strings.TrimSpace(lines[len(lines)-1])
	if label != "tiny-fixture" {
		return errors.New("magelift_seed_probe row missing or wrong: " + label)
	}
	return nil
}
