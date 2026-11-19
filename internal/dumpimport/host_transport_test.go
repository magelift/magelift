package dumpimport

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostImportTLSAndNonemptyAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, version, wantFlag, nonempty string
		yes                               bool
		wantError                         bool
		wantExecutions                    int
	}{
		{"mysql empty", "mysql  Ver 8.4.10", "--ssl-mode=VERIFY_IDENTITY", "0", false, false, 2},
		{"mariadb empty", "mysql  Ver 15.1 Distrib 11.4.5-MariaDB", "--ssl-verify-server-cert", "0", false, false, 2},
		{"nonempty refused", "mysql  Ver 8.4.10", "--ssl-mode=VERIFY_IDENTITY", "1", false, true, 1},
		{"nonempty approved", "mysql  Ver 8.4.10", "--ssl-mode=VERIFY_IDENTITY", "1", true, false, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "argv")
			pw := filepath.Join(dir, "password")
			ca := filepath.Join(dir, "ca.pem")
			dump := filepath.Join(dir, "seed.sql")
			if err := os.WriteFile(ca, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dump, []byte("SELECT 1;"), 0600); err != nil {
				t.Fatal(err)
			}
			script := `#!/bin/sh
if [ "$1" = --version ]; then printf '%s\n' "$FAKE_VERSION"; exit 0; fi
printf '%s\n' '---' "$@" >> "$FAKE_ARGV"
printf '%s' "$MYSQL_PWD" > "$FAKE_PASSWORD"
case "$*" in *information_schema.tables*) printf '%s\n' "$FAKE_NONEMPTY";; esac
`
			if err := os.WriteFile(filepath.Join(dir, "mysql"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("FAKE_VERSION", tc.version)
			t.Setenv("FAKE_ARGV", log)
			t.Setenv("FAKE_PASSWORD", pw)
			t.Setenv("FAKE_NONEMPTY", tc.nonempty)
			opts := Options{DumpPath: dump, Host: "db.internal.example", Port: 3307, User: "shop", Password: "secret-value", Database: "magento", TLSMode: "verify", CAFile: ca, Yes: tc.yes}
			err := Import(context.Background(), opts)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			if tc.wantError && !errors.Is(err, ErrNonEmptyRequiresYes) {
				t.Fatalf("error=%v", err)
			}
			args, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			text := string(args)
			for _, want := range []string{tc.wantFlag, "--ssl-ca=" + ca, "db.internal.example", "3307", "shop"} {
				if !strings.Contains(text, want) {
					t.Errorf("args missing %q: %s", want, text)
				}
			}
			if strings.Contains(text, "secret-value") || strings.Contains(text, "-psecret") {
				t.Fatalf("password exposed in argv: %s", text)
			}
			if got, err := os.ReadFile(pw); err != nil || string(got) != "secret-value" {
				t.Fatalf("password not in process environment: %v", err)
			}
			if got := strings.Count(text, "---\n"); got != tc.wantExecutions {
				t.Fatalf("executions=%d want %d: %s", got, tc.wantExecutions, text)
			}
		})
	}
}

func TestHostTLSRejectsMissingCAAndUnknownClient(t *testing.T) {
	opts := Options{Host: "db.example", User: "shop", Port: 3306, TLSMode: "verify", CAFile: filepath.Join(t.TempDir(), "missing")}
	if _, err := resolveRunner(opts); err == nil || !strings.Contains(err.Error(), "CA file") {
		t.Fatalf("error=%v", err)
	}
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mysql"), []byte("#!/bin/sh\necho unknown-client\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	opts.CAFile = ca
	runner, err := resolveRunner(opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Query(context.Background(), "magento", "SELECT 1"); err == nil || !strings.Contains(err.Error(), "cannot determine") {
		t.Fatalf("error=%v", err)
	}
}

func TestImportRejectsTruncatedGzipBeforeAnyDatabaseCommand(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte("CREATE TABLE should_never_exist (id INT);")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	content := compressed.Bytes()
	path := filepath.Join(t.TempDir(), "truncated.sql.gz")
	if err := os.WriteFile(path, content[:len(content)-6], 0600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "invoked")
	script := "#!/bin/sh\ntouch \"$MARKER\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "mysql"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MARKER", marker)
	err := Import(context.Background(), Options{DumpPath: path, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "invalid gzip") {
		t.Fatalf("error=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("database client was invoked: %v", err)
	}
}
