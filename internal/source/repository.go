package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var revisionPattern = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)

type Repository struct {
	Root      string
	Revision  string
	OriginURL string
}

type Inspector interface {
	Inspect(context.Context, string) (Repository, error)
}

type GitInspector struct{}

func (GitInspector) Inspect(ctx context.Context, directory string) (Repository, error) {
	root, err := gitOutput(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{}, fmt.Errorf("locate Git repository: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Repository{}, fmt.Errorf("resolve Git repository root: %w", err)
	}

	revision, err := gitOutput(ctx, root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return Repository{}, fmt.Errorf("resolve source revision: %w", err)
	}
	if !revisionPattern.MatchString(revision) {
		return Repository{}, fmt.Errorf("Git returned invalid source revision %q", revision)
	}

	status, err := gitBytes(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return Repository{}, fmt.Errorf("inspect repository status: %w", err)
	}
	if len(status) != 0 {
		return Repository{}, errors.New("repository contains tracked or untracked changes; commit or remove them before building")
	}

	originURL, err := gitOutput(ctx, root, "config", "--get", "remote.origin.url")
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return Repository{}, fmt.Errorf("inspect Git origin: %w", err)
		}
	}

	return Repository{Root: filepath.Clean(root), Revision: revision, OriginURL: originURL}, nil
}

func gitOutput(ctx context.Context, directory string, args ...string) (string, error) {
	output, err := gitBytes(ctx, directory, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func gitBytes(ctx context.Context, directory string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return nil, err
		}
		return nil, errors.New(message)
	}
	return output, nil
}
