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
	"path/filepath"
	"regexp"
	"strings"
	"time"

	buildcontext "github.com/magelift/magelift"
)

const alphaPlatform = "linux/amd64"

var (
	imageRepositoryPattern = regexp.MustCompile(`^(?:[a-z0-9]+(?:[._-][a-z0-9]+)*)(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*)?$`)
	metadataDigestPattern  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type dockerBuildRunner interface {
	Run(ctx context.Context, diagnostics io.Writer, args []string) error
}

type defaultDockerBuildRunner struct{}

func (defaultDockerBuildRunner) Run(ctx context.Context, diagnostics io.Writer, args []string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	if diagnostics != nil {
		cmd.Stderr = diagnostics
	}
	cmd.Stdout = io.Discard
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Run(); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return err
	}
	return nil
}

var dockerBuild = dockerBuildRunner(defaultDockerBuildRunner{})

var imageInspectCommand Command = SystemCommandRunner()

// BuildImages builds the community-alpha builder and runtime images from the
// embedded private context. An empty imagePrefix loads fingerprinted local
// tags; a non-empty prefix must be a registry-qualified repository without tag
// or digest and is used to derive <prefix>:<fingerprint>-builder and
// <prefix>:<fingerprint>-runtime pushed references.
func BuildImages(ctx context.Context, diagnostics io.Writer, tempRoot, platform, imagePrefix string) (Images, error) {
	if diagnostics == nil {
		return Images{}, errors.New("image build diagnostics writer is required")
	}
	if platform != alphaPlatform {
		return Images{}, fmt.Errorf("unsupported platform %q: community alpha builds %s only", platform, alphaPlatform)
	}
	push := imagePrefix != ""
	if push {
		if err := validateImagePrefix(imagePrefix); err != nil {
			return Images{}, err
		}
	}

	contextDir, fingerprint, cleanup, err := buildcontext.MaterializePrivateContext(tempRoot, platform)
	if err != nil {
		return Images{}, err
	}
	defer cleanup()

	tagFingerprint := fingerprint
	if !push {
		// Keep concurrent invocations from moving each other's local FROM tags.
		tagFingerprint += "-" + strings.TrimPrefix(filepath.Base(contextDir), "magelift-buildcontext-")
	}
	builderTag, runtimeTag, err := imageTags(imagePrefix, tagFingerprint)
	if err != nil {
		return Images{}, err
	}

	metadataRoot := tempRoot
	if metadataRoot == "" {
		metadataRoot = os.TempDir()
	}
	metadataDir, err := os.MkdirTemp(metadataRoot, "magelift-image-metadata-")
	if err != nil {
		return Images{}, fmt.Errorf("create private metadata directory: %w", err)
	}
	defer os.RemoveAll(metadataDir)

	builderMetadata := filepath.Join(metadataDir, "builder.json")
	if err := runImageBuild(ctx, diagnostics, contextDir, "builder", builderTag, builderMetadata, push); err != nil {
		return Images{}, fmt.Errorf("build builder image: %w", err)
	}
	runtimeMetadata := filepath.Join(metadataDir, "runtime.json")
	if err := runImageBuild(ctx, diagnostics, contextDir, "runtime", runtimeTag, runtimeMetadata, push); err != nil {
		return Images{}, fmt.Errorf("build runtime image: %w", err)
	}

	if push {
		builderDigest, err := readBuildMetadataDigest(builderMetadata)
		if err != nil {
			return Images{}, fmt.Errorf("builder image metadata: %w", err)
		}
		runtimeDigest, err := readBuildMetadataDigest(runtimeMetadata)
		if err != nil {
			return Images{}, fmt.Errorf("runtime image metadata: %w", err)
		}
		builderRepository := imagePrefix
		runtimeRepository := imagePrefix
		return Images{
			BuilderID:        builderDigest,
			BuilderReference: builderRepository + "@" + builderDigest,
			RuntimeID:        runtimeDigest,
			RuntimeReference: runtimeRepository + "@" + runtimeDigest,
		}, nil
	}

	builderID, err := inspectImageID(ctx, builderTag)
	if err != nil {
		return Images{}, fmt.Errorf("inspect builder image: %w", err)
	}
	runtimeID, err := inspectImageID(ctx, runtimeTag)
	if err != nil {
		return Images{}, fmt.Errorf("inspect runtime image: %w", err)
	}
	return Images{
		BuilderID:        builderID,
		RuntimeID:        runtimeID,
		RuntimeReference: runtimeTag,
	}, nil
}

