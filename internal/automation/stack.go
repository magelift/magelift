package automation

import (
	"context"
	"errors"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// NewInlineStackWithBackend creates an Automation API stack from any provider
// program. An empty backendURL keeps Pulumi's normal backend selection.
func NewInlineStackWithBackend(ctx context.Context, stackName string, program pulumi.RunFunc, backendURL string) (*auto.Stack, error) {
	if ctx == nil {
		return nil, errors.New("automation context is required")
	}
	if stackName == "" {
		return nil, errors.New("Pulumi stack name is required")
	}
	if program == nil {
		return nil, errors.New("Pulumi program is required")
	}
	var options []auto.LocalWorkspaceOption
	if backendURL != "" {
		options = append(options, auto.EnvVars(map[string]string{"PULUMI_BACKEND_URL": backendURL}))
	}
	stack, err := auto.UpsertStackInlineSource(ctx, stackName, "magelift", program, options...)
	if err != nil {
		return nil, err
	}
	return &stack, nil
}
