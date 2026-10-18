package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

const gcpIdentityRelocationStep = "Relocate GCP federated credentials"

func TestGCPIdentityFileRelocationMovesCredentialsOutOfWorkspace(t *testing.T) {
	script := gcpIdentityRelocationScript(t)
	root := t.TempDir()
	workspace := filepath.Join(root, "repo with spaces")
	runnerTemp := filepath.Join(root, "runner temp")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runnerTemp, 0o700); err != nil {
		t.Fatal(err)
	}
	payloadPath := filepath.Join(workspace, "source-only.txt")
	const payload = "unchanged payload"
	if err := os.WriteFile(payloadPath, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, workspace)
	secretMarker := "FAKE_GCP_IDENTITY_SECRET_MARKER"
	credBody := `{"type":"service_account","private_key":"` + secretMarker + `"}`
	credName := "gha-creds-" + strings.Repeat("a", 8) + ".json"
	credPath := filepath.Join(workspace, credName)
	if err := os.WriteFile(credPath, []byte(credBody), 0o644); err != nil {
		t.Fatal(err)
	}
	githubEnv := filepath.Join(root, "github_env")
	if err := os.WriteFile(githubEnv, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, runErr := runGCPIdentityRelocation(t, script, workspace, runnerTemp, credPath, githubEnv)
	if runErr != nil {
		t.Fatalf("relocation failed: %v\nstdout=%s\nstderr=%s", runErr, out, errOut)
	}
	assertNoSecretLeak(t, secretMarker, out, errOut, githubEnv)
	if _, err := os.Stat(credPath); !os.IsNotExist(err) {
		t.Fatalf("source credential still present: err=%v", err)
	}
	if data, err := os.ReadFile(payloadPath); err != nil || string(data) != payload {
		t.Fatalf("workspace payload changed: err=%v data=%q", err, data)
	}
	assertGitClean(t, workspace)
	dest := parseRelocatedPath(t, githubEnv)
	if !strings.HasPrefix(dest, runnerTemp) {
		t.Fatalf("relocated path %q not under RUNNER_TEMP %q", dest, runnerTemp)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != credBody {
		t.Fatalf("relocated credential content mismatch: err=%v", err)
	}
	assertMode(t, dest, 0o600)
}

func TestGCPIdentityFileRelocationRejectsInvalidSources(t *testing.T) {
	script := gcpIdentityRelocationScript(t)
	for _, scenario := range []struct {
		name      string
		setup     func(root, workspace, runnerTemp string) (credPath string, githubEnv string)
		wantAlive bool
	}{
		{
			name: "missing path",
			setup: func(root, workspace, runnerTemp string) (string, string) {
				githubEnv := filepath.Join(root, "github_env")
				if err := os.WriteFile(githubEnv, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return "", githubEnv
			},
		},
		{
			name: "outside workspace",
			setup: func(root, workspace, runnerTemp string) (string, string) {
				credPath := filepath.Join(root, "gha-creds-outside.json")
				if err := os.WriteFile(credPath, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
				githubEnv := filepath.Join(root, "github_env")
				if err := os.WriteFile(githubEnv, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return credPath, githubEnv
			},
			wantAlive: true,
		},
		{
			name: "unexpected basename",
			setup: func(root, workspace, runnerTemp string) (string, string) {
				credPath := filepath.Join(workspace, "other-credentials.json")
				if err := os.WriteFile(credPath, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
				githubEnv := filepath.Join(root, "github_env")
				if err := os.WriteFile(githubEnv, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return credPath, githubEnv
			},
			wantAlive: true,
		},
		{
			name: "symlink",
			setup: func(root, workspace, runnerTemp string) (string, string) {
				target := filepath.Join(root, "real-cred.json")
				if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
				link := filepath.Join(workspace, "gha-creds-link.json")
				if err := os.Symlink(target, link); err != nil {
					t.Fatal(err)
				}
				githubEnv := filepath.Join(root, "github_env")
				if err := os.WriteFile(githubEnv, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return link, githubEnv
			},
			wantAlive: true,
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			workspace := filepath.Join(root, "workspace")
			runnerTemp := filepath.Join(root, "runner-temp")
			if err := os.MkdirAll(workspace, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(runnerTemp, 0o700); err != nil {
				t.Fatal(err)
			}
			credPath, githubEnv := scenario.setup(root, workspace, runnerTemp)
			_, _, runErr := runGCPIdentityRelocation(t, script, workspace, runnerTemp, credPath, githubEnv)
			if runErr == nil {
				t.Fatal("expected relocation to fail")
			}
			envData, _ := os.ReadFile(githubEnv)
			if len(bytes.TrimSpace(envData)) != 0 {
				t.Fatalf("GITHUB_ENV was written on failure: %q", envData)
			}
			if scenario.wantAlive && credPath != "" {
				if _, err := os.Stat(credPath); err != nil {
					t.Fatalf("expected source to remain: %v", err)
				}
			}
			entries, err := os.ReadDir(runnerTemp)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "gha-creds-reloc-") {
					t.Fatalf("leftover temp relocation file %q", entry.Name())
				}
			}
		})
	}
}

func gcpIdentityRelocationScript(t *testing.T) string {
	t.Helper()
	var w strings.Builder
	w.WriteString("jobs:\n  build:\n    steps:\n")
	writeGCPIdentityFileRelocation(&w)
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string
				Run  string
			}
		}
	}
	if err := yaml.Unmarshal([]byte(w.String()), &workflow); err != nil {
		t.Fatal(err)
	}
	for _, step := range workflow.Jobs["build"].Steps {
		if step.Name == gcpIdentityRelocationStep {
			return step.Run
		}
	}
	t.Fatal("missing relocation step")
	return ""
}

func runGCPIdentityRelocation(t *testing.T, script, workspace, runnerTemp, credPath, githubEnv string) (string, string, error) {
	t.Helper()
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(),
		"BASH_ENV=",
		"GITHUB_WORKSPACE="+workspace,
		"RUNNER_TEMP="+runnerTemp,
		"GITHUB_ENV="+githubEnv,
		"GOOGLE_GHA_CREDS_PATH="+credPath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func parseRelocatedPath(t *testing.T, githubEnv string) string {
	t.Helper()
	data, err := os.ReadFile(githubEnv)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	values := map[string]string{}
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected GITHUB_ENV line: %q", line)
		}
		values[parts[0]] = parts[1]
	}
	for _, key := range []string{"GOOGLE_GHA_CREDS_PATH", "GOOGLE_APPLICATION_CREDENTIALS", "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"} {
		if values[key] == "" {
			t.Fatalf("missing %s in GITHUB_ENV", key)
		}
	}
	if values["GOOGLE_GHA_CREDS_PATH"] != values["GOOGLE_APPLICATION_CREDENTIALS"] ||
		values["GOOGLE_GHA_CREDS_PATH"] != values["CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"] {
		t.Fatalf("GITHUB_ENV paths disagree: %#v", values)
	}
	return values["GOOGLE_GHA_CREDS_PATH"]
}

func assertNoSecretLeak(t *testing.T, marker string, stdout, stderr, githubEnv string) {
	t.Helper()
	for _, blob := range []string{stdout, stderr} {
		if strings.Contains(blob, marker) {
			t.Fatalf("secret marker leaked to output: %q", blob)
		}
	}
	data, err := os.ReadFile(githubEnv)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), marker) {
		t.Fatalf("secret marker leaked to GITHUB_ENV: %q", data)
	}
}

