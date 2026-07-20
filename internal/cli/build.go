package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	buildkit "github.com/acourtiol/magelift/internal/build/kit"
	buildpipeline "github.com/acourtiol/magelift/internal/build/pipeline"
	"github.com/acourtiol/magelift/internal/cosign"
	"github.com/acourtiol/magelift/internal/secretref"
	"github.com/acourtiol/magelift/internal/source"
	"github.com/acourtiol/magelift/internal/toolchain"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/spf13/cobra"
)

type composerSecretProvider interface {
	secretref.SecretsManagerProvider
	secretref.ParameterStoreProvider
}

func buildCommand(o *options) *cobra.Command {
	var push bool
	var imageReference string
	var builderImage string
	var runtimeImage string
	var sourceURL string
	var platforms []string
	command := &cobra.Command{Use: "build", Short: "Build an immutable Magento application image", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		file, err := o.load()
		if err != nil {
			return invalid(err)
		}
		spec, err := file.ResolveBuild()
		if err != nil {
			return invalid(err)
		}
		defaults, err := file.ProjectDefaults()
		if err != nil {
			return invalid(err)
		}
		repository, err := (source.GitInspector{}).Inspect(cmd.Context(), filepath.Dir(o.configPath))
		if err != nil {
			return invalid(err)
		}
		var images toolchain.Images
		if !push {
			images, err = toolchain.New().Resolve(cmd.Context())
			if err != nil {
				return &exitError{code: 3, err: err}
			}
		}
		composerAuth, err := o.loadComposerCredentials(cmd.Context(), spec.Build.Composer.Credentials, defaults.Region)
		if err != nil {
			return &exitError{code: 3, err: err}
		}
		defer clear(composerAuth)
		artifactDirectory, err := artifactDirectory()
		if err != nil {
			return err
		}
		request, err := buildPipelineRequest(buildRequestOptions{
			Push:              push,
			ImageReference:    imageReference,
			BuilderImage:      builderImage,
			RuntimeImage:      runtimeImage,
			SourceURL:         sourceURL,
			Platforms:         platforms,
			Repository:        repository,
			ProjectName:       spec.Project.Name,
			ArtifactDirectory: artifactDirectory,
			ComposerAuth:      composerAuth,
		})
		if err != nil {
			return invalid(err)
		}
		pipeline := buildpipeline.New(
			buildpipeline.ContainerAdapter{Stderr: o.stderr, Network: "bridge"},
			buildkit.NewRunner(o.stderr),
		)
		request.Config = file
		request.MageLiftVersion = Version
		if !push {
			request.BuilderImage = images.BuilderID
			request.RuntimeImage = toolchain.DefaultRuntimeTag
			request.RuntimeImageID = images.RuntimeID
		}
		result, err := pipeline.Run(cmd.Context(), request)
		if err != nil {
			return &exitError{code: 3, err: err}
		}
		if push {
			reference, err := digestReference(result.Image.ImageReference, result.Image.Digest)
			if err != nil {
				_ = os.Remove(result.Manifest)
				return &exitError{code: 3, err: err}
			}
			if err := cosign.New().Sign(cmd.Context(), reference); err != nil {
				_ = os.Remove(result.Manifest)
				return &exitError{code: 3, err: fmt.Errorf("sign pushed image: %w", err)}
			}
			return o.write(result)
		}
		verifiedImages, err := toolchain.New().Resolve(cmd.Context())
		if err != nil || verifiedImages.RuntimeID != images.RuntimeID {
			_ = os.Remove(result.Manifest)
			if err != nil {
				return &exitError{code: 3, err: fmt.Errorf("verify local runtime image after build: %w", err)}
			}
			return &exitError{code: 3, err: errors.New("local runtime image changed during the build")}
		}
		return o.write(result)
	}}
	command.Flags().BoolVar(&push, "push", false, "push the image and publish build attestations")
	command.Flags().StringVar(&imageReference, "image", "", "target image reference for a pushed build")
	command.Flags().StringVar(&builderImage, "builder-image", "", "builder image pinned by registry digest")
	command.Flags().StringVar(&runtimeImage, "runtime-image", "", "runtime image pinned by registry digest")
	command.Flags().StringVar(&sourceURL, "source-url", "", "HTTPS Git source URL used for provenance")
	command.Flags().StringSliceVar(&platforms, "platform", nil, "target platform, repeatable or comma-separated")
	return command
}

func digestReference(imageReference, digest string) (string, error) {
	name, _, _ := strings.Cut(imageReference, "@")
	lastSlash := strings.LastIndexByte(name, '/')
	if tag := strings.LastIndexByte(name, ':'); tag > lastSlash {
		name = name[:tag]
	}
	reference := name + "@" + digest
	if err := cosign.ValidateReference(reference); err != nil {
		return "", err
	}
	return reference, nil
}

type buildRequestOptions struct {
	Push              bool
	ImageReference    string
	BuilderImage      string
	RuntimeImage      string
	SourceURL         string
	Platforms         []string
	Repository        source.Repository
	ProjectName       string
	ArtifactDirectory string
	ComposerAuth      []byte
}

