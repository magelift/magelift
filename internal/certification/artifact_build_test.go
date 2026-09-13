package certification

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	buildkit "github.com/magelift/magelift/internal/build/kit"
	buildpipeline "github.com/magelift/magelift/internal/build/pipeline"
	"github.com/magelift/magelift/internal/cosign"
)

type artifactCommand struct {
	name string
	args []string
}

type artifactCommandRunner struct {
	mu       sync.Mutex
	commands []artifactCommand
	err      error
}

func (runner *artifactCommandRunner) Run(_ context.Context, name string, args ...string) error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.commands = append(runner.commands, artifactCommand{name: name, args: append([]string(nil), args...)})
	return runner.err
}

func (runner *artifactCommandRunner) Commands() []artifactCommand {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	commands := make([]artifactCommand, len(runner.commands))
	copy(commands, runner.commands)
	return commands
}

func testPipelineResult() buildpipeline.Result {
	return buildpipeline.Result{
		Image: buildkit.Result{
			ImageReference: "registry.example.invalid/magelift/shop:release",
			Digest:         "sha256:" + strings.Repeat("b", 64),
			Pushed:         true,
		},
		ManifestSHA256: strings.Repeat("c", 64),
	}
}

func testArtifactSigningOptions(client *cosign.Client) ArtifactSigningOptions {
	return ArtifactSigningOptions{
		ProvenanceReference: "https://example.invalid/provenance/commit-sha",
		SignatureReference:  "oci://registry.example.invalid/magelift/signature@sha256:" + strings.Repeat("d", 64),
		CertificateIdentity: "https://github.com/magelift/magelift/.github/workflows/release.yml@refs/heads/main",
		OIDCIssuer:          "https://token.actions.githubusercontent.com",
		Signer:              client,
		Verifier:            client,
	}
}

var (
	_ ArtifactSigner   = (*cosign.Client)(nil)
	_ ArtifactVerifier = (*cosign.Client)(nil)
)

func TestEnsurePipelineArtifactBuildsSignsVerifiesAndReusesImmutableResult(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	request := testArtifactRequest("magelift/test/pipeline-artifact")
	result := testPipelineResult()
	runner := &artifactCommandRunner{}
	client := cosign.NewWithRunner(runner)
	options := testArtifactSigningOptions(client)
	builds := 0
	build := func(context.Context) (buildpipeline.Result, error) {
		builds++
		return result, nil
	}
	first, reused, err := EnsurePipelineArtifact(context.Background(), registry, request, build, options)
	if err != nil || reused || builds != 1 {
		t.Fatalf("first pipeline artifact = %#v, reused=%v, builds=%d, err=%v", first, reused, builds, err)
	}
	if first.Artifact.ImageDigest != result.Image.ImageReference[:len(result.Image.ImageReference)-len(":release")]+"@"+result.Image.Digest {
		t.Fatalf("artifact image digest = %q", first.Artifact.ImageDigest)
	}
	if first.Artifact.ProvenanceReference != options.ProvenanceReference || first.Artifact.SignatureReference != options.SignatureReference {
		t.Fatalf("artifact references = %#v", first.Artifact)
	}
	commands := runner.Commands()
	if len(commands) != 2 {
		t.Fatalf("sign/verify commands = %#v", commands)
	}
	wantSign := artifactCommand{name: "cosign", args: []string{"sign", "--yes", first.Artifact.ImageDigest}}
	wantVerify := artifactCommand{name: "cosign", args: []string{"verify", "--certificate-identity", options.CertificateIdentity, "--certificate-oidc-issuer", options.OIDCIssuer, first.Artifact.ImageDigest}}
	if !reflect.DeepEqual(commands[0], wantSign) || !reflect.DeepEqual(commands[1], wantVerify) {
		t.Fatalf("sign/verify commands = %#v, want %#v and %#v", commands, wantSign, wantVerify)
	}
	second, reused, err := EnsurePipelineArtifact(context.Background(), registry, request, build, options)
	if err != nil || !reused || builds != 1 || second != first {
		t.Fatalf("reused pipeline artifact = %#v, reused=%v, builds=%d, err=%v", second, reused, builds, err)
	}
	if got := len(runner.Commands()); got != 2 {
		t.Fatalf("reuse reran signing commands: %d", got)
	}
}

func TestEnsurePipelineArtifactReusesPublishedRecordWithoutBuildOrSigner(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	request := testArtifactRequest("magelift/test/pipeline-artifact-reuse")
	runner := &artifactCommandRunner{}
	client := cosign.NewWithRunner(runner)
	build := func(context.Context) (buildpipeline.Result, error) { return testPipelineResult(), nil }
	first, reused, err := EnsurePipelineArtifact(context.Background(), registry, request, build, testArtifactSigningOptions(client))
	if err != nil || reused {
		t.Fatalf("first artifact = %#v, reused=%v, err=%v", first, reused, err)
	}
	second, reused, err := EnsurePipelineArtifact(context.Background(), registry, request, nil, ArtifactSigningOptions{})
	if err != nil || !reused || second != first {
		t.Fatalf("cached artifact = %#v, reused=%v, err=%v", second, reused, err)
	}
	if got := len(runner.Commands()); got != 2 {
		t.Fatalf("cached lookup invoked signer/verifier: %d commands", got)
	}
	unsafe := ArtifactSigningOptions{ProvenanceReference: "token=abcdefghijklmnop"}
	if _, _, err := EnsurePipelineArtifact(context.Background(), registry, request, nil, unsafe); err == nil || !strings.Contains(err.Error(), "secret material") {
		t.Fatalf("unsafe cached metadata error = %v", err)
	}
}

