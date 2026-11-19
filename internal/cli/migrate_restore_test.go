package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestoreRejectsWrongNamespaceBeforeMutation(t *testing.T) {
	root := t.TempDir()
	dump := filepath.Join(root, "dump.sql")
	if err := os.WriteFile(dump, []byte("SELECT 1;"), 0600); err != nil {
		t.Fatal(err)
	}
	o := restoreOptions{Context: "target", Namespace: "shop", NamespaceUID: "expected", DatabaseHost: "db", Database: "magento", DatabaseSecret: "db-secret", SQL: dump, TLS: "verify"}
	calls := 0
	_, err := runKubeRestore(context.Background(), o, func(_ context.Context, _ io.Reader, args ...string) (string, error) { calls++; return "other", nil })
	if err == nil || !strings.Contains(err.Error(), "UID") || calls != 1 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}

func TestRestoreImportsFromSecretAndMediaPVC(t *testing.T) {
	root := t.TempDir()
	dump := filepath.Join(root, "dump.sql")
	_ = os.WriteFile(dump, []byte("SELECT 1;"), 0600)
	media := filepath.Join(root, "media")
	_ = os.Mkdir(media, 0700)
	_ = os.WriteFile(filepath.Join(media, "asset.txt"), []byte("content"), 0600)
	o := restoreOptions{Context: "target", Namespace: "shop", NamespaceUID: "uid", DatabaseHost: "db.internal", Database: "magento", DatabaseSecret: "db-secret", SQL: dump, Media: media, MediaClaim: "magelift-media", TLS: "verify"}
	if err := o.validate(); err != nil {
		t.Fatal(err)
	}
	var creates, removes, imports, archives int
	fake := func(_ context.Context, input io.Reader, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--context target --namespace shop") {
			t.Errorf("missing context: %s", joined)
		}
		switch {
		case strings.Contains(joined, "get namespace"):
			return "uid", nil
		case strings.Contains(joined, "create -f"):
			creates++
			data, err := io.ReadAll(input)
			if err != nil {
				t.Fatal(err)
			}
			var pod map[string]any
			if err := json.Unmarshal(data, &pod); err != nil {
				t.Fatal(err)
			}
			if creates == 1 && (!bytes.Contains(data, []byte("db-secret")) || bytes.Contains(data, []byte("secret-value"))) {
				t.Errorf("SQL helper manifest %s", data)
			}
			if creates == 2 && !bytes.Contains(data, []byte("magelift-media")) {
				t.Errorf("media helper manifest %s", data)
			}
		case strings.Contains(joined, "delete pod"):
			removes++
		case strings.Contains(joined, "SHOW FULL TABLES"):
			return "", nil
		case strings.Contains(joined, "exec -i") && strings.Contains(joined, "mysql"):
			imports++
			if !strings.Contains(joined, "--max-allowed-packet=1073741824") {
				t.Fatal("SQL client packet limit excludes large dumps")
			}
			data, _ := io.ReadAll(input)
			if string(data) != "SELECT 1;" {
				t.Errorf("SQL stream %q", data)
			}
		case strings.Contains(joined, "tar -xf"):
			archives++
			reader := tar.NewReader(input)
			h, err := reader.Next()
			if err != nil || h.Name != "asset.txt" || h.Uid != 10001 || h.Gid != 10001 {
				t.Errorf("archive header %v %v", h, err)
			}
			data, _ := io.ReadAll(reader)
			if string(data) != "content" {
				t.Errorf("media stream %q", data)
			}
			_, _ = io.Copy(io.Discard, input)
		}
		return "", nil
	}
	result, err := runKubeRestore(context.Background(), o, fake)
	if err != nil || creates != 2 || removes != 2 || imports != 1 || archives != 1 {
		t.Fatalf("result=%v err=%v creates=%d removes=%d imports=%d archives=%d", result, err, creates, removes, imports, archives)
	}
}

