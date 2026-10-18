package automation

import (
	"context"
	"errors"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const pulumiProjectName = "magelift"

type upsertInlineStackFunc func(context.Context, string, string, pulumi.RunFunc, ...auto.LocalWorkspaceOption) (auto.Stack, error)

// NewInlineStackWithBackend creates an Automation API stack from any provider
// program. An empty backendURL keeps Pulumi's normal backend selection.
func NewInlineStackWithBackend(ctx context.Context, stackName string, program pulumi.RunFunc, backendURL string) (*auto.Stack, error) {
	return NewInlineStackWithEnvironment(ctx, stackName, program, backendURL, nil)
}

// NewInlineStackWithEnvironment is like NewInlineStackWithBackend but also
// supplies workspace-scoped environment variables for the Pulumi Automation
// API. backendURL, when non-empty, sets PULUMI_BACKEND_URL and overrides any
// value in env. env is not mutated.
func NewInlineStackWithEnvironment(
	ctx context.Context,
	stackName string,
	program pulumi.RunFunc,
	backendURL string,
	env map[string]string,
) (*auto.Stack, error) {
	return newInlineStackWithEnvironment(ctx, stackName, program, backendURL, env, auto.UpsertStackInlineSource)
}

func newInlineStackWithEnvironment(ctx context.Context, stackName string, program pulumi.RunFunc, backendURL string, env map[string]string, upsert upsertInlineStackFunc) (*auto.Stack, error) {
	if ctx == nil {
		return nil, errors.New("automation context is required")
	}
	if stackName == "" {
		return nil, errors.New("Pulumi stack name is required")
	}
	if program == nil {
		return nil, errors.New("Pulumi program is required")
	}
	options := inlineStackWorkspaceOptions(backendURL, env)
	stack, err := upsert(ctx, stackName, pulumiProjectName, program, options...)
	if err != nil {
		return nil, err
	}
	return &stack, nil
}

func inlineStackWorkspaceOptions(backendURL string, env map[string]string) []auto.LocalWorkspaceOption {
	merged := mergeInlineStackEnv(backendURL, env)
	if len(merged) == 0 {
		return nil
	}
	return []auto.LocalWorkspaceOption{auto.EnvVars(merged)}
}

func mergeInlineStackEnv(backendURL string, env map[string]string) map[string]string {
	var merged map[string]string
	if env != nil {
		merged = make(map[string]string, len(env)+1)
		for key, value := range env {
			merged[key] = value
		}
	}
	if strings.TrimSpace(backendURL) != "" {
		if merged == nil {
			merged = make(map[string]string, 1)
		}
		merged["PULUMI_BACKEND_URL"] = backendURL
	}
	return merged
}
