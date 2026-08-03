package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestRunnerBuildsWithArgvAndParsesDigest(t *testing.T) {
	var diagnostics bytes.Buffer
	runner := NewRunner(&diagnostics)
	runner.command = helperCommand(t, "success")
	secret := []byte("private-composer-auth")

	result, err := runner.Build(context.Background(), Request{
		Source:         Source{LocalDirectory: "."},
		Dockerfile:     "Dockerfile",
		ImageReference: "magelift/test:revision",
		Platforms:      []string{"linux/amd64"},
		Output:         OutputLoad,
		Builder:        "default",
		Secrets:        []Secret{{ID: "composer-auth", Value: secret}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Digest != "sha256:"+strings.Repeat("a", 64) || result.Pushed {
		t.Fatalf("unexpected result: %#v", result)
	}
	if strings.Contains(diagnostics.String(), string(secret)) {
		t.Fatal("secret value was written to diagnostics")
	}
	if !strings.Contains(diagnostics.String(), "--load") || strings.Contains(diagnostics.String(), "--push") {
		t.Fatalf("unexpected build arguments: %s", diagnostics.String())
	}
	if !strings.Contains(diagnostics.String(), "--builder\ndefault") {
		t.Fatalf("selected builder missing: %s", diagnostics.String())
	}
}

func TestRunnerAddsReleaseAttestationsForPush(t *testing.T) {
	var diagnostics bytes.Buffer
	runner := NewRunner(&diagnostics)
	runner.command = helperCommand(t, "success")
	result, err := runner.Build(context.Background(), Request{
		Source:         Source{GitURL: "https://example.invalid/shop.git?ref=v1&checksum=" + strings.Repeat("a", 40)},
		Dockerfile:     "Dockerfile",
		ImageReference: "registry.example.invalid/shop:candidate",
		Platforms:      []string{"linux/amd64", "linux/arm64"},
		Output:         OutputPush,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed || !strings.Contains(diagnostics.String(), "--sbom=true") || !strings.Contains(diagnostics.String(), "--provenance=mode=max,version=v1") {
		t.Fatalf("release attestations missing: %s", diagnostics.String())
	}
}

func TestRunnerPushesPreparedLocalContextWithSeparateProvenanceSource(t *testing.T) {
	var diagnostics bytes.Buffer
	runner := NewRunner(&diagnostics)
	runner.command = helperCommand(t, "success")
	provenance := "https://example.invalid/shop.git?checksum=" + strings.Repeat("b", 40)
	result, err := runner.Build(context.Background(), Request{
		Source:           Source{LocalDirectory: "prepared/rootfs"},
		ProvenanceSource: provenance,
		Dockerfile:       "Dockerfile",
		ImageReference:   "registry.example.invalid/shop:candidate",
		Platforms:        []string{"linux/amd64", "linux/arm64"},
		Output:           OutputPush,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed {
		t.Fatal("build was not reported as pushed")
	}
	arguments := diagnostics.String()
	if !strings.HasSuffix(strings.TrimSpace(arguments), "prepared/rootfs") {
		t.Fatalf("prepared directory is not the build context: %s", arguments)
	}
	if strings.Contains(arguments, provenance) {
		t.Fatalf("provenance source was passed as the build context: %s", arguments)
	}
}

func TestRunnerRejectsInvalidLocalPushProvenance(t *testing.T) {
	tests := []struct {
		name       string
		provenance string
		message    string
	}{
		{name: "missing", message: "require an immutable provenance source"},
		{name: "partial checksum", provenance: "https://example.invalid/shop.git?checksum=abc123", message: "full commit checksum"},
		{name: "insecure", provenance: "http://example.invalid/shop.git?checksum=" + strings.Repeat("a", 40), message: "valid HTTPS provenance source"},
		{name: "credentials", provenance: "https://user@example.invalid/shop.git?checksum=" + strings.Repeat("a", 40), message: "valid HTTPS provenance source"},
		{name: "query credentials", provenance: "https://example.invalid/shop.git?checksum=" + strings.Repeat("a", 40) + "&token=secret", message: "unsupported query parameters"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := NewRunner(io.Discard)
			called := false
			runner.command = func(context.Context, string, ...string) *exec.Cmd { called = true; return nil }
			_, err := runner.Build(context.Background(), Request{
				Source:           Source{LocalDirectory: "."},
				ProvenanceSource: test.provenance,
				Dockerfile:       "Dockerfile",
				ImageReference:   "registry.example.invalid/shop:candidate",
				Platforms:        []string{"linux/amd64"},
				Output:           OutputPush,
			})
			if err == nil || !strings.Contains(err.Error(), test.message) || called {
				t.Fatalf("error=%v called=%v", err, called)
			}
		})
	}
}

func TestRunnerReturnsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("build stopped"))
	runner := NewRunner(io.Discard)
	runner.command = helperCommand(t, "success")
	_, err := runner.Build(ctx, Request{Source: Source{LocalDirectory: "."}, Dockerfile: "Dockerfile", ImageReference: "test:latest", Platforms: []string{"linux/amd64"}, Output: OutputLoad})
	if err == nil || err.Error() != "build stopped" {
		t.Fatalf("unexpected cancellation error: %v", err)
	}
}

func TestRunnerRejectsUnsafeRequestsBeforeExecution(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Request)
		message string
	}{
		{name: "both sources", mutate: func(r *Request) { r.Source.GitURL = "https://example.invalid/repo.git?checksum=abc" }, message: "exactly one source"},
		{name: "multi-platform load", mutate: func(r *Request) { r.Platforms = append(r.Platforms, "linux/arm64") }, message: "exactly one platform"},
		{name: "invalid secret ID", mutate: func(r *Request) { r.Secrets = []Secret{{ID: "bad,id", Value: []byte("value")}} }, message: "invalid BuildKit secret"},
		{name: "duplicate secret", mutate: func(r *Request) {
			r.Secrets = []Secret{{ID: "auth", Value: []byte("one")}, {ID: "auth", Value: []byte("two")}}
		}, message: "duplicate BuildKit secret"},
		{name: "local push", mutate: func(r *Request) { r.Output = OutputPush }, message: "immutable provenance source"},
		{name: "secret build argument", mutate: func(r *Request) { r.BuildArgs = map[string]string{"API_TOKEN": "value"} }, message: "secret-bearing"},
		{name: "invalid builder", mutate: func(r *Request) { r.Builder = "bad builder" }, message: "builder name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := Request{Source: Source{LocalDirectory: "."}, Dockerfile: "Dockerfile", ImageReference: "test:latest", Platforms: []string{"linux/amd64"}, Output: OutputLoad}
			test.mutate(&request)
			runner := NewRunner(io.Discard)
			called := false
			runner.command = func(context.Context, string, ...string) *exec.Cmd { called = true; return nil }
			_, err := runner.Build(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), test.message) || called {
				t.Fatalf("error=%v called=%v", err, called)
			}
		})
	}
}

