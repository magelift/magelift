package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	buildkit "github.com/magelift/magelift/internal/build/kit"
	buildplan "github.com/magelift/magelift/internal/build/plan"
	buildrunner "github.com/magelift/magelift/internal/build/runner"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/containerrunner"
	"github.com/magelift/magelift/internal/source"
	"github.com/magelift/magelift/sdk"
)

const maxManifestBytes int64 = 4 << 20

var pinnedImage = regexp.MustCompile(`^(?:sha256:[a-f0-9]{64}|[^\s@]+@sha256:[a-f0-9]{64})$`)
var repositoryDigest = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)
var sourceRevision = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
var localImageTag = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*:[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var hexDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

//go:embed assets/application.Dockerfile
var applicationDockerfile []byte

type Container interface {
	Run(context.Context, string, string, []byte, ...containerrunner.Secret) (containerrunner.Result, error)
	RunInOutput(context.Context, string, string, string, []byte, ...containerrunner.Secret) (containerrunner.Result, error)
}

type Builder interface {
	Build(context.Context, buildkit.Request) (buildkit.Result, error)
}

type Request struct {
	Repository        source.Repository
	Config            *config.File
	BuilderImage      string
	RuntimeImage      string
	RuntimeImageID    string
	ImageReference    string
	Platform          string
	Platforms         []string
	Output            buildkit.OutputMode
	ProvenanceSource  string
	MageLiftVersion   string
	ArtifactDirectory string
	ComposerAuth      []byte
}

type Result struct {
	Image          buildkit.Result `json:"image" yaml:"image"`
	Manifest       string          `json:"manifest" yaml:"manifest"`
	ManifestSHA256 string          `json:"manifestSha256" yaml:"manifestSha256"`
	Runtime        RuntimeContract `json:"runtime" yaml:"runtime"`
}

// ImmutableArtifactContract converts a pushed pipeline result into the
// provider-neutral artifact identity used by certification reuse. The build
// pipeline proves the image and manifest digests; signing remains an explicit
// release step, so its opaque reference is supplied by the caller.
func (result Result) ImmutableArtifactContract(inputFingerprint, provenanceReference, signatureReference string) (sdk.ImmutableArtifactContract, error) {
	if !result.Image.Pushed {
		return sdk.ImmutableArtifactContract{}, errors.New("immutable artifact contract requires a confirmed image push")
	}
	baseReference := result.Image.ImageReference
	if base, _, found := strings.Cut(baseReference, "@"); found {
		baseReference = base
	}
	if !registryQualifiedReference(baseReference) {
		return sdk.ImmutableArtifactContract{}, errors.New("immutable artifact contract requires a registry-qualified image reference")
	}
	if !digestPattern.MatchString(result.Image.Digest) {
		return sdk.ImmutableArtifactContract{}, errors.New("immutable artifact contract requires an image SHA-256 digest")
	}
	manifestDigest := result.ManifestSHA256
	if !strings.HasPrefix(manifestDigest, "sha256:") {
		if !hexDigestPattern.MatchString(manifestDigest) {
			return sdk.ImmutableArtifactContract{}, errors.New("immutable artifact contract requires a manifest SHA-256 digest")
		}
		manifestDigest = "sha256:" + manifestDigest
	}
	contract := sdk.ImmutableArtifactContract{
		ImageDigest:         baseReference + "@" + result.Image.Digest,
		ManifestDigest:      manifestDigest,
		InputFingerprint:    inputFingerprint,
		ProvenanceReference: provenanceReference,
		SignatureReference:  signatureReference,
	}
	if err := contract.Validate(); err != nil {
		return sdk.ImmutableArtifactContract{}, err
	}
	return contract, nil
}

// RuntimeContract is the resolved runtime that the isolated builder proved
// before the application image was created. It is kept alongside the manifest
// path so callers can publish the same contract in certification evidence.
type RuntimeContract struct {
	PHPVersion      string   `json:"phpVersion" yaml:"phpVersion"`
	PHPExtensions   []string `json:"phpExtensions" yaml:"phpExtensions"`
	ComposerVersion string   `json:"composerVersion" yaml:"composerVersion"`
}

type Pipeline struct {
	containers Container
	builder    Builder
}

func New(containers Container, builder Builder) *Pipeline {
	return &Pipeline{containers: containers, builder: builder}
}

func (pipeline *Pipeline) Run(ctx context.Context, request Request) (result Result, err error) {
	artifactDirectory, output, platforms, err := validateRequest(request, pipeline.containers, pipeline.builder)
	if err != nil {
		return Result{}, err
	}

	prepareRequest, err := buildplan.PrepareRequest(request.Config, request.Repository, "/workspace")
	if err != nil {
		return Result{}, fmt.Errorf("plan application build: %w", err)
	}
	prepareJSON, err := buildrunner.EncodeRequest(prepareRequest)
	if err != nil {
		return Result{}, err
	}
	secrets := []containerrunner.Secret{}
	if len(request.ComposerAuth) != 0 {
		secrets = append(secrets, containerrunner.Secret{ID: "composer-auth", Value: request.ComposerAuth})
	}
	prepared, err := pipeline.containers.Run(ctx, request.BuilderImage, request.Repository.Root, prepareJSON, secrets...)
	if err != nil {
		return Result{}, fmt.Errorf("prepare application filesystem: %w", err)
	}
	defer func() {
		if cleanupErr := prepared.Cleanup(); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove private build output: %w", cleanupErr))
		}
	}()
	if err := validatePrivateOutput(prepared.OutputDir, request.Repository.Root); err != nil {
		return Result{}, err
	}
	prepareResponse, err := buildrunner.DecodeResponse(bytes.NewReader(prepared.Response))
	if err != nil {
		return Result{}, fmt.Errorf("decode prepare response: %w", err)
	}
	if prepareResponse.Stage != buildrunner.StagePrepare || prepareResponse.Prepare == nil {
		return Result{}, errors.New("build runner returned the wrong prepare stage")
	}
	preparedExtensions, err := canonicalPHPExtensions(prepareResponse.Prepare.PHPExtensions)
	if err != nil {
		return Result{}, fmt.Errorf("normalize prepared PHP extensions: %w", err)
	}
	if err := validatePreparedRuntimeContract(prepareRequest.Prepare, prepareResponse.Prepare); err != nil {
		return Result{}, fmt.Errorf("validate prepared runtime contract: %w", err)
	}

	dockerfilePath := filepath.Join(prepared.OutputDir, "Dockerfile")
	if err := writeNewRegularFile(dockerfilePath, applicationDockerfile, 0o600); err != nil {
		return Result{}, fmt.Errorf("materialize application Dockerfile: %w", err)
	}
	buildArguments := map[string]string{
		"RUNTIME_BASE":    request.RuntimeImage,
		"SOURCE_REVISION": request.Repository.Revision,
	}
	if request.ProvenanceSource != "" {
		buildArguments["SOURCE_URI"] = request.ProvenanceSource
	}
	if request.MageLiftVersion != "" {
		buildArguments["MAGELIFT_VERSION"] = request.MageLiftVersion
	}
	image, err := pipeline.builder.Build(ctx, buildkit.Request{
		Source:           buildkit.Source{LocalDirectory: prepared.OutputDir},
		Dockerfile:       dockerfilePath,
		ImageReference:   request.ImageReference,
		Platforms:        platforms,
		Output:           output,
		ProvenanceSource: request.ProvenanceSource,
		Builder:          buildkitBuilder(),
		BuildArgs:        buildArguments,
	})
	if err != nil {
		return Result{}, fmt.Errorf("build application image: %w", err)
	}
	if output == buildkit.OutputPush && !image.Pushed {
		return Result{}, errors.New("BuildKit did not confirm the release image push")
	}

	finalizeRequest := buildrunner.Request{
		ProtocolVersion: buildrunner.ProtocolVersion,
		Stage:           buildrunner.StageFinalize,
		Finalize: &buildrunner.FinalizeRequest{
			PreparedArtifact: prepareResponse.Prepare.PreparedArtifact,
			SourceRevision:   request.Repository.Revision,
			ImageDigest:      image.Digest,
		},
	}
	finalizeJSON, err := buildrunner.EncodeRequest(finalizeRequest)
	if err != nil {
		return Result{}, err
	}
	finalized, err := pipeline.containers.RunInOutput(ctx, request.BuilderImage, request.Repository.Root, prepared.OutputDir, finalizeJSON)
	if err != nil {
		return Result{}, fmt.Errorf("finalize artifact manifest: %w", err)
	}
	finalizeResponse, err := buildrunner.DecodeResponse(bytes.NewReader(finalized.Response))
	if err != nil {
		return Result{}, fmt.Errorf("decode finalize response: %w", err)
	}
	if finalizeResponse.Stage != buildrunner.StageFinalize || finalizeResponse.Finalize == nil {
		return Result{}, errors.New("build runner returned the wrong finalize stage")
	}
	final := finalizeResponse.Finalize
	if final.ImageDigest != image.Digest {
		return Result{}, errors.New("finalized manifest digest does not match the built image")
	}
	manifest, err := verifiedOutputFile(prepared.OutputDir, final.ManifestPath, final.ManifestSHA256)
	if err != nil {
		return Result{}, err
	}
	destination := filepath.Join(artifactDirectory, filepath.Base(final.ManifestPath))
	if err := atomicCopy(destination, manifest); err != nil {
		return Result{}, fmt.Errorf("publish artifact manifest: %w", err)
	}
	return Result{
		Image:          image,
		Manifest:       destination,
		ManifestSHA256: final.ManifestSHA256,
		Runtime: RuntimeContract{
			PHPVersion:      prepareResponse.Prepare.PHPVersion,
			PHPExtensions:   append([]string(nil), preparedExtensions...),
			ComposerVersion: prepareResponse.Prepare.ComposerVersion,
		},
	}, nil
}

