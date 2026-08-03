package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	buildkit "github.com/magelift/magelift/internal/build/kit"
	buildrunner "github.com/magelift/magelift/internal/build/runner"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/containerrunner"
	"github.com/magelift/magelift/internal/source"
)

const pipelineDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const pinnedBuilder = "ghcr.io/magelift/magelift-builder@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const pinnedRuntime = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

func TestPipelineCoordinatesBuildAndPublishesVerifiedManifest(t *testing.T) {
	request := pipelineRequest(t)
	events := []string{}
	containers := &fakeContainers{t: t, events: &events}
	builder := &fakeBuilder{t: t, events: &events}

	result, err := New(containers, builder).Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "prepare,build,finalize" {
		t.Fatalf("events = %v", events)
	}
	if result.Image.Digest != pipelineDigest || result.ManifestSHA256 == "" {
		t.Fatalf("result = %#v", result)
	}
	contents, err := os.ReadFile(result.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != containers.manifest {
		t.Fatalf("manifest = %q", contents)
	}
	artifacts, err := filepath.EvalSymlinks(request.ArtifactDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(result.Manifest) != artifacts {
		t.Fatalf("manifest was not published externally: %q (artifact dir %q)", result.Manifest, artifacts)
	}
	if containers.prepareImage != pinnedBuilder || containers.finalizeImage != pinnedBuilder {
		t.Fatal("builder image was not bound to both container stages")
	}
	if len(containers.prepareSecrets) != 1 || containers.prepareSecrets[0].ID != "composer-auth" {
		t.Fatalf("prepare secrets = %#v", containers.prepareSecrets)
	}
	if len(containers.finalizeSecrets) != 0 {
		t.Fatalf("finalize received secrets: %#v", containers.finalizeSecrets)
	}
}

func TestPipelineRejectsPinsAndArtifactDirectoryInsideSource(t *testing.T) {
	request := pipelineRequest(t)
	request.BuilderImage = "builder:latest"
	_, err := New(&fakeContainers{t: t}, &fakeBuilder{t: t}).Run(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("unexpected pin error: %v", err)
	}

	request = pipelineRequest(t)
	request.ArtifactDirectory = request.Repository.Root
	_, err = New(&fakeContainers{t: t}, &fakeBuilder{t: t}).Run(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("unexpected artifact directory error: %v", err)
	}
}

func TestPipelineAcceptsBothImmutableImageReferenceForms(t *testing.T) {
	request := pipelineRequest(t)
	request.BuilderImage = "sha256:" + strings.Repeat("d", 64)
	request.RuntimeImage = "ghcr.io/magelift/runtime@sha256:" + strings.Repeat("e", 64)
	containers := &fakeContainers{t: t}
	builder := &fakeBuilder{t: t, expectedRuntime: request.RuntimeImage}

	if _, err := New(containers, builder).Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineRetainsLocalRuntimeTagIdentity(t *testing.T) {
	request := pipelineRequest(t)
	request.RuntimeImage = "magelift/php-runtime:local"
	request.RuntimeImageID = "sha256:" + strings.Repeat("e", 64)
	builder := &fakeBuilder{t: t, expectedRuntime: request.RuntimeImage}

	if _, err := New(&fakeContainers{t: t}, builder).Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if builder.request.Output != buildkit.OutputLoad {
		t.Fatalf("local output = %q", builder.request.Output)
	}
}

func TestPipelineCoordinatesReleaseBuild(t *testing.T) {
	request := pipelineRequest(t)
	request.Output = buildkit.OutputPush
	request.Platform = ""
	request.Platforms = []string{"linux/amd64", "linux/arm64"}
	request.BuilderImage = "ghcr.io/magelift/magelift-builder@sha256:" + strings.Repeat("d", 64)
	request.RuntimeImage = "ghcr.io/magelift/magelift-runtime@sha256:" + strings.Repeat("e", 64)
	request.ImageReference = "ghcr.io/magelift/shop:revision"
	request.ProvenanceSource = "https://github.com/acourtiol/shop.git?checksum=" + request.Repository.Revision
	request.MageLiftVersion = "1.2.3"
	builder := &fakeBuilder{t: t, expectedRuntime: request.RuntimeImage, expectedOutput: buildkit.OutputPush}

	result, err := New(&fakeContainers{t: t}, builder).Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Image.Pushed {
		t.Fatal("release result was not marked as pushed")
	}
	got := builder.request
	if got.Output != buildkit.OutputPush || strings.Join(got.Platforms, ",") != "linux/amd64,linux/arm64" {
		t.Fatalf("release output = %q, platforms = %v", got.Output, got.Platforms)
	}
	if got.ProvenanceSource != request.ProvenanceSource || got.Source.GitURL != "" || got.Source.LocalDirectory == "" {
		t.Fatalf("release source = %#v, provenance = %q", got.Source, got.ProvenanceSource)
	}
	if got.BuildArgs["SOURCE_URI"] != request.ProvenanceSource || got.BuildArgs["MAGELIFT_VERSION"] != "1.2.3" {
		t.Fatalf("release labels = %#v", got.BuildArgs)
	}
	if !strings.Contains(string(applicationDockerfile), "org.opencontainers.image.source") || !strings.Contains(string(applicationDockerfile), "org.opencontainers.image.version") {
		t.Fatal("embedded Dockerfile does not declare release labels")
	}
}

func TestPipelineRejectsInvalidReleaseInputsBeforePrepare(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Request)
		want   string
	}{
		{"local builder ID", func(request *Request) { request.BuilderImage = "sha256:" + strings.Repeat("d", 64) }, "repository digest"},
		{"local runtime tag", func(request *Request) {
			request.RuntimeImage = "runtime:local"
			request.RuntimeImageID = "sha256:" + strings.Repeat("e", 64)
		}, "repository digest"},
		{"unqualified target", func(request *Request) { request.ImageReference = "shop:revision" }, "registry-qualified"},
		{"missing provenance", func(request *Request) { request.ProvenanceSource = "" }, "provenance"},
		{"mutable provenance", func(request *Request) { request.ProvenanceSource = "https://github.com/acourtiol/shop.git" }, "checksum"},
		{"wrong revision", func(request *Request) {
			request.ProvenanceSource = "https://github.com/acourtiol/shop.git?checksum=" + strings.Repeat("f", 40)
		}, "match"},
		{"query credentials", func(request *Request) {
			request.ProvenanceSource += "&token=secret"
		}, "checksum"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := releaseRequest(t)
			test.mutate(&request)
			containers := &fakeContainers{t: t, events: &[]string{}}
			_, err := New(containers, &fakeBuilder{t: t, expectedOutput: buildkit.OutputPush}).Run(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run() error = %v, want %q", err, test.want)
			}
			if len(*containers.events) != 0 {
				t.Fatalf("container ran before validation: %v", *containers.events)
			}
		})
	}
}