type artifactSignerFunc func(context.Context, string) error

func (function artifactSignerFunc) Sign(ctx context.Context, reference string) error {
	return function(ctx, reference)
}

type artifactVerifierFunc func(context.Context, string, string, string) error

func (function artifactVerifierFunc) VerifyArtifact(ctx context.Context, reference, identity, issuer string) error {
	return function(ctx, reference, identity, issuer)
}

func TestEnsurePipelineArtifactRedactsInjectedSigningErrorsAndReleasesClaim(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	request := testArtifactRequest("magelift/test/pipeline-artifact-secret")
	secret := "token=abcdefghijklmnop"
	options := ArtifactSigningOptions{
		ProvenanceReference: "https://example.invalid/provenance/commit-sha",
		SignatureReference:  "oci://registry.example.invalid/magelift/signature@sha256:" + strings.Repeat("d", 64),
		CertificateIdentity: "release-identity",
		OIDCIssuer:          "https://issuer.example.invalid",
		Signer: artifactSignerFunc(func(context.Context, string) error {
			return errors.New(secret)
		}),
		Verifier: artifactVerifierFunc(func(context.Context, string, string, string) error {
			t.Fatal("verifier ran after signing failed")
			return nil
		}),
	}
	_, _, err := EnsurePipelineArtifact(context.Background(), registry, request, func(context.Context) (buildpipeline.Result, error) {
		return testPipelineResult(), nil
	}, options)
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "sign immutable certification artifact") {
		t.Fatalf("signing error = %v", err)
	}
	if _, found, lookupErr := registry.Lookup(context.Background(), request.CompatibilityFingerprint); lookupErr != nil || found {
		t.Fatalf("failed signing publication found=%v err=%v", found, lookupErr)
	}
	if _, err := registry.Claim(context.Background(), request.CompatibilityFingerprint, "magelift/test/next-run"); err != nil {
		t.Fatalf("claim after failed signing = %v", err)
	}
}

func TestEnsurePipelineArtifactRedactsInjectedVerificationErrorsAndReleasesClaim(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	request := testArtifactRequest("magelift/test/pipeline-artifact-verify-secret")
	secret := "password=abcdefghijklmnop"
	options := ArtifactSigningOptions{
		ProvenanceReference: "https://example.invalid/provenance/commit-sha",
		SignatureReference:  "oci://registry.example.invalid/magelift/signature@sha256:" + strings.Repeat("d", 64),
		CertificateIdentity: "release-identity",
		OIDCIssuer:          "https://issuer.example.invalid",
		Signer:              artifactSignerFunc(func(context.Context, string) error { return nil }),
		Verifier: artifactVerifierFunc(func(context.Context, string, string, string) error {
			return errors.New(secret)
		}),
	}
	_, _, err := EnsurePipelineArtifact(context.Background(), registry, request, func(context.Context) (buildpipeline.Result, error) {
		return testPipelineResult(), nil
	}, options)
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "verify immutable certification artifact") {
		t.Fatalf("verification error = %v", err)
	}
	if _, found, lookupErr := registry.Lookup(context.Background(), request.CompatibilityFingerprint); lookupErr != nil || found {
		t.Fatalf("failed verification publication found=%v err=%v", found, lookupErr)
	}
	if _, err := registry.Claim(context.Background(), request.CompatibilityFingerprint, "magelift/test/next-verify-run"); err != nil {
		t.Fatalf("claim after failed verification = %v", err)
	}
}

func TestEnsurePipelineArtifactRejectsSecretLikeSigningMetadataBeforeBuild(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	request := testArtifactRequest("magelift/test/pipeline-artifact-invalid")
	builds := 0
	options := ArtifactSigningOptions{
		ProvenanceReference: "https://example.invalid/provenance/commit-sha",
		SignatureReference:  "oci://registry.example.invalid/magelift/signature@sha256:" + strings.Repeat("d", 64),
		CertificateIdentity: "release-identity",
		OIDCIssuer:          "https://issuer.example.invalid",
		Signer:              artifactSignerFunc(func(context.Context, string) error { return nil }),
		Verifier:            artifactVerifierFunc(func(context.Context, string, string, string) error { return nil }),
	}
	options.ProvenanceReference = "token=abcdefghijklmnop"
	_, _, err := EnsurePipelineArtifact(context.Background(), registry, request, func(context.Context) (buildpipeline.Result, error) {
		builds++
		return testPipelineResult(), nil
	}, options)
	if err == nil || !strings.Contains(err.Error(), "secret material") || builds != 0 {
		t.Fatalf("invalid signing metadata err=%v builds=%d", err, builds)
	}
}
