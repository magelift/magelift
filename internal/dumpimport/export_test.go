package dumpimport_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/dumpimport"
)

func TestExportKubeWritesLocalFileWithoutPasswordOnArgv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "staging.sql")
	fake := &fakeKubeMySQL{}
	opts := kubeOpts(t, fake)
	opts.OutputPath = dest
	opts.DumpPath = ""
	if err := dumpimport.Export(context.Background(), opts); err != nil {
		t.Fatalf("Export: %v", err)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("CREATE TABLE magelift_export_probe")) {
		t.Fatalf("dump body = %q", body)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 600", info.Mode().Perm())
	}
	for _, call := range fake.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "secret") || strings.Contains(joined, "-p") {
			t.Fatalf("password leaked onto argv: %v", call)
		}
		if strings.Contains(joined, "mysqldump") && strings.Contains(joined, "exec") {
			return
		}
	}
	t.Fatalf("no mysqldump exec recorded: %#v", fake.calls)
}

func TestExportGzipCompressesDestination(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "staging.sql.gz")
	fake := &fakeKubeMySQL{}
	opts := kubeOpts(t, fake)
	opts.OutputPath = dest
	opts.DumpPath = ""
	if err := dumpimport.Export(context.Background(), opts); err != nil {
		t.Fatalf("Export: %v", err)
	}
	f, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	body, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("CREATE TABLE magelift_export_probe")) {
		t.Fatalf("gunzipped body = %q", body)
	}
}

func TestExportRefusesExistingWithoutYes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "staging.sql")
	if err := os.WriteFile(dest, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &fakeKubeMySQL{}
	opts := kubeOpts(t, fake)
	opts.OutputPath = dest
	opts.DumpPath = ""
	err := dumpimport.Export(context.Background(), opts)
	if !errors.Is(err, dumpimport.ErrOutputExists) {
		t.Fatalf("err = %v, want ErrOutputExists", err)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "keep" {
		t.Fatalf("existing file was mutated: %q", body)
	}
}

func TestExportOverwritesWithYes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "staging.sql")
	if err := os.WriteFile(dest, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &fakeKubeMySQL{}
	opts := kubeOpts(t, fake)
	opts.OutputPath = dest
	opts.DumpPath = ""
	opts.Yes = true
	if err := dumpimport.Export(context.Background(), opts); err != nil {
		t.Fatalf("Export: %v", err)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("CREATE TABLE magelift_export_probe")) {
		t.Fatalf("dump body = %q", body)
	}
}

func TestExportRequiresOutputPath(t *testing.T) {
	t.Parallel()
	fake := &fakeKubeMySQL{}
	opts := kubeOpts(t, fake)
	opts.DumpPath = ""
	err := dumpimport.Export(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "output path is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestSanitizeSQLHashesMailboxesAndLeavesDefiners(t *testing.T) {
	t.Parallel()
	sql := []byte("INSERT INTO `customer_entity` VALUES (1,'Ada','Lovelace','ada@example.com');\nCREATE DEFINER=`root`@`localhost` PROCEDURE `p`() BEGIN END;\n")
	got := dumpimport.SanitizeSQL(sql)
	if bytes.Contains(got, []byte("ada@example.com")) {
		t.Fatalf("mailbox left in dump: %s", got)
	}
	if !bytes.Contains(got, []byte("@sanitized.invalid")) {
		t.Fatalf("hashed mailbox missing: %s", got)
	}
	if !bytes.Contains(got, []byte("DEFINER=`root`@`localhost`")) {
		t.Fatalf("DEFINER rewritten: %s", got)
	}
	again := dumpimport.SanitizeSQL(sql)
	if !bytes.Equal(got, again) {
		t.Fatal("email hashing is not stable")
	}
}

func TestExportSanitizeRewritesMailboxes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "staging.sql")
	fake := &fakeKubeMySQL{dumpBody: "INSERT INTO customer_entity VALUES (1,'shop@example.com');\nCREATE TABLE magelift_export_probe (id INT);\n"}
	opts := kubeOpts(t, fake)
	opts.OutputPath = dest
	opts.DumpPath = ""
	opts.Sanitize = true
	if err := dumpimport.Export(context.Background(), opts); err != nil {
		t.Fatalf("Export: %v", err)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("shop@example.com")) {
		t.Fatalf("mailbox left in dump: %s", body)
	}
	if !bytes.Contains(body, []byte("CREATE TABLE magelift_export_probe")) {
		t.Fatalf("dump body = %q", body)
	}
}
