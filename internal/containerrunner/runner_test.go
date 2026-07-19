package containerrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		fakeDocker()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRunUsesIsolatedMountsAndProtocolStdin(t *testing.T) {
	source := t.TempDir()
	t.Setenv("MAGELIFT_TEST_SECRET", "ambient-secret-marker")
	var diagnostics bytes.Buffer
	runner := Runner{Binary: os.Args[0], Image: "helper/inspect@" + testDigest, TempRoot: t.TempDir(), Stderr: &diagnostics}
	input := []byte(`{"protocolVersion":1,"token":"stdin-only-marker"}`)

	result, err := runner.Run(context.Background(), source, input, Secret{ID: "composer-auth", Value: []byte("secret-file-marker")})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	var invocation helperInvocation
	if err := json.Unmarshal(result.Response, &invocation); err != nil {
		t.Fatalf("decode helper response: %v: %s", err, result.Response)
	}
	if string(invocation.Stdin) != string(input) {
		t.Fatalf("stdin = %s", invocation.Stdin)
	}
	assertContainsArgument(t, invocation.Args, "--rm")
	assertContainsArgument(t, invocation.Args, "-i")
	assertContainsArgument(t, invocation.Args, "--network=none")
	assertContainsArgument(t, invocation.Args, "--read-only")
	assertContainsArgument(t, invocation.Args, "--cap-drop=ALL")
	assertContainsArgument(t, invocation.Args, "--security-opt=no-new-privileges")
	assertContainsArgument(t, invocation.Args, "--user="+containerUser())
	assertContainsArgument(t, invocation.Args, "--tmpfs")
	assertContainsArgument(t, invocation.Args, "/tmp:rw,noexec,nosuid,nodev,mode=1777")
	assertContainsArgument(t, invocation.Args, "--env")
	assertContainsArgument(t, invocation.Args, "HOME=/tmp")
	assertContainsArgument(t, invocation.Args, "type=bind,src="+source+",dst=/workspace,readonly")
	assertContainsArgument(t, invocation.Args, "type=bind,src="+result.OutputDir+",dst=/output")
	if !containsMountDestination(invocation.Args, "/run/secrets/composer-auth") {
		t.Fatalf("composer secret mount missing from %#v", invocation.Args)
	}
	if invocation.EnvironmentCount != 0 {
		t.Fatalf("child inherited %d environment entries", invocation.EnvironmentCount)
	}
	info, err := os.Stat(result.OutputDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("output permissions=%v", info.Mode().Perm())
	}
	if !strings.Contains(diagnostics.String(), "helper diagnostic") {
		t.Fatalf("missing stderr diagnostic: %q", diagnostics.String())
	}
	joined := strings.Join(invocation.Args, " ")
	if strings.Contains(joined, "stdin-only-marker") || strings.Contains(joined, "ambient-secret-marker") || strings.Contains(joined, "secret-file-marker") {
		t.Fatal("secret or protocol value leaked into argv")
	}
}

