package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	buildcontext "github.com/magelift/magelift"
	buildkit "github.com/magelift/magelift/internal/build/kit"
	buildpipeline "github.com/magelift/magelift/internal/build/pipeline"
	"github.com/magelift/magelift/internal/cosign"
	"github.com/magelift/magelift/internal/secretref"
	"github.com/magelift/magelift/internal/source"
	"github.com/magelift/magelift/internal/toolchain"
	"github.com/spf13/cobra"
)

const defaultBuildPlatform = "linux/amd64"

const devPublicVersion = "0.0.0-dev"

var publicVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

// ComposerSecretProvider resolves Composer credentials from supported secret
// reference backends.
type ComposerSecretProvider interface {
	secretref.SecretsManagerProvider
	secretref.ParameterStoreProvider
}

type composerSecretProvider = ComposerSecretProvider

func writeCloudPatchesBuildNotice(stderr io.Writer, enabled bool) {
	if !enabled || stderr == nil {
		return
	}
	fmt.Fprintln(stderr, "magelift: build.cloudPatches delegates one locked ece-patches apply: required cloud patches, declared Quality Patch IDs and project hotfixes.")
}

func buildCommand(o *options) *cobra.Command {
	var push bool
	var imageReference string
	var builderImage string
	var runtimeImage string
	var sourceURL string
	var exportRecipe string
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
		writeCloudPatchesBuildNotice(o.stderr, spec.Build.CloudPatches)
		if err := validateDefaultImageRecipe(spec.Build.PHP, string(spec.Application.WebRuntime), builderImage, runtimeImage); err != nil {
			return invalid(err)
		}
		if exportRecipe != "" {
			if push || imageReference != "" || builderImage != "" || runtimeImage != "" || sourceURL != "" {
				return invalid(errors.New("--export-recipe cannot be combined with image or push overrides"))
			}
			platform, err := resolveLocalPlatform(platforms)
			if err != nil {
				return invalid(err)
			}
			result, err := exportBuildRecipe(exportRecipe, platform)
			if err != nil {
				return invalid(err)
			}
			return o.write(result)
		}
		defaults, err := file.ProjectDefaults()
		if err != nil {
			return invalid(err)
		}
		if err := requireDependencies(cmd.Context(), o, toolchain.SpecsForBuild(push), "local build"); err != nil {
			return err
		}
		repository, err := (source.GitInspector{}).Inspect(cmd.Context(), filepath.Dir(o.configPath))
		if err != nil {
			return invalid(err)
		}
		resolver, err := imageResolver(builderImage, runtimeImage)
		if err != nil {
			return invalid(err)
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
		tempRoot, err := buildTempRoot(artifactDirectory)
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
		request.BuildCache = spec.Build.Cache
		if cache := request.BuildCache; cache != nil {
			for _, ref := range append(append([]string(nil), cache.From...), cache.To) {
				if ref != "" && ref == request.ImageReference {
					return invalid(errors.New("build cache must use a different reference from the application image"))
				}
			}
		}
		mageLiftVersion, err := normalizePublicVersion(Version)
		if err != nil {
			return invalid(err)
		}
		var images toolchain.Images
		if builderImage == "" && runtimeImage == "" {
			// Intermediates stay on the runner. Only the final app is published.
			var cacheFrom []string
			if spec.Build.Cache != nil {
				cacheFrom = spec.Build.Cache.From
			}
			images, err = toolchain.BuildImages(cmd.Context(), o.stderr, tempRoot, defaultBuildPlatform, "", cacheFrom...)
			request.UseEmbeddedRecipe = true
			if err == nil {
				resolver = toolchain.NewResolver(toolchain.SystemCommandRunner(), images.BuilderID, images.RuntimeReference)
			}
		} else {
			images, err = resolver.Resolve(cmd.Context())
		}
		if err != nil {
			return &exitError{code: 3, err: err}
		}
		applyResolvedImages(&request, images)
		containerPlatform := request.Platform
		if containerPlatform == "" && len(request.Platforms) > 0 {
			containerPlatform = request.Platforms[0]
		}
		builder := buildkit.NewRunner(o.stderr)
		builder.TempRoot = tempRoot
		pipeline := buildpipeline.New(
			buildpipeline.ContainerAdapter{
				TempRoot:        tempRoot,
				Stderr:          o.stderr,
				Network:         "bridge",
				Platform:        containerPlatform,
				MageLiftVersion: mageLiftVersion,
			},
			builder,
		)
		request.Config = file
		request.MageLiftVersion = mageLiftVersion
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
			if err := o.signDigest(cmd.Context(), reference, ""); err != nil {
				_ = os.Remove(result.Manifest)
				return &exitError{code: 3, err: fmt.Errorf("sign pushed image: %w", err)}
			}
			return o.write(result)
		}
		verifiedImages, err := resolver.Resolve(cmd.Context())
		if err != nil || !sameResolvedRuntime(images, verifiedImages) {
			_ = os.Remove(result.Manifest)
			if err != nil {
				return &exitError{code: 3, err: fmt.Errorf("verify local runtime image after build: %w", err)}
			}
			return &exitError{code: 3, err: errors.New("local runtime image changed during the build")}
		}
		return o.write(result)
	}}
	command.Flags().StringVar(&exportRecipe, "export-recipe", "", "export the bundled build recipe without building or publishing")
	command.Flags().BoolVar(&push, "push", false, "push the image and publish build attestations")
	command.Flags().StringVar(&imageReference, "image", "", "target image reference for a pushed build")
	command.Flags().StringVar(&builderImage, "builder-image", "", "advanced builder image override (pair with --runtime-image; registry digest required with --push)")
	command.Flags().StringVar(&runtimeImage, "runtime-image", "", "advanced runtime image override (pair with --builder-image; registry digest required with --push)")
	command.Flags().StringVar(&sourceURL, "source-url", "", "HTTPS Git source URL used for provenance")
	command.Flags().StringSliceVar(&platforms, "platform", nil, "target platform, repeatable or comma-separated")
	return command
}

