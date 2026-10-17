package toolchain

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

	buildcontext "github.com/magelift/magelift"
)

func TestBuildImagesLoadsLocalTagsAndInspectsIDs(t *testing.T) {
	fingerprint, err := buildcontext.ContentFingerprint(alphaPlatform)
	if err != nil {
		t.Fatal(err)
	}
	_ = "magelift/php-builder:" + fingerprint
	runtimeTag := "magelift/php-nginx:" + fingerprint
	builderID := "sha256:" + strings.Repeat("a", 64)
	runtimeID := "sha256:" + strings.Repeat("b", 64)

	var diagnostics bytes.Buffer
	var captured [][]string
	ids := map[string]string{}
	defer stubDockerBuildFunc(t, func(ctx context.Context, writer io.Writer, args []string) error {
		captured = append(captured, slices.Clone(args))
		tag := args[slices.Index(args, "--tag")+1]
		if args[slices.Index(args, "--target")+1] == "builder" {
			ids[tag] = builderID
		} else {
			ids[tag] = runtimeID
		}
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestBuildImagesHelperProcess", "--", "success")
		cmd.Env = append(os.Environ(), "MAGELIFT_BUILD_IMAGES_HELPER=1", "MAGELIFT_BUILD_IMAGES_ARGS="+strings.Join(args, "\x1f"))
		cmd.Stderr = writer
		return cmd.Run()
	})()
	defer stubImageInspect(t, ids)()

	images, err := BuildImages(context.Background(), &diagnostics, t.TempDir(), alphaPlatform, "")
	if err != nil {
		t.Fatal(err)
	}
	if images.BuilderReference != "" {
		t.Fatalf("local builder reference must be empty: %#v", images)
	}
	if !strings.HasPrefix(images.RuntimeReference, runtimeTag+"-") || images.BuilderID != builderID || images.RuntimeID != runtimeID {
		t.Fatalf("unexpected images: %#v", images)
	}
	if len(captured) != 2 {
		t.Fatalf("expected two build invocations, got %d", len(captured))
	}
	for _, args := range captured {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--load") || strings.Contains(joined, "--push") {
			t.Fatalf("expected local load build: %s", joined)
		}
		if !strings.Contains(joined, "--provenance=false") || !strings.Contains(joined, "--sbom=false") {
			t.Fatalf("expected disabled attestations for local build: %s", joined)
		}
	}
}

func TestBuildImagesPushesRegistryQualifiedPrefix(t *testing.T) {
	fingerprint, err := buildcontext.ContentFingerprint(alphaPlatform)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "registry.example.invalid/acme/magelift"
	builderDigest := "sha256:" + strings.Repeat("c", 64)
	runtimeDigest := "sha256:" + strings.Repeat("d", 64)

	var diagnostics bytes.Buffer
	var captured [][]string
	defer stubDockerBuildFunc(t, func(ctx context.Context, writer io.Writer, args []string) error {
		captured = append(captured, slices.Clone(args))
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestBuildImagesHelperProcess", "--", "push")
		cmd.Env = append(os.Environ(), "MAGELIFT_BUILD_IMAGES_HELPER=1", "MAGELIFT_BUILD_IMAGES_ARGS="+strings.Join(args, "\x1f"))
		cmd.Stderr = writer
		return cmd.Run()
	})()

	images, err := BuildImages(context.Background(), &diagnostics, t.TempDir(), alphaPlatform, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if images.BuilderReference != prefix+"@"+builderDigest || images.RuntimeReference != prefix+"@"+runtimeDigest {
		t.Fatalf("unexpected references: %#v", images)
	}
	if len(captured) != 2 {
		t.Fatalf("expected two build invocations, got %d", len(captured))
	}
	for _, args := range captured {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--push") || !strings.Contains(joined, "--sbom=true") || !strings.Contains(joined, "--provenance=mode=max,version=v1") {
			t.Fatalf("expected release attestations: %s", joined)
		}
	}
	if !strings.Contains(strings.Join(captured[0], " "), prefix+":"+fingerprint+"-builder") {
		t.Fatalf("expected fingerprinted builder tag: %#v", captured[0])
	}
	if !strings.Contains(strings.Join(captured[1], " "), prefix+":"+fingerprint+"-runtime") {
		t.Fatalf("expected fingerprinted runtime tag: %#v", captured[1])
	}
}