func buildPipelineRequest(options buildRequestOptions) (buildpipeline.Request, error) {
	if len(options.Repository.Revision) < 12 {
		return buildpipeline.Request{}, errors.New("inspected source revision is invalid")
	}
	request := buildpipeline.Request{
		Repository:        options.Repository,
		ArtifactDirectory: options.ArtifactDirectory,
		ComposerAuth:      options.ComposerAuth,
	}
	if !options.Push {
		if options.ImageReference != "" || options.BuilderImage != "" || options.RuntimeImage != "" || options.SourceURL != "" || len(options.Platforms) != 0 {
			return buildpipeline.Request{}, errors.New("release image, source, and platform flags require --push")
		}
		platform, err := localLinuxPlatform()
		if err != nil {
			return buildpipeline.Request{}, err
		}
		request.ImageReference = "magelift/" + options.ProjectName + ":" + options.Repository.Revision[:12]
		request.Platform = platform
		return request, nil
	}
	if options.ImageReference == "" || options.BuilderImage == "" || options.RuntimeImage == "" {
		return buildpipeline.Request{}, errors.New("--push requires --image, --builder-image, and --runtime-image")
	}
	baseSource := options.SourceURL
	if baseSource == "" {
		baseSource = options.Repository.OriginURL
	}
	provenanceSource, err := immutableSourceURL(baseSource, options.Repository.Revision)
	if err != nil {
		return buildpipeline.Request{}, err
	}
	request.Output = buildkit.OutputPush
	request.ImageReference = options.ImageReference
	request.BuilderImage = options.BuilderImage
	request.RuntimeImage = options.RuntimeImage
	request.ProvenanceSource = provenanceSource
	request.Platforms = append([]string(nil), options.Platforms...)
	if len(request.Platforms) == 0 {
		request.Platforms = []string{"linux/amd64", "linux/arm64"}
	}
	return request, nil
}

func immutableSourceURL(raw, revision string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("release builds require an HTTPS Git origin or --source-url")
	}
	query := parsed.Query()
	if len(query) > 1 {
		return "", errors.New("source URL may only contain a checksum query parameter")
	}
	for name := range query {
		if name != "checksum" {
			return "", errors.New("source URL may only contain a checksum query parameter")
		}
	}
	if values := query["checksum"]; len(values) != 0 && (len(values) != 1 || values[0] != revision) {
		return "", errors.New("source URL checksum does not match the inspected revision")
	}
	query.Set("checksum", revision)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (o *options) loadComposerCredentials(ctx context.Context, rawReference, region string) ([]byte, error) {
	if rawReference == "" {
		return nil, nil
	}
	reference, err := secretref.Parse(rawReference)
	if err != nil {
		return nil, fmt.Errorf("parse Composer credentials reference: %w", err)
	}
	switch reference.Kind {
	case secretref.SecretsManager, secretref.ParameterStore:
		region, err = secretRegion(reference, region)
		if err != nil {
			return nil, err
		}
		if o == nil || o.newComposerSecrets == nil {
			return nil, errors.New("Composer credentials resolution is not configured for this CLI")
		}
		provider, err := o.newComposerSecrets(ctx, region)
		if err != nil {
			return nil, fmt.Errorf("initialize secret provider: %w", err)
		}
		return resolveComposerCredentials(ctx, reference, provider)
	case secretref.GCPSecretManager:
		return nil, errors.New("gcp-secret-manager Composer credentials resolution is not implemented yet; use a custom build path or contribute the GCP Secret Manager adapter")
	default:
		return nil, fmt.Errorf("unsupported Composer credentials scheme %q", reference.Kind)
	}
}

func secretRegion(reference secretref.Reference, defaultRegion string) (string, error) {
	if !arn.IsARN(reference.ID) {
		if len(reference.ID) >= 4 && reference.ID[:4] == "arn:" {
			return "", errors.New("secret reference contains an invalid AWS ARN")
		}
		return defaultRegion, nil
	}
	parsed, err := arn.Parse(reference.ID)
	if err != nil || parsed.Region == "" {
		return "", errors.New("secret reference contains an invalid AWS ARN")
	}
	wantService := "secretsmanager"
	if reference.Kind == secretref.ParameterStore {
		wantService = "ssm"
	}
	if parsed.Service != wantService {
		return "", fmt.Errorf("secret reference ARN must use the %s service", wantService)
	}
	return parsed.Region, nil
}

func resolveComposerCredentials(ctx context.Context, reference secretref.Reference, provider composerSecretProvider) ([]byte, error) {
	value, err := (secretref.Resolver{SecretsManager: provider, ParameterStore: provider}).Resolve(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("resolve Composer credentials: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil || len(object) == 0 {
		clear(value)
		return nil, errors.New("resolved Composer credentials must be a non-empty JSON object")
	}
	return value, nil
}

func artifactDirectory() (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache: %w", err)
	}
	directory := filepath.Join(cacheRoot, "magelift", "artifacts")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create artifact directory: %w", err)
	}
	return directory, nil
}

func localLinuxPlatform() (string, error) {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return "linux/" + runtime.GOARCH, nil
	default:
		return "", fmt.Errorf("local builds do not support architecture %q", runtime.GOARCH)
	}
}
