package toolchain

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

const (
	DefaultBuilderTag = "magelift/php-builder:local"
	DefaultRuntimeTag = "magelift/php-nginx:local"
	DefaultPHPBranch  = "8.5"
	DefaultWebRuntime = "nginx-fpm"

	// Release pins are filled only after publication and anonymous pull verification.
	DefaultPublishedBuilderImage = ""
	DefaultPublishedRuntimeImage = ""
)

const maxImageIDBytes = 80

var (
	imageIDPattern              = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	immutableImageReference     = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)
	ErrImageUnavailable         = errors.New("local Docker image is unavailable")
	ErrImmutableImageMissing    = errors.New("immutable Docker image is not available locally")
	ErrReleaseImagesUnavailable = errors.New("community alpha base images are not published yet; contributor qualification requires explicit --builder-image and --runtime-image overrides")
)

type Command interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type Images struct {
	BuilderID        string
	BuilderReference string
	RuntimeID        string
	RuntimeReference string
}

type Resolver struct {
	command     Command
	builderRef  string
	runtimeRef  string
	unavailable error
}

func New() *Resolver {
	return DefaultResolver(commandRunner{})
}

func DefaultResolver(command Command) *Resolver {
	if DefaultPublishedBuilderImage == "" || DefaultPublishedRuntimeImage == "" {
		return &Resolver{command: command, unavailable: ErrReleaseImagesUnavailable}
	}
	return NewResolver(command, DefaultPublishedBuilderImage, DefaultPublishedRuntimeImage)
}

func NewResolver(command Command, builderRef, runtimeRef string) *Resolver {
	return &Resolver{command: command, builderRef: builderRef, runtimeRef: runtimeRef}
}

func IsImmutableReference(reference string) bool {
	return immutableImageReference.MatchString(reference)
}

func (resolver *Resolver) Resolve(ctx context.Context) (Images, error) {
	if resolver.unavailable != nil {
		return Images{}, resolver.unavailable
	}
	if resolver.command == nil {
		return Images{}, errors.New("Docker image resolver requires a command runner")
	}
	if err := validateImageReference(resolver.builderRef); err != nil {
		return Images{}, fmt.Errorf("invalid builder image reference: %w", err)
	}
	if err := validateImageReference(resolver.runtimeRef); err != nil {
		return Images{}, fmt.Errorf("invalid runtime image reference: %w", err)
	}
	builderID, err := resolver.resolveImageID(ctx, resolver.builderRef)
	if err != nil {
		return Images{}, err
	}
	runtimeID, err := resolver.resolveImageID(ctx, resolver.runtimeRef)
	if err != nil {
		return Images{}, err
	}
	runtimeReference, err := resolver.resolveRuntimeReference(ctx, runtimeID)
	if err != nil {
		return Images{}, err
	}
	images := Images{
		BuilderID:        builderID,
		RuntimeID:        runtimeID,
		RuntimeReference: runtimeReference,
	}
	if IsImmutableReference(resolver.builderRef) {
		images.BuilderReference = resolver.builderRef
	}
	return images, nil
}

func (resolver *Resolver) resolveRuntimeReference(ctx context.Context, runtimeID string) (string, error) {
	if IsImmutableReference(resolver.runtimeRef) {
		if runtimeID == "" {
			return "", fmt.Errorf("inspect immutable runtime image reference %q: missing image ID", resolver.runtimeRef)
		}
		return resolver.runtimeRef, nil
	}
	output, err := resolver.command.Run(ctx, "docker", "image", "inspect", "--format={{index .RepoDigests 0}}", resolver.runtimeRef)
	if err != nil {
		return "", fmt.Errorf("inspect immutable runtime image reference %q: %w", resolver.runtimeRef, err)
	}
	reference := stripSingleLineEnding(string(output))
	if !immutableImageReference.MatchString(reference) {
		return "", fmt.Errorf("inspect %q returned an invalid immutable runtime image reference", resolver.runtimeRef)
	}
	return reference, nil
}

func (resolver *Resolver) resolveImageID(ctx context.Context, reference string) (string, error) {
	if IsImmutableReference(reference) {
		if err := resolver.ensureImmutableImage(ctx, reference); err != nil {
			return "", err
		}
	}
	output, err := resolver.command.Run(ctx, "docker", "image", "inspect", "--format={{.Id}}", reference)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("inspect local Docker image %q: %w", reference, ctx.Err())
		}
		detail := ""
		if len(output) <= 512 {
			if message := strings.TrimSpace(string(output)); message != "" {
				detail = ": " + message
			}
		}
		if IsImmutableReference(reference) {
			return "", fmt.Errorf(
				"%w: inspect %q after pull: %v%s",
				ErrImmutableImageMissing,
				reference,
				err,
				detail,
			)
		}
		return "", fmt.Errorf(
			"%w: inspect %q: %v%s; build or load the image locally before continuing",
			ErrImageUnavailable,
			reference,
			err,
			detail,
		)
	}
	if len(output) > maxImageIDBytes {
		return "", fmt.Errorf("inspect %q returned an oversized Docker image ID", reference)
	}
	id := stripSingleLineEnding(string(output))
	if !imageIDPattern.MatchString(id) {
		return "", fmt.Errorf(
			"inspect %q returned an invalid Docker image ID; expected exactly one lowercase sha256 image ID",
			reference,
		)
	}
	return id, nil
}

func (resolver *Resolver) ensureImmutableImage(ctx context.Context, reference string) error {
	_, err := resolver.command.Run(ctx, "docker", "image", "inspect", "--format={{.Id}}", reference)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("inspect immutable Docker image %q: %w", reference, ctx.Err())
	}
	output, pullErr := resolver.command.Run(ctx, "docker", "image", "pull", reference)
	if pullErr != nil {
		detail := ""
		if len(output) <= 512 {
			if message := strings.TrimSpace(string(output)); message != "" {
				detail = ": " + message
			}
		}
		return fmt.Errorf("pull immutable image %q: %w%s", reference, pullErr, detail)
	}
	return nil
}

func stripSingleLineEnding(value string) string {
	if strings.HasSuffix(value, "\r\n") {
		return strings.TrimSuffix(value, "\r\n")
	}
	return strings.TrimSuffix(value, "\n")
}

func validateImageReference(reference string) error {
	if reference == "" || strings.ContainsAny(reference, "\x00\r\n\t ") {
		return errors.New("reference must be a non-empty value without whitespace")
	}
	if IsImmutableReference(reference) {
		return nil
	}
	if strings.Contains(reference, "@") {
		return errors.New("mutable image references must use a tag, not a digest alias")
	}
	return nil
}

type commandRunner struct{}

func SystemCommandRunner() Command {
	return commandRunner{}
}

func (commandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
