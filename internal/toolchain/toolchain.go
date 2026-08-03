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
	DefaultRuntimeTag = "magelift/php-runtime:local"
	maxImageIDBytes   = 80
)

var (
	imageIDPattern      = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	ErrImageUnavailable = errors.New("local Docker image is unavailable")
)

type Command interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type Images struct {
	BuilderID        string
	RuntimeID        string
	RuntimeReference string
}

type Resolver struct {
	command    Command
	builderTag string
	runtimeTag string
}

func New() *Resolver {
	return NewResolver(commandRunner{}, DefaultBuilderTag, DefaultRuntimeTag)
}

func NewResolver(command Command, builderTag, runtimeTag string) *Resolver {
	return &Resolver{command: command, builderTag: builderTag, runtimeTag: runtimeTag}
}

func (resolver *Resolver) Resolve(ctx context.Context) (Images, error) {
	if resolver.command == nil {
		return Images{}, errors.New("Docker image resolver requires a command runner")
	}
	if err := validateTag(resolver.builderTag); err != nil {
		return Images{}, fmt.Errorf("invalid builder image tag: %w", err)
	}
	if err := validateTag(resolver.runtimeTag); err != nil {
		return Images{}, fmt.Errorf("invalid runtime image tag: %w", err)
	}
	builderID, err := resolver.resolve(ctx, resolver.builderTag)
	if err != nil {
		return Images{}, err
	}
	runtimeID, err := resolver.resolve(ctx, resolver.runtimeTag)
	if err != nil {
		return Images{}, err
	}
	runtimeReference, err := resolver.resolveRuntimeReference(ctx)
	if err != nil {
		return Images{}, err
	}
	return Images{BuilderID: builderID, RuntimeID: runtimeID, RuntimeReference: runtimeReference}, nil
}

func (resolver *Resolver) resolveRuntimeReference(ctx context.Context) (string, error) {
	output, err := resolver.command.Run(ctx, "docker", "image", "inspect", "--format={{index .RepoDigests 0}}", resolver.runtimeTag)
	if err != nil {
		return "", fmt.Errorf("inspect immutable runtime image reference %q: %w", resolver.runtimeTag, err)
	}
	reference := stripSingleLineEnding(string(output))
	if !regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`).MatchString(reference) {
		return "", fmt.Errorf("inspect %q returned an invalid immutable runtime image reference", resolver.runtimeTag)
	}
	return reference, nil
}

func (resolver *Resolver) resolve(ctx context.Context, tag string) (string, error) {
	output, err := resolver.command.Run(ctx, "docker", "image", "inspect", "--format={{.Id}}", tag)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("inspect local Docker image %q: %w", tag, ctx.Err())
		}
		detail := ""
		if len(output) <= 512 {
			if message := strings.TrimSpace(string(output)); message != "" {
				detail = ": " + message
			}
		}
		return "", fmt.Errorf(
			"%w: inspect %q: %v%s; build or load the image locally before continuing",
			ErrImageUnavailable,
			tag,
			err,
			detail,
		)
	}
	if len(output) > maxImageIDBytes {
		return "", fmt.Errorf("inspect %q returned an oversized Docker image ID", tag)
	}
	id := stripSingleLineEnding(string(output))
	if !imageIDPattern.MatchString(id) {
		return "", fmt.Errorf(
			"inspect %q returned an invalid Docker image ID; expected exactly one lowercase sha256 image ID",
			tag,
		)
	}
	return id, nil
}

func stripSingleLineEnding(value string) string {
	if strings.HasSuffix(value, "\r\n") {
		return strings.TrimSuffix(value, "\r\n")
	}
	return strings.TrimSuffix(value, "\n")
}

func validateTag(tag string) error {
	if tag == "" || strings.ContainsAny(tag, "\x00\r\n\t ") {
		return errors.New("tag must be a non-empty value without whitespace")
	}
	return nil
}

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
