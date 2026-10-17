package githubci

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// NewFromEnvironment reuses the operator's GitHub login without implementing OAuth.
// GH_TOKEN takes precedence over GITHUB_TOKEN, followed by the official gh CLI.
func NewFromEnvironment(ctx context.Context) (*Client, error) {
	token, err := resolveToken(ctx, os.Getenv, readCLIToken)
	if err != nil {
		return nil, err
	}
	return New(token, nil)
}

func resolveToken(ctx context.Context, getenv func(string) string, cli func(context.Context) (string, error)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if token := strings.TrimSpace(getenv(name)); token != "" {
			return token, nil
		}
	}
	token, err := cli(ctx)
	if err != nil || strings.TrimSpace(token) == "" {
		return "", errors.New("GitHub login unavailable: run gh auth login for github.com or supply GH_TOKEN with repository metadata read and Actions variables write access")
	}
	return strings.TrimSpace(token), nil
}

func readCLIToken(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", "github.com").Output()
	if err != nil {
		return "", errors.New("GitHub CLI token lookup failed")
	}
	return string(output), nil
}