func assertMode(t *testing.T, path string, want uint32) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != os.FileMode(want) {
		t.Fatalf("mode = %o, want %o", info.Mode().Perm(), want)
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("-c", "user.email=test@example.com", "-c", "user.name=test", "add", "source-only.txt")
	git("-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-m", "init")
}

func assertGitClean(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	if len(bytes.TrimSpace(out)) != 0 {
		t.Fatalf("git working tree not clean after relocation:\n%s", out)
	}
}

func TestGCPIdentityFileRelocationRejectsTempInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "repo")
	temp := filepath.Join(workspace, "tmp")
	if err := os.MkdirAll(temp, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workspace, "gha-creds-test.json")
	if err := os.WriteFile(source, []byte("fake credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	envfile := filepath.Join(root, "env")
	if err := os.WriteFile(envfile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runGCPIdentityRelocation(t, gcpIdentityRelocationScript(t), workspace, temp, source, envfile)
	if err == nil {
		t.Fatal("expected refusal of credentials inside checkout")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source changed: %v", err)
	}
	files, err := os.ReadDir(temp)
	if err != nil || len(files) != 0 {
		t.Fatalf("unexpected temp mutation: %v %v", files, err)
	}
}

func TestGCPIdentityFileRelocationCleansUpIfEnvPublicationFails(t *testing.T) {
	root := t.TempDir()
	workspace, temp := filepath.Join(root, "repo"), filepath.Join(root, "temp")
	for _, dir := range []string{workspace, temp} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(workspace, "gha-creds-test.json")
	if err := os.WriteFile(source, []byte("fake credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Redirecting GITHUB_ENV into a directory must fail after the move.
	_, _, err := runGCPIdentityRelocation(t, gcpIdentityRelocationScript(t), workspace, temp, source, root)
	if err == nil {
		t.Fatal("expected failed environment publication")
	}
	files, readErr := os.ReadDir(temp)
	if readErr != nil || len(files) != 0 {
		t.Fatalf("credential left after publication failure: %v %v", files, readErr)
	}
}