func TestRestoreRefusesNonEmptyAndCleansUp(t *testing.T) {
	root := t.TempDir()
	dump := filepath.Join(root, "dump.sql")
	_ = os.WriteFile(dump, []byte("DROP TABLE x;"), 0600)
	o := restoreOptions{Context: "target", Namespace: "shop", NamespaceUID: "uid", DatabaseHost: "db", Database: "magento", DatabaseSecret: "secret", SQL: dump, TLS: "verify"}
	removed, imported := false, false
	_, err := runKubeRestore(context.Background(), o, func(_ context.Context, input io.Reader, args ...string) (string, error) {
		s := strings.Join(args, " ")
		switch {
		case strings.Contains(s, "get namespace"):
			return "uid", nil
		case strings.Contains(s, "SHOW FULL TABLES"):
			if strings.Contains(s, "WHERE") {
				t.Fatal("a view-only destination must also count as nonempty")
			}
			return "catalog_view\tVIEW\n", nil
		case strings.Contains(s, "delete pod"):
			removed = true
		case strings.Contains(s, "exec -i"):
			imported = true
		}
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "non-empty") || !removed || imported {
		t.Fatalf("err=%v removed=%t imported=%t", err, removed, imported)
	}
}

func TestRestoreTarRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := writeRestoreTar(io.Discard, root); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestRestoreCleansUpWhenHelperFails(t *testing.T) {
	root := t.TempDir()
	dump := filepath.Join(root, "dump.sql")
	_ = os.WriteFile(dump, []byte("x"), 0600)
	o := restoreOptions{Context: "target", Namespace: "shop", NamespaceUID: "uid", DatabaseHost: "db", Database: "magento", DatabaseSecret: "secret", SQL: dump, TLS: "verify"}
	removed := false
	_, err := runKubeRestore(context.Background(), o, func(_ context.Context, _ io.Reader, args ...string) (string, error) {
		s := strings.Join(args, " ")
		if strings.Contains(s, "get namespace") {
			return "uid", nil
		}
		if strings.Contains(s, "wait ") {
			return "", errors.New("failed")
		}
		if strings.Contains(s, "delete pod") {
			removed = true
		}
		return "", nil
	})
	if err == nil || !removed {
		t.Fatalf("err=%v removed=%t", err, removed)
	}
}

func TestRestoreCommandDefaultsToReadOnlyPlan(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "dump.sql")
	_ = os.WriteFile(dump, []byte("SELECT 1"), 0600)
	var out, stderr bytes.Buffer
	cmd := newCommand(&out, &stderr, nil)
	cmd.SetArgs([]string{"--output", "json", "migrate", "restore", "--context", "target", "--namespace", "shop", "--namespace-uid", "uid", "--database-host", "db", "--database-secret", "shop-db-credentials", "--sql", dump})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\"action\": \"plan\"") {
		t.Fatalf("output=%s", out.String())
	}
}