func validatePreparedRuntimeContract(request *buildrunner.PrepareRequest, response *buildrunner.PrepareResponse) error {
	if request == nil || response == nil {
		return errors.New("prepare request and response are required")
	}
	if !runtimeVersionMatches(request.PHPVersion, response.PHPVersion) {
		return fmt.Errorf("prepared PHP version %q does not satisfy requested %q", response.PHPVersion, request.PHPVersion)
	}
	if !runtimeVersionMatches(request.ComposerVersion, response.ComposerVersion) {
		return fmt.Errorf("prepared Composer version %q does not satisfy requested %q", response.ComposerVersion, request.ComposerVersion)
	}
	availableExtensions, err := canonicalPHPExtensions(response.PHPExtensions)
	if err != nil {
		return err
	}
	available := make(map[string]struct{}, len(availableExtensions))
	for _, extension := range availableExtensions {
		available[extension] = struct{}{}
	}
	missing := make([]string, 0)
	for _, extension := range request.PHPRequiredExtensions {
		extension = canonicalPHPExtensionName(extension)
		if _, ok := available[extension]; ok {
			continue
		}
		if extension == "opcache" {
			if _, ok := available["zend_opcache"]; ok {
				continue
			}
		}
		missing = append(missing, extension)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("prepared PHP runtime is missing requested extensions: %s", strings.Join(missing, ", "))
	}
	return nil
}