func TestBuildImagesRejectsMaliciousPrefixBeforeDocker(t *testing.T) {
	tests := []string{
		"registry.example.invalid/repo:tag",
		"registry.example.invalid/repo@sha256:" + strings.Repeat("a", 64),
		"registry.example.invalid/repo;rm",
		"short",
		"registry.example.invalid/../repo",
	}
	for _, prefix := range tests {
		t.Run(prefix, func(t *testing.T) {
			called := false
			restore := stubDockerBuildFunc(t, func(context.Context, io.Writer, []string) error {
				called = true
				return nil
			})
			defer restore()
			_, err := BuildImages(context.Background(), io.Discard, t.TempDir(), alphaPlatform, prefix)
			if err == nil || called {
				t.Fatalf("prefix=%q err=%v called=%v", prefix, err, called)
			}
		})
	}
}

func TestBuildImagesPropagatesBuildFailure(t *testing.T) {
	restore := stubDockerBuild(t, "failure", io.Discard)
	defer restore()
	_, err := BuildImages(context.Background(), io.Discard, t.TempDir(), alphaPlatform, "")
	if err == nil || !strings.Contains(err.Error(), "build builder image") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildImagesRejectsMalformedMetadata(t *testing.T) {
	for _, mode := range []string{"missing-digest", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			restore := stubDockerBuild(t, mode, io.Discard)
			defer restore()
			_, err := BuildImages(context.Background(), io.Discard, t.TempDir(), alphaPlatform, "registry.example.invalid/acme/shop")
			if err == nil {
				t.Fatal("expected metadata error")
			}
		})
	}
}

func TestValidateImagePrefix(t *testing.T) {
	if err := validateImagePrefix("registry.example.invalid/acme/magelift"); err != nil {
		t.Fatal(err)
	}
	if err := validateImagePrefix("localhost:5000/acme/magelift"); err != nil {
		t.Fatal(err)
	}
}

func stubDockerBuild(t *testing.T, mode string, diagnostics io.Writer) func() {
	return stubDockerBuildFunc(t, func(ctx context.Context, writer io.Writer, args []string) error {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestBuildImagesHelperProcess", "--", mode)
		cmd.Env = append(os.Environ(), "MAGELIFT_BUILD_IMAGES_HELPER=1", "MAGELIFT_BUILD_IMAGES_ARGS="+strings.Join(args, "\x1f"))
		cmd.Stderr = writer
		return cmd.Run()
	})
}

func stubDockerBuildFunc(t *testing.T, fn func(context.Context, io.Writer, []string) error) func() {
	t.Helper()
	previous := dockerBuild
	dockerBuild = dockerBuildFunc{fn: fn}
	return func() { dockerBuild = previous }
}

type dockerBuildFunc struct {
	fn func(context.Context, io.Writer, []string) error
}

func (runner dockerBuildFunc) Run(ctx context.Context, diagnostics io.Writer, args []string) error {
	return runner.fn(ctx, diagnostics, args)
}

func stubImageInspect(t *testing.T, ids map[string]string) func() {
	t.Helper()
	previous := imageInspectCommand
	imageInspectCommand = &inspectByReference{ids: ids}
	return func() { imageInspectCommand = previous }
}

type inspectByReference struct {
	ids map[string]string
}

func (command *inspectByReference) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if len(args) < 4 || args[0] != "image" || args[1] != "inspect" {
		return nil, errors.New("unexpected inspect invocation")
	}
	reference := args[len(args)-1]
	id, ok := command.ids[reference]
	if !ok {
		return nil, fmt.Errorf("missing image %q", reference)
	}
	return []byte(id + "\n"), nil
}

func TestBuildImagesHelperProcess(t *testing.T) {
	if os.Getenv("MAGELIFT_BUILD_IMAGES_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	args := strings.Split(os.Getenv("MAGELIFT_BUILD_IMAGES_ARGS"), "\x1f")
	if mode == "failure" {
		os.Exit(1)
	}
	metadataIndex := slices.Index(args, "--metadata-file")
	if metadataIndex < 0 || metadataIndex+1 >= len(args) {
		os.Exit(2)
	}
	push := slices.Contains(args, "--push")
	if push {
		metadata := map[string]string{}
		if mode != "missing-digest" {
			metadataPath := args[metadataIndex+1]
			if strings.HasSuffix(metadataPath, "builder.json") {
				metadata["containerimage.digest"] = "sha256:" + strings.Repeat("c", 64)
			} else {
				metadata["containerimage.digest"] = "sha256:" + strings.Repeat("d", 64)
			}
		}
		contents, _ := json.Marshal(metadata)
		if mode == "trailing" {
			contents = append(contents, []byte("{}")...)
		}
		if err := os.WriteFile(args[metadataIndex+1], contents, 0o600); err != nil {
			os.Exit(3)
		}
	}
	os.Exit(0)
}