func TestRunnerRejectsMalformedMetadata(t *testing.T) {
	for _, mode := range []string{"missing-digest", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			runner := NewRunner(io.Discard)
			runner.command = helperCommand(t, mode)
			_, err := runner.Build(context.Background(), Request{Source: Source{LocalDirectory: "."}, Dockerfile: "Dockerfile", ImageReference: "test:latest", Platforms: []string{"linux/amd64"}, Output: OutputLoad})
			if err == nil {
				t.Fatal("expected metadata error")
			}
		})
	}
}

func helperCommand(t *testing.T, mode string) commandFactory {
	t.Helper()
	return func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestBuildKitHelperProcess", "--", mode)
		cmd.Env = append(os.Environ(), "MAGELIFT_BUILDKIT_HELPER=1", "MAGELIFT_BUILDKIT_ARGS="+strings.Join(args, "\x1f"))
		return cmd
	}
}

func TestBuildKitHelperProcess(t *testing.T) {
	if os.Getenv("MAGELIFT_BUILDKIT_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	args := strings.Split(os.Getenv("MAGELIFT_BUILDKIT_ARGS"), "\x1f")
	_, _ = fmt.Fprintln(os.Stdout, strings.Join(args, "\n"))
	metadataIndex := slices.Index(args, "--metadata-file")
	if metadataIndex < 0 || metadataIndex+1 >= len(args) {
		os.Exit(2)
	}
	metadata := map[string]string{}
	if mode != "missing-digest" {
		metadata["containerimage.digest"] = "sha256:" + strings.Repeat("a", 64)
	}
	contents, _ := json.Marshal(metadata)
	if mode == "trailing" {
		contents = append(contents, []byte("{}")...)
	}
	if err := os.WriteFile(args[metadataIndex+1], contents, 0o600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