func canonicalPHPExtensions(values []string) ([]string, error) {
	canonical := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := canonicalPHPExtensionName(value)
		if name == "" {
			return nil, fmt.Errorf("prepared PHP runtime returned invalid extension %q", value)
		}
		key := name
		if key == "zend_opcache" {
			key = "opcache"
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("prepared PHP runtime returned duplicate extension %q", name)
		}
		seen[key] = struct{}{}
		canonical = append(canonical, name)
	}
	sort.Strings(canonical)
	return canonical, nil
}

func canonicalPHPExtensionName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	pendingSeparator := false
	for _, character := range value {
		valid := (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-'
		if valid {
			if pendingSeparator && builder.Len() > 0 {
				builder.WriteByte('_')
			}
			builder.WriteRune(character)
			pendingSeparator = false
			continue
		}
		if builder.Len() > 0 {
			pendingSeparator = true
		}
	}
	return builder.String()
}

func runtimeVersionMatches(requested, prepared string) bool {
	requested = strings.TrimSpace(requested)
	prepared = strings.TrimSpace(prepared)
	if requested == "" || prepared == "" {
		return requested == prepared
	}
	if strings.HasSuffix(requested, "+") {
		return numericVersionAtLeast(prepared, strings.TrimSuffix(requested, "+"))
	}
	return prepared == requested || strings.HasPrefix(prepared, requested+".")
}

func numericVersionAtLeast(got, want string) bool {
	gotParts := strings.Split(got, ".")
	wantParts := strings.Split(want, ".")
	for index := 0; index < len(gotParts) || index < len(wantParts); index++ {
		gotValue, wantValue := 0, 0
		if index < len(gotParts) {
			parsed, err := strconv.Atoi(gotParts[index])
			if err != nil {
				return false
			}
			gotValue = parsed
		}
		if index < len(wantParts) {
			parsed, err := strconv.Atoi(wantParts[index])
			if err != nil {
				return false
			}
			wantValue = parsed
		}
		if gotValue != wantValue {
			return gotValue > wantValue
		}
	}
	return true
}

func buildkitBuilder() string {
	return strings.TrimSpace(os.Getenv("BUILDX_BUILDER"))
}