func imageTags(imagePrefix, fingerprint string) (string, string, error) {
	if imagePrefix == "" {
		builderRepo, _, ok := strings.Cut(DefaultBuilderTag, ":")
		if !ok || builderRepo == "" {
			return "", "", errors.New("default builder tag is invalid")
		}
		runtimeRepo, _, ok := strings.Cut(DefaultRuntimeTag, ":")
		if !ok || runtimeRepo == "" {
			return "", "", errors.New("default runtime tag is invalid")
		}
		return builderRepo + ":" + fingerprint, runtimeRepo + ":" + fingerprint, nil
	}
	return imagePrefix + ":" + fingerprint + "-builder", imagePrefix + ":" + fingerprint + "-runtime", nil
}

func validateImagePrefix(prefix string) error {
	if prefix == "" || strings.ContainsAny(prefix, "\x00\r\n\t ") {
		return errors.New("image prefix must be a non-empty value without whitespace")
	}
	if strings.ContainsAny(prefix, "@;|&$`\"'<>") {
		return errors.New("image prefix must be a registry-qualified repository without tag or digest")
	}
	if strings.Contains(prefix, "..") || strings.HasPrefix(prefix, "/") || strings.HasSuffix(prefix, "/") {
		return errors.New("image prefix must be a registry-qualified repository without tag or digest")
	}
	first, remainder, hasSlash := strings.Cut(prefix, "/")
	if !hasSlash {
		return errors.New("image prefix must be a registry-qualified repository without tag or digest")
	}
	if strings.Contains(remainder, ":") {
		return errors.New("image prefix must be a registry-qualified repository without tag or digest")
	}
	if strings.Count(first, ":") > 1 {
		return errors.New("image prefix must be a registry-qualified repository without tag or digest")
	}
	if !registryHostValid(first) {
		return errors.New("image prefix must include a registry host")
	}
	if !imageRepositoryPattern.MatchString(remainder) {
		return errors.New("image prefix repository path is invalid")
	}
	return nil
}

func registryHostValid(host string) bool {
	if host == "localhost" {
		return true
	}
	return strings.ContainsAny(host, ".:") && !strings.Contains(host, "/")
}

func runImageBuild(ctx context.Context, diagnostics io.Writer, contextDir, target, tag, metadataPath string, push bool) error {
	args := []string{
		"buildx", "build",
		"--file", filepath.Join(contextDir, "images", "php-nginx", "Dockerfile"),
		"--target", target,
		"--platform", alphaPlatform,
		"--metadata-file", metadataPath,
		"--progress", "plain",
		"--tag", tag,
	}
	if push {
		args = append(args, "--sbom=true", "--provenance=mode=max,version=v1", "--push")
	} else {
		args = append(args, "--provenance=false", "--sbom=false", "--load")
	}
	args = append(args, contextDir)
	return dockerBuild.Run(ctx, diagnostics, args)
}

func readBuildMetadataDigest(path string) (string, error) {
	const maxMetadataBytes = 4 << 20
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("open metadata: %w", err)
	}
	if info.Size() > maxMetadataBytes {
		return "", errors.New("metadata exceeds the size limit")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("metadata must be a regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read metadata: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var metadata struct {
		Digest string `json:"containerimage.digest"`
	}
	if err := decoder.Decode(&metadata); err != nil {
		return "", fmt.Errorf("decode metadata: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("metadata contains trailing data")
	}
	if !metadataDigestPattern.MatchString(metadata.Digest) {
		return "", errors.New("metadata does not contain a valid immutable image digest")
	}
	return metadata.Digest, nil
}

func inspectImageID(ctx context.Context, reference string) (string, error) {
	output, err := imageInspectCommand.Run(ctx, "docker", "image", "inspect", "--format={{.Id}}", reference)
	if err != nil {
		return "", err
	}
	if len(output) > maxImageIDBytes {
		return "", fmt.Errorf("inspect %q returned an oversized Docker image ID", reference)
	}
	id := stripSingleLineEnding(string(output))
	if !imageIDPattern.MatchString(id) {
		return "", fmt.Errorf("inspect %q returned an invalid Docker image ID", reference)
	}
	return id, nil
}