func TestRestoreDetectsNamespaceReplacementBeforeImport(t *testing.T) {
	root := t.TempDir()
	dump := filepath.Join(root, "dump.sql")
	_ = os.WriteFile(dump, []byte("SELECT 1"), 0600)
	o := restoreOptions{Context: "target", Namespace: "shop", NamespaceUID: "uid", DatabaseHost: "db", Database: "magento", DatabaseSecret: "secret", SQL: dump, TLS: "verify"}
	lookups, imported := 0, false
	_, err := runKubeRestore(context.Background(), o, func(_ context.Context, _ io.Reader, args ...string) (string, error) {
		s := strings.Join(args, " ")
		if strings.Contains(s, "get namespace") {
			lookups++
			if lookups == 1 {
				return "uid", nil
			}
			return "new-uid", nil
		}
		if strings.Contains(s, "exec -i") && !strings.Contains(s, "SHOW FULL TABLES") {
			imported = true
		}
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "UID") || imported {
		t.Fatalf("err=%v lookups=%d imported=%t", err, lookups, imported)
	}
}

func TestRestoreReportsFailedCleanup(t *testing.T) {
	root := t.TempDir()
	dump := filepath.Join(root, "dump.sql")
	_ = os.WriteFile(dump, []byte("SELECT 1"), 0600)
	o := restoreOptions{Context: "target", Namespace: "shop", NamespaceUID: "uid", DatabaseHost: "db", Database: "magento", DatabaseSecret: "secret", SQL: dump, TLS: "verify"}
	_, err := runKubeRestore(context.Background(), o, func(_ context.Context, input io.Reader, args ...string) (string, error) {
		s := strings.Join(args, " ")
		if strings.Contains(s, "get namespace") {
			return "uid", nil
		}
		if strings.Contains(s, "delete pod") {
			return "", errors.New("denied")
		}
		if input != nil {
			_, _ = io.Copy(io.Discard, input)
		}
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "remove helper pod") {
		t.Fatalf("cleanup error=%v", err)
	}
}

func TestRestorePodMountsCustomTLSAuthority(t *testing.T) {
	pod := restorePod("helper", restoreMySQLImage, restoreOptions{DatabaseHost: "db.example", Database: "magento", DatabaseSecret: "db-creds", CASecret: "db-ca"}, false)
	data, err := json.Marshal(pod)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"db-ca", "/tls", "db-creds", "username", "password"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestRestorePodAcceptsPasswordOnlySecret(t *testing.T) {
	pod := restorePod("helper", restoreMySQLImage, restoreOptions{DatabaseHost: "db", Database: "magento", DatabaseUser: "shopuser", DatabaseSecret: "db-secret"}, false)
	data, err := json.Marshal(pod)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("shopuser")) || bytes.Contains(data, []byte("\"key\":\"username\"")) || !bytes.Contains(data, []byte("\"key\":\"password\"")) {
		t.Fatalf("pod %s", data)
	}
}

func TestRestoreDoesNotDeleteAnExistingWriter(t *testing.T) {
	o := restoreOptions{Context: "target", Namespace: "shop", NamespaceUID: "uid", Media: t.TempDir(), MediaClaim: "media", Timeout: 2 * time.Hour}
	var first string
	for range 2 {
		_, err := runKubeRestore(t.Context(), o, func(_ context.Context, in io.Reader, args ...string) (string, error) {
			switch cmd := strings.Join(args, " "); {
			case strings.Contains(cmd, "get namespace"):
				return "uid", nil
			case strings.Contains(cmd, "create -f"):
				var pod struct {
					Metadata struct{ Name string }
					Spec     struct{ ActiveDeadlineSeconds int }
				}
				if err := json.NewDecoder(in).Decode(&pod); err != nil {
					t.Fatal(err)
				}
				if pod.Metadata.Name == "" || (first != "" && first != pod.Metadata.Name) || pod.Spec.ActiveDeadlineSeconds != 7200 {
					t.Fatalf("helper identity and timeout: %#v", pod)
				}
				first = pod.Metadata.Name
				return "", errors.New("AlreadyExists")
			default:
				t.Fatalf("unexpected operation on another restore's pod: %s", cmd)
				return "", nil
			}
		})
		if err == nil || !strings.Contains(err.Error(), "AlreadyExists") {
			t.Fatal(err)
		}
	}
}

func TestRestoreReportsCompletedSQLWhenMediaFails(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "dump.sql")
	if err := os.WriteFile(dump, []byte("SELECT 1;"), 0600); err != nil {
		t.Fatal(err)
	}
	opts := restoreOptions{Context: "target", Namespace: "shop", NamespaceUID: "uid", DatabaseHost: "db", Database: "magento", DatabaseSecret: "db-secret", SQL: dump, Media: t.TempDir(), MediaClaim: "media", TLS: "verify"}
	creates := 0
	_, err := runKubeRestore(context.Background(), opts, func(_ context.Context, input io.Reader, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "get namespace"):
			return "uid", nil
		case strings.Contains(joined, "create -f"):
			creates++
			if creates == 2 {
				return "", errors.New("media helper unavailable")
			}
			return "", nil
		case strings.Contains(joined, "SHOW FULL TABLES"):
			return "", nil
		case strings.Contains(joined, "exec -i") && strings.Contains(joined, "mysql"):
			_, _ = io.Copy(io.Discard, input)
			return "", nil
		default:
			return "", nil
		}
	})
	if err == nil || !strings.Contains(err.Error(), "completed sql before failing") || !strings.Contains(err.Error(), "inspect the destination before retrying") {
		t.Fatalf("error=%v", err)
	}
}

func TestRestoreReportsTruncatedGzipMayHavePartiallyImported(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte("CREATE TABLE fixture (id INT);")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data := compressed.Bytes()
	dump := filepath.Join(t.TempDir(), "dump.sql.gz")
	if err := os.WriteFile(dump, data[:len(data)-6], 0600); err != nil {
		t.Fatal(err)
	}
	opts := restoreOptions{Context: "target", Namespace: "shop", NamespaceUID: "uid", DatabaseHost: "db", Database: "magento", DatabaseSecret: "db-secret", SQL: dump, TLS: "verify"}
	_, err := runKubeRestore(context.Background(), opts, func(_ context.Context, input io.Reader, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "get namespace"):
			return "uid", nil
		case strings.Contains(joined, "SHOW FULL TABLES"):
			return "", nil
		case strings.Contains(joined, "exec -i") && strings.Contains(joined, "mysql"):
			_, err := io.Copy(io.Discard, input)
			return "", err
		default:
			return "", nil
		}
	})
	if err == nil || !strings.Contains(err.Error(), "destination database may be partially modified") {
		t.Fatalf("error=%v", err)
	}
}