func normalizePublicVersion(version string) (string, error) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" || strings.EqualFold(trimmed, "dev") {
		return devPublicVersion, nil
	}
	if strings.HasPrefix(trimmed, "v") || strings.HasPrefix(trimmed, "V") {
		trimmed = trimmed[1:]
	}
	if !publicVersionPattern.MatchString(trimmed) {
		return "", fmt.Errorf("CLI version %q is not a semantic version", version)
	}
	return trimmed, nil
}

func imageResolver(builderImage, runtimeImage string) (*toolchain.Resolver, error) {
	if builderImage == "" && runtimeImage == "" {
		return toolchain.New(), nil
	}
	if builderImage == "" || runtimeImage == "" {
		return nil, errors.New("--builder-image and --runtime-image must be set together")
	}
	return toolchain.NewResolver(toolchain.SystemCommandRunner(), builderImage, runtimeImage), nil
}

func validateDefaultImageRecipe(php, webRuntime, builderImage, runtimeImage string) error {
	if builderImage == "" && runtimeImage == "" && (php != toolchain.DefaultPHPBranch || webRuntime != toolchain.DefaultWebRuntime) {
		return fmt.Errorf("default community images require PHP %s and %s; other recipes need explicit --builder-image and --runtime-image overrides", toolchain.DefaultPHPBranch, toolchain.DefaultWebRuntime)
	}
	return nil
}

func applyResolvedImages(request *buildpipeline.Request, images toolchain.Images) {
	if !toolchain.IsImmutableReference(request.BuilderImage) {
		if images.BuilderReference != "" {
			request.BuilderImage = images.BuilderReference
		} else {
			request.BuilderImage = images.BuilderID
		}
	}
	if request.RuntimeImage == "" {
		if toolchain.IsImmutableReference(images.RuntimeReference) {
			request.RuntimeImage = images.RuntimeReference
			request.RuntimeImageID = ""
		} else {
			request.RuntimeImage = images.RuntimeReference
			if request.RuntimeImage == "" {
				request.RuntimeImage = toolchain.DefaultRuntimeTag
			}
			request.RuntimeImageID = images.RuntimeID
		}
	}
	if !toolchain.IsImmutableReference(request.RuntimeImage) {
		// Advanced local tags are checked against the captured image ID again
		// after the build; pushed builds still require repository digests.
		request.RuntimeImageID = images.RuntimeID
	}
}

func sameResolvedRuntime(before, after toolchain.Images) bool {
	if toolchain.IsImmutableReference(before.RuntimeReference) {
		return before.RuntimeReference == after.RuntimeReference && before.RuntimeID == after.RuntimeID
	}
	return before.RuntimeID == after.RuntimeID
}

