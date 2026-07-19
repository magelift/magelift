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
	"strings"

	buildkit "github.com/acourtiol/magelift/internal/build/kit"
	buildplan "github.com/acourtiol/magelift/internal/build/plan"
	buildrunner "github.com/acourtiol/magelift/internal/build/runner"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/containerrunner"
	"github.com/acourtiol/magelift/internal/source"
)

const maxManifestBytes int64 = 4 << 20

var pinnedImage = regexp.MustCompile(`^(?:sha256:[a-f0-9]{64}|[^\s@]+@sha256:[a-f0-9]{64})$`)
var repositoryDigest = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)
var sourceRevision = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
var localImageTag = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*:[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

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
		Builder:          "default",
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
	return Result{Image: image, Manifest: destination, ManifestSHA256: final.ManifestSHA256}, nil
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