func TestPipelineDefaultsToLocalLoadAndLegacyPlatform(t *testing.T) {
	request := pipelineRequest(t)
	request.Output = ""
	request.Platforms = nil
	builder := &fakeBuilder{t: t}
	if _, err := New(&fakeContainers{t: t}, builder).Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if builder.request.Output != buildkit.OutputLoad || len(builder.request.Platforms) != 1 || builder.request.Platforms[0] != request.Platform {
		t.Fatalf("local defaults = output %q platforms %v", builder.request.Output, builder.request.Platforms)
	}
	if _, exists := builder.request.BuildArgs["SOURCE_URI"]; exists {
		t.Fatal("local build received an empty SOURCE_URI argument")
	}
	if _, exists := builder.request.BuildArgs["MAGELIFT_VERSION"]; exists {
		t.Fatal("local build received an empty MAGELIFT_VERSION argument")
	}
}

func TestPipelineRejectsStrictProtocolAndManifestMismatches(t *testing.T) {
	for _, test := range []struct {
		name       string
		containers func(*testing.T) *fakeContainers
		message    string
	}{
		{
			name: "unknown response field",
			containers: func(t *testing.T) *fakeContainers {
				return &fakeContainers{t: t, malformedPrepare: true}
			},
			message: "unknown field",
		},
		{
			name: "digest mismatch",
			containers: func(t *testing.T) *fakeContainers {
				return &fakeContainers{t: t, mismatchedDigest: true}
			},
			message: "digest does not match",
		},
		{
			name: "manifest checksum mismatch",
			containers: func(t *testing.T) *fakeContainers {
				return &fakeContainers{t: t, mismatchedChecksum: true}
			},
			message: "checksum does not match",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.containers(t), &fakeBuilder{t: t}).Run(context.Background(), pipelineRequest(t))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPipelineRejectsNonPrivatePrepareOutput(t *testing.T) {
	containers := &fakeContainers{t: t, publicOutput: true}
	_, err := New(containers, &fakeBuilder{t: t}).Run(context.Background(), pipelineRequest(t))
	if err == nil || !strings.Contains(err.Error(), "private directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeContainers struct {
	t                  *testing.T
	events             *[]string
	prepareImage       string
	finalizeImage      string
	prepareSecrets     []containerrunner.Secret
	finalizeSecrets    []containerrunner.Secret
	output             string
	manifest           string
	malformedPrepare   bool
	mismatchedDigest   bool
	mismatchedChecksum bool
	publicOutput       bool
}

func (fake *fakeContainers) Run(_ context.Context, image, _ string, payload []byte, secrets ...containerrunner.Secret) (containerrunner.Result, error) {
	fake.record("prepare")
	fake.prepareImage = image
	fake.prepareSecrets = append([]containerrunner.Secret(nil), secrets...)
	request, err := buildrunner.DecodeRequest(strings.NewReader(string(payload)))
	if err != nil {
		fake.t.Fatal(err)
	}
	if request.Stage != buildrunner.StagePrepare {
		fake.t.Fatalf("stage = %s", request.Stage)
	}
	if request.Prepare.RepositoryRoot != "/workspace" {
		fake.t.Fatalf("runner repository root = %q", request.Prepare.RepositoryRoot)
	}
	fake.output = fake.t.TempDir()
	if err := os.Chmod(fake.output, 0o700); err != nil {
		fake.t.Fatal(err)
	}
	if fake.publicOutput {
		if err := os.Chmod(fake.output, 0o755); err != nil {
			fake.t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(fake.output, "rootfs"), 0o700); err != nil {
		fake.t.Fatal(err)
	}
	preparedPath := filepath.Join("prepared", request.Prepare.SourceRevision+".json")
	if err := os.Mkdir(filepath.Join(fake.output, "prepared"), 0o700); err != nil {
		fake.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fake.output, preparedPath), []byte(`{"prepared":true}`), 0o600); err != nil {
		fake.t.Fatal(err)
	}
	if fake.malformedPrepare {
		return containerrunner.Result{OutputDir: fake.output, Response: []byte(`{"protocolVersion":1,"stage":"prepare","prepare":{},"extra":true}`)}, nil
	}
	response, err := buildrunner.EncodeResponse(buildrunner.Response{
		ProtocolVersion: buildrunner.ProtocolVersion,
		Stage:           buildrunner.StagePrepare,
		Prepare: &buildrunner.PrepareResponse{
			PreparedArtifact:            filepath.ToSlash(preparedPath),
			PHPVersion:                  "8.5.4",
			PHPExtensions:               []string{"intl"},
			EnabledModules:              []string{"Magento_Catalog"},
			Checksums:                   []buildrunner.FileChecksum{{Path: "vendor/autoload.php", SHA256: strings.Repeat("d", 64)}},
			RequiredRuntimeCapabilities: []string{"database.mysql"},
		},
	})
	if err != nil {
		fake.t.Fatal(err)
	}
	return containerrunner.Result{OutputDir: fake.output, Response: response}, nil
}

func (fake *fakeContainers) RunInOutput(_ context.Context, image, _ string, output string, payload []byte, secrets ...containerrunner.Secret) (containerrunner.Result, error) {
	fake.record("finalize")
	fake.finalizeImage = image
	fake.finalizeSecrets = append([]containerrunner.Secret(nil), secrets...)
	request, err := buildrunner.DecodeRequest(strings.NewReader(string(payload)))
	if err != nil {
		fake.t.Fatal(err)
	}
	fake.manifest = `{"imageDigest":"` + request.Finalize.ImageDigest + `"}`
	manifestDirectory := filepath.Join(output, "manifests")
	if err := os.Mkdir(manifestDirectory, 0o700); err != nil {
		fake.t.Fatal(err)
	}
	manifestPath := filepath.Join(manifestDirectory, request.Finalize.SourceRevision+".json")
	if err := os.WriteFile(manifestPath, []byte(fake.manifest), 0o600); err != nil {
		fake.t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(fake.manifest))
	checksum := hex.EncodeToString(sum[:])
	if fake.mismatchedChecksum {
		checksum = strings.Repeat("e", 64)
	}
	digest := request.Finalize.ImageDigest
	if fake.mismatchedDigest {
		digest = "sha256:" + strings.Repeat("f", 64)
	}
	response, err := buildrunner.EncodeResponse(buildrunner.Response{
		ProtocolVersion: buildrunner.ProtocolVersion,
		Stage:           buildrunner.StageFinalize,
		Finalize: &buildrunner.FinalizeResponse{
			ImageDigest:    digest,
			ManifestPath:   filepath.ToSlash(filepath.Join("manifests", filepath.Base(manifestPath))),
			ManifestSHA256: checksum,
		},
	})
	if err != nil {
		fake.t.Fatal(err)
	}
	return containerrunner.Result{OutputDir: output, Response: response}, nil
}

func (fake *fakeContainers) record(event string) {
	if fake.events != nil {
		*fake.events = append(*fake.events, event)
	}
}

type fakeBuilder struct {
	t               *testing.T
	events          *[]string
	expectedRuntime string
	expectedOutput  buildkit.OutputMode
	request         buildkit.Request
}

func (fake *fakeBuilder) Build(_ context.Context, request buildkit.Request) (buildkit.Result, error) {
	fake.request = request
	if fake.events != nil {
		*fake.events = append(*fake.events, "build")
	}
	expectedOutput := fake.expectedOutput
	if expectedOutput == "" {
		expectedOutput = buildkit.OutputLoad
	}
	if request.Output != expectedOutput || len(request.Platforms) == 0 {
		fake.t.Fatalf("build request output = %#v", request)
	}
	expectedRuntime := fake.expectedRuntime
	if expectedRuntime == "" {
		expectedRuntime = pinnedRuntime
	}
	if request.BuildArgs["RUNTIME_BASE"] != expectedRuntime {
		fake.t.Fatalf("runtime pin = %q", request.BuildArgs["RUNTIME_BASE"])
	}
	contents, err := os.ReadFile(request.Dockerfile)
	if err != nil {
		fake.t.Fatal(err)
	}
	if string(contents) != string(applicationDockerfile) || filepath.Dir(request.Dockerfile) != request.Source.LocalDirectory {
		fake.t.Fatal("BuildKit did not receive the embedded Dockerfile in the private context")
	}
	return buildkit.Result{ImageReference: request.ImageReference, Digest: pipelineDigest, Pushed: request.Output == buildkit.OutputPush}, nil
}

func releaseRequest(t *testing.T) Request {
	t.Helper()
	request := pipelineRequest(t)
	request.Output = buildkit.OutputPush
	request.Platforms = []string{"linux/amd64", "linux/arm64"}
	request.BuilderImage = "ghcr.io/magelift/magelift-builder@sha256:" + strings.Repeat("d", 64)
	request.RuntimeImage = "ghcr.io/magelift/magelift-runtime@sha256:" + strings.Repeat("e", 64)
	request.ImageReference = "ghcr.io/magelift/shop:revision"
	request.ProvenanceSource = "https://github.com/acourtiol/shop.git?checksum=" + request.Repository.Revision
	return request
}

func pipelineRequest(t *testing.T) Request {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.lock"), []byte(`{"packages":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := config.Load([]byte(`schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.9, mode: integrated}
build: {php: "8.5"}
target: {provider: aws, runtime: ecs-fargate}
defaults: {region: eu-west-3, preset: preview}
environments:
  staging: {account: "123456789012"}
`))
	if err != nil {
		t.Fatal(err)
	}
	return Request{
		Repository:        source.Repository{Root: root, Revision: strings.Repeat("a", 40)},
		Config:            file,
		BuilderImage:      pinnedBuilder,
		RuntimeImage:      pinnedRuntime,
		ImageReference:    "magelift-test:revision",
		Platform:          "linux/amd64",
		ArtifactDirectory: t.TempDir(),
		ComposerAuth:      []byte(`{"github-oauth":{"github.com":"token"}}`),
	}
}