// applicationImageRepository keeps base images in the same user-owned repository.
func applicationImageRepository(reference string) (string, error) {
	if strings.ContainsAny(reference, "@\x00\r\n\t ") {
		return "", errors.New("--image requires a registry-qualified repository and explicit tag")
	}
	lastSlash := strings.LastIndexByte(reference, '/')
	lastColon := strings.LastIndexByte(reference, ':')
	if lastSlash < 1 || lastColon <= lastSlash || lastColon == len(reference)-1 {
		return "", errors.New("--image requires a registry-qualified repository and explicit tag")
	}
	repository, tag := reference[:lastColon], reference[lastColon+1:]
	host := reference[:strings.IndexByte(reference, '/')]
	if host != "localhost" && !strings.ContainsAny(host, ".:") {
		return "", errors.New("--image must name the user's registry")
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9.:-]*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+$`).MatchString(repository) || !regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`).MatchString(tag) {
		return "", errors.New("--image has an invalid repository or tag")
	}
	return repository, nil
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
		if options.ImageReference != "" || options.SourceURL != "" {
			return buildpipeline.Request{}, errors.New("release image and source flags require --push")
		}
		platform, err := resolveLocalPlatform(options.Platforms)
		if err != nil {
			return buildpipeline.Request{}, err
		}
		if platform != defaultBuildPlatform && (options.BuilderImage == "" || options.RuntimeImage == "") {
			return buildpipeline.Request{}, errors.New("non-amd64 builds require explicit builder/runtime images and are outside the community alpha qualification")
		}
		request.ImageReference = "magelift/" + options.ProjectName + ":" + options.Repository.Revision[:12]
		request.Platform = platform
		request.BuilderImage = options.BuilderImage
		request.RuntimeImage = options.RuntimeImage
		return request, nil
	}
	if options.ImageReference == "" {
		return buildpipeline.Request{}, errors.New("--push requires --image")
	}
	if _, err := applicationImageRepository(options.ImageReference); err != nil {
		return buildpipeline.Request{}, err
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
	request.Platforms, err = resolveReleasePlatforms(options.Platforms)
	if err != nil {
		return buildpipeline.Request{}, err
	}
	for _, platform := range request.Platforms {
		if platform != defaultBuildPlatform && (options.BuilderImage == "" || options.RuntimeImage == "") {
			return buildpipeline.Request{}, errors.New("non-amd64 builds require explicit builder/runtime images and are outside the community alpha qualification")
		}
	}
	return request, nil
}

func resolveLocalPlatform(platforms []string) (string, error) {
	flattened := flattenPlatforms(platforms)
	if len(flattened) == 0 {
		return defaultBuildPlatform, nil
	}
	if len(flattened) != 1 {
		return "", errors.New("local builds require exactly one --platform")
	}
	return compatibleBuildPlatform(flattened[0])
}

func resolveReleasePlatforms(platforms []string) ([]string, error) {
	flattened := flattenPlatforms(platforms)
	if len(flattened) == 0 {
		return []string{defaultBuildPlatform}, nil
	}
	resolved := make([]string, 0, len(flattened))
	for _, platform := range flattened {
		normalized, err := compatibleBuildPlatform(platform)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, normalized)
	}
	return resolved, nil
}

func flattenPlatforms(platforms []string) []string {
	flattened := make([]string, 0, len(platforms))
	for _, entry := range platforms {
		for _, part := range strings.Split(entry, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				flattened = append(flattened, part)
			}
		}
	}
	return flattened
}

func compatibleBuildPlatform(platform string) (string, error) {
	switch platform {
	case defaultBuildPlatform, "linux/arm64":
		return platform, nil
	default:
		return "", fmt.Errorf("unsupported build platform %q for the community alpha recipe (default: %s; explicit ARM images are advanced overrides)", platform, defaultBuildPlatform)
	}
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
	if strings.HasPrefix(rawReference, "env://") {
		name := strings.TrimPrefix(rawReference, "env://")
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(name) {
			return nil, errors.New("Composer environment reference must name one variable")
		}
		value, present := os.LookupEnv(name)
		if !present || value == "" {
			return nil, errors.New("Composer credentials environment variable is unset or empty")
		}
		if len(value) > secretref.MaxValueSize {
			return nil, secretref.ErrValueTooLarge
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(value), &object); err != nil || len(object) == 0 {
			return nil, errors.New("Composer credentials environment value must be a non-empty JSON object")
		}
		return []byte(value), nil
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
		if o == nil || o.newComposerGCPSecrets == nil {
			return nil, errors.New("Composer credentials resolution is not configured for this CLI")
		}
		provider, err := o.newComposerGCPSecrets(ctx)
		if err != nil {
			return nil, fmt.Errorf("initialize GCP secret provider: %w", err)
		}
		return resolveComposerCredentials(ctx, reference, provider)
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
	resolver := secretref.Resolver{}
	switch reference.Kind {
	case secretref.GCPSecretManager:
		resolver.GCPSecretManager = provider
	case secretref.ParameterStore:
		resolver.ParameterStore = provider
	default:
		resolver.SecretsManager = provider
		resolver.ParameterStore = provider
	}
	value, err := resolver.Resolve(ctx, reference)
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

func buildTempRoot(artifactDirectory string) (string, error) {
	root := filepath.Join(filepath.Dir(artifactDirectory), "tmp")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create private build temp directory: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect private build temp directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("private build temp directory must be a mode 700 directory")
	}

	return root, nil
}

// exportBuildRecipe writes only versioned inputs and refuses an occupied target.
func exportBuildRecipe(destination, platform string) (map[string]string, error) {
	fingerprint, err := buildcontext.ContentFingerprint(platform)
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return nil, err
	}
	if err := os.Mkdir(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("recipe destination must not exist: %w", err)
	}
	if err := buildcontext.WriteContext(absolute); err != nil {
		os.RemoveAll(absolute)
		return nil, err
	}
	return map[string]string{"directory": absolute, "recipeSha256": fingerprint, "platform": platform}, nil
}