func validateRequest(request Request, containers Container, builder Builder) (string, buildkit.OutputMode, []string, error) {
	if containers == nil || builder == nil {
		return "", "", nil, errors.New("build pipeline requires container and BuildKit runners")
	}
	if request.Config == nil {
		return "", "", nil, errors.New("build configuration is required")
	}
	if request.Repository.Root == "" || request.Repository.Revision == "" {
		return "", "", nil, errors.New("inspected source repository is required")
	}
	output := request.Output
	if output == "" {
		output = buildkit.OutputLoad
	}
	platforms := append([]string(nil), request.Platforms...)
	if len(platforms) == 0 && request.Platform != "" {
		platforms = []string{request.Platform}
	}
	if output != buildkit.OutputLoad && output != buildkit.OutputPush {
		return "", "", nil, errors.New("build output must be load or push")
	}
	if len(platforms) == 0 || output == buildkit.OutputLoad && len(platforms) != 1 {
		return "", "", nil, errors.New("local builds require one platform and release builds require at least one platform")
	}
	if !pinnedImage.MatchString(request.BuilderImage) {
		return "", "", nil, errors.New("builder image must be pinned by a lowercase SHA-256 digest")
	}
	if !pinnedImage.MatchString(request.RuntimeImage) && !(localImageTag.MatchString(request.RuntimeImage) && regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(request.RuntimeImageID)) {
		return "", "", nil, errors.New("runtime image must use a registry digest or a local tag with its immutable image ID")
	}
	if request.ImageReference == "" {
		return "", "", nil, errors.New("image reference is required")
	}
	if output == buildkit.OutputPush {
		if !repositoryDigest.MatchString(request.BuilderImage) || !repositoryDigest.MatchString(request.RuntimeImage) || request.RuntimeImageID != "" {
			return "", "", nil, errors.New("release builder and runtime images must use repository digest references")
		}
		if !registryQualifiedReference(request.ImageReference) {
			return "", "", nil, errors.New("release image reference must be registry-qualified")
		}
		if err := validateProvenanceSource(request.ProvenanceSource, request.Repository.Revision); err != nil {
			return "", "", nil, err
		}
	}
	root, err := filepath.EvalSymlinks(request.Repository.Root)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolve source repository: %w", err)
	}
	artifactDirectory, err := filepath.EvalSymlinks(request.ArtifactDirectory)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolve artifact directory: %w", err)
	}
	info, err := os.Stat(artifactDirectory)
	if err != nil || !info.IsDir() {
		return "", "", nil, errors.New("artifact directory must be an existing directory")
	}
	if pathContains(root, artifactDirectory) || pathContains(artifactDirectory, root) {
		return "", "", nil, errors.New("artifact directory must be outside the source tree")
	}
	return artifactDirectory, output, platforms, nil
}

func registryQualifiedReference(reference string) bool {
	first, _, found := strings.Cut(reference, "/")
	return found && (first == "localhost" || strings.ContainsAny(first, ".:"))
}

func validateProvenanceSource(raw, revision string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("release builds require an immutable HTTPS provenance source")
	}
	checksums := parsed.Query()["checksum"]
	if len(parsed.Query()) != 1 || len(checksums) != 1 || !sourceRevision.MatchString(checksums[0]) || checksums[0] != revision {
		return errors.New("release provenance source checksum must match the full source revision")
	}
	return nil
}

func validatePrivateOutput(output, sourceRoot string) error {
	resolved, err := filepath.EvalSymlinks(output)
	if err != nil {
		return fmt.Errorf("resolve private build output: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("container prepare output must be a private directory")
	}
	root, err := filepath.EvalSymlinks(sourceRoot)
	if err != nil {
		return fmt.Errorf("resolve source repository: %w", err)
	}
	if pathContains(root, resolved) || pathContains(resolved, root) {
		return errors.New("private build output must not overlap the source tree")
	}
	return nil
}

func verifiedOutputFile(output, relative, expectedSHA string) ([]byte, error) {
	path := filepath.Join(output, filepath.FromSlash(relative))
	rel, err := filepath.Rel(output, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errors.New("manifest path escapes private build output")
	}
	resolvedOutput, err := filepath.EvalSymlinks(output)
	if err != nil {
		return nil, fmt.Errorf("resolve private build output: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil || !pathContains(resolvedOutput, resolvedPath) {
		return nil, errors.New("manifest path escapes private build output through a symbolic link")
	}
	info, err := os.Lstat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("inspect finalized manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
		return nil, errors.New("finalized manifest must be a bounded regular file")
	}
	contents, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("read finalized manifest: %w", err)
	}
	sum := sha256.Sum256(contents)
	if hex.EncodeToString(sum[:]) != expectedSHA {
		return nil, errors.New("finalized manifest checksum does not match the runner response")
	}
	return contents, nil
}

func atomicCopy(destination string, contents []byte) error {
	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(directory, ".magelift-manifest-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destination)
}

func writeNewRegularFile(path string, contents []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