func TestRunInOutputReusesPrivateDirectory(t *testing.T) {
	source := t.TempDir()
	output := t.TempDir()
	if err := os.Chmod(output, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(output, "prepared.json")
	if err := os.WriteFile(marker, []byte("prepared"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Binary: os.Args[0], Image: "helper/inspect@" + testDigest}
	result, err := runner.RunInOutput(context.Background(), source, output, []byte(`{"protocolVersion":1,"stage":"finalize"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.OutputDir != output {
		t.Fatalf("output directory = %q", result.OutputDir)
	}
	if contents, err := os.ReadFile(marker); err != nil || string(contents) != "prepared" {
		t.Fatalf("prepared marker=%q error=%v", contents, err)
	}
	var invocation helperInvocation
	if err := json.Unmarshal(result.Response, &invocation); err != nil {
		t.Fatal(err)
	}
	assertContainsArgument(t, invocation.Args, "type=bind,src="+output+",dst=/output")
}

func TestRunInOutputRejectsNonPrivateDirectory(t *testing.T) {
	output := t.TempDir()
	if err := os.Chmod(output, 0o750); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Binary: os.Args[0], Image: "helper/inspect@" + testDigest}
	_, err := runner.RunInOutput(context.Background(), t.TempDir(), output, []byte(`{"protocolVersion":1}`))
	if err == nil || !strings.Contains(err.Error(), "must not be accessible by group or other users") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInOutputRejectsWritableSourceAlias(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(parent, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(source, "output")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Binary: os.Args[0], Image: "helper/inspect@" + testDigest}
	for _, test := range []struct {
		name   string
		output string
	}{
		{name: "same directory", output: source},
		{name: "source child", output: child},
		{name: "source parent", output: parent},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := runner.RunInOutput(context.Background(), source, test.output, []byte(`{"protocolVersion":1}`))
			if err == nil || !strings.Contains(err.Error(), "must not overlap") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRunAllowsExplicitBridgeNetworkForPreparation(t *testing.T) {
	runner := Runner{Binary: os.Args[0], Image: "helper/inspect@" + testDigest, TempRoot: t.TempDir(), Network: "bridge"}
	result, err := runner.Run(context.Background(), t.TempDir(), []byte(`{"protocolVersion":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()
	var invocation helperInvocation
	if err := json.Unmarshal(result.Response, &invocation); err != nil {
		t.Fatal(err)
	}
	assertContainsArgument(t, invocation.Args, "--network=bridge")
}

func TestRunBoundsStdoutAndCleansFailedOutput(t *testing.T) {
	tempRoot := t.TempDir()
	runner := Runner{Binary: os.Args[0], Image: "helper/oversize@" + testDigest, TempRoot: tempRoot, MaxStdout: 32}
	_, err := runner.Run(context.Background(), t.TempDir(), []byte(`{"protocolVersion":1}`))
	if !errors.Is(err, ErrStdoutLimit) {
		t.Fatalf("unexpected error: %v", err)
	}
	entries, readErr := os.ReadDir(tempRoot)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed run left output directories: %v", entries)
	}
}

func TestRunPropagatesCancellation(t *testing.T) {
	tempRoot := t.TempDir()
	runner := Runner{Binary: os.Args[0], Image: "helper/sleep@" + testDigest, TempRoot: tempRoot}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runner.Run(ctx, t.TempDir(), []byte(`{"protocolVersion":1}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("canceled container did not stop promptly")
	}
	entries, readErr := os.ReadDir(tempRoot)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("canceled run left output directories: %v", entries)
	}
}

func TestRejectsUnpinnedImageBeforeExecution(t *testing.T) {
	runner := Runner{Binary: os.Args[0], Image: "helper/latest"}
	_, err := runner.Run(context.Background(), t.TempDir(), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAcceptsPinnedLocalImageID(t *testing.T) {
	runner := Runner{Binary: os.Args[0], Image: testDigest, TempRoot: t.TempDir()}
	result, err := runner.Run(context.Background(), t.TempDir(), []byte(`{"protocolVersion":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()
}

type helperInvocation struct {
	Args             []string        `json:"args"`
	Stdin            json.RawMessage `json:"stdin"`
	EnvironmentCount int             `json:"environmentCount"`
}

func fakeDocker() {
	image := ""
	for _, argument := range os.Args[1:] {
		if strings.HasPrefix(argument, "helper/") {
			image = argument
		}
	}
	switch {
	case strings.HasPrefix(image, "helper/oversize@"):
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", 4096))
	case strings.HasPrefix(image, "helper/sleep@"):
		time.Sleep(10 * time.Second)
	default:
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(10)
		}
		_, _ = fmt.Fprintln(os.Stderr, "helper diagnostic")
		response, err := json.Marshal(helperInvocation{Args: os.Args[1:], Stdin: input, EnvironmentCount: len(os.Environ())})
		if err != nil {
			os.Exit(11)
		}
		_, _ = os.Stdout.Write(response)
	}
}

func assertContainsArgument(t *testing.T, arguments []string, wanted string) {
	t.Helper()
	for _, argument := range arguments {
		if argument == wanted {
			return
		}
	}
	t.Fatalf("argument %q missing from %#v", wanted, arguments)
}

func containsMountDestination(arguments []string, destination string) bool {
	for _, argument := range arguments {
		if strings.Contains(argument, "dst="+destination) {
			return true
		}
	}
	return false
}

func TestResultCleanupRemovesOutput(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "output")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := (Result{OutputDir: directory}).Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output still exists: %v", err)
	}
}
