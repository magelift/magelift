package dumpimport_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/dumpimport"
)

// fakeKubeMySQL simulates an in-pod mysql client reached via kubectl exec.
// It records argv for assertions and tracks BASE TABLE count + probe label.
type fakeKubeMySQL struct {
	failExec bool
	tables   int
	probe    string
	calls    [][]string
}

func (f *fakeKubeMySQL) Exec(ctx context.Context, stdin io.Reader, name string, args []string, env []string) (stdout, stderr string, err error) {
	_ = ctx
	_ = env
	if name != "kubectl" {
		return "", "unexpected binary", errors.New("want kubectl")
	}
	cp := append([]string(nil), args...)
	f.calls = append(f.calls, cp)

	// Selector resolution: kubectl get pods …
	if len(args) > 0 && args[0] == "get" {
		return "web-0", "", nil
	}

	mysqlArgs := afterDoubleDash(args)
	if containsArg(mysqlArgs, "-e") {
		sql := argAfter(mysqlArgs, "-e")
		return f.handleQuery(sql)
	}

	if f.failExec {
		return "", "mysql: fake exec failure", errors.New("exit status 1")
	}
	if stdin != nil {
		body, readErr := io.ReadAll(stdin)
		if readErr != nil {
			return "", readErr.Error(), readErr
		}
		f.applyImport(body)
	}
	return "", "", nil
}

func (f *fakeKubeMySQL) handleQuery(sql string) (stdout, stderr string, err error) {
	lower := strings.ToLower(sql)
	if strings.Contains(lower, "count(*)") && strings.Contains(lower, "information_schema") {
		return strconv.Itoa(f.tables), "", nil
	}
	if strings.Contains(lower, "magelift_seed_probe") {
		return f.probe, "", nil
	}
	if strings.Contains(lower, "show tables") {
		if f.tables == 0 {
			return "", "", nil
		}
		return "magelift_seed_probe\nstore", "", nil
	}
	return "", "unknown query", errors.New("exit status 1")
}

func (f *fakeKubeMySQL) applyImport(body []byte) {
	if bytes.Contains(body, []byte("DROP DATABASE")) {
		f.tables = 0
		f.probe = ""
	}
	if bytes.Contains(body, []byte("CREATE TABLE")) || bytes.Contains(body, []byte("magelift_seed_probe")) {
		f.tables = 2
		f.probe = "tiny-fixture"
	}
}

func afterDoubleDash(args []string) []string {
	for i, a := range args {
		if a == "--" && i+1 < len(args) {
			return args[i+1:]
		}
	}
	return nil
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func kubeOpts(t *testing.T, fake *fakeKubeMySQL) dumpimport.Options {
	t.Helper()
	return dumpimport.Options{
		DumpPath:  fixturePath(t, "tiny.sql"),
		Runner:    dumpimport.RunnerKube,
		Namespace: "magento",
		Pod:       "deploy/web",
		Container: "php",
		Host:      "10.10.0.5",
		Port:      3306,
		User:      "magento",
		Password:  "secret",
		Database:  "magento",
		KubeExec:  fake.Exec,
	}
}

func TestKubeRunnerTableDriven(t *testing.T) {
	tests := []struct {
		name       string
		failExec   bool
		preTables  int
		yes        bool
		wantErr    error
		wantProbe  string
		wantTables bool
	}{
		{
			name:       "tiny.sql succeeds and tables queryable",
			wantProbe:  "tiny-fixture",
			wantTables: true,
		},
		{
			name:     "exec failure is loud not silent",
			failExec: true,
		},
		{
			name:      "nonempty without yes refuses",
			preTables: 2,
			yes:       false,
			wantErr:   dumpimport.ErrNonEmptyRequiresYes,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeKubeMySQL{failExec: tc.failExec, tables: tc.preTables}
			if tc.preTables > 0 {
				fake.probe = "existing"
			}
			opts := kubeOpts(t, fake)
			opts.Yes = tc.yes

			err := dumpimport.Import(context.Background(), opts)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if tc.failExec {
				if err == nil {
					t.Fatal("expected kube exec failure to surface")
				}
				msg := strings.ToLower(err.Error())
				if strings.Contains(msg, "not implemented") {
					t.Fatalf("must not stub: %v", err)
				}
				if !strings.Contains(msg, "import") && !strings.Contains(msg, "mysql") {
					t.Fatalf("failure should mention import/mysql for journal mapping, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Import: %v", err)
			}

			nonEmpty, err := dumpimport.NonEmpty(context.Background(), opts)
			if err != nil {
				t.Fatalf("NonEmpty: %v", err)
			}
			if nonEmpty != tc.wantTables {
				t.Fatalf("NonEmpty = %v, want %v", nonEmpty, tc.wantTables)
			}
			if tc.wantProbe == "" {
				return
			}
			label, err := dumpimport.Query(context.Background(), opts, "SELECT label FROM magelift_seed_probe WHERE id = 1")
			if err != nil {
				t.Fatalf("verification query: %v", err)
			}
			if label != tc.wantProbe {
				t.Fatalf("probe label = %q, want %q", label, tc.wantProbe)
			}

			sawShell := false
			for _, call := range fake.calls {
				joined := strings.Join(call, " ")
				if strings.Contains(joined, opts.Password) {
					t.Fatalf("password leaked into kubectl argv: %v", call)
				}
				if strings.Contains(joined, "MYSQL_PWD="+opts.Password) {
					t.Fatalf("MYSQL_PWD=<secret> must not appear on argv: %v", call)
				}
				if strings.Contains(joined, "-p"+opts.Password) {
					t.Fatalf("password leaked into mysql argv: %v", call)
				}
				mysqlArgs := afterDoubleDash(call)
				if len(mysqlArgs) == 0 {
					continue
				}
				if containsArg(mysqlArgs, "sh") && containsArg(mysqlArgs, "-c") {
					sawShell = true
				}
			}
			if !sawShell {
				t.Fatal("expected in-pod sh -c password decode before mysql")
			}
		})
	}
}

func TestKubeRunnerArgvShape(t *testing.T) {
	fake := &fakeKubeMySQL{}
	opts := kubeOpts(t, fake)
	if err := dumpimport.Import(context.Background(), opts); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(fake.calls) == 0 {
		t.Fatal("expected kubectl invocations")
	}
	foundExec := false
	for _, call := range fake.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "exec") && strings.Contains(joined, "deploy/web") {
			foundExec = true
			if !strings.Contains(joined, "-n magento") {
				t.Fatalf("missing namespace in %v", call)
			}
			if !strings.Contains(joined, "10.10.0.5") {
				t.Fatalf("missing private SQL host in %v", call)
			}
			if !strings.Contains(joined, "-c php") {
				t.Fatalf("missing container in %v", call)
			}
		}
	}
	if !foundExec {
		t.Fatalf("no kubectl exec call recorded: %#v", fake.calls)
	}
}

func TestKubeRunnerSelectorResolvesPod(t *testing.T) {
	fake := &fakeKubeMySQL{}
	opts := kubeOpts(t, fake)
	opts.Pod = ""
	opts.PodSelector = "app=magento-web"
	if err := dumpimport.Import(context.Background(), opts); err != nil {
		t.Fatalf("Import with selector: %v", err)
	}
	foundGet := false
	for _, call := range fake.calls {
		if len(call) > 0 && call[0] == "get" {
			foundGet = true
			joined := strings.Join(call, " ")
			if !strings.Contains(joined, "app=magento-web") {
				t.Fatalf("selector missing from get: %v", call)
			}
		}
	}
	if !foundGet {
		t.Fatal("expected kubectl get pods for selector resolution")
	}
}
