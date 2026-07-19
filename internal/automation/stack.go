package automation

import (
	"context"
	"errors"

	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// NewAWSStack creates the same inline program used by the AWS target and mock
// graph tests. The caller still owns backend selection and Pulumi config.
func NewAWSStack(ctx context.Context, stackName string, spec awsstack.Spec) (*auto.Stack, error) {
	return NewAWSStackWithBackend(ctx, stackName, spec, "")
}

// NewAWSStackWithBackend creates an inline stack and scopes the selected
// Pulumi backend to its workspace. An empty backend keeps Pulumi's normal
// selection, which is useful for users already logged in with the Pulumi CLI.
func NewAWSStackWithBackend(ctx context.Context, stackName string, spec awsstack.Spec, backendURL string) (*auto.Stack, error) {
	if ctx == nil {
		return nil, errors.New("automation context is required")
	}
	if stackName == "" {
		return nil, errors.New("Pulumi stack name is required")
	}
	var options []auto.LocalWorkspaceOption
	if backendURL != "" {
		options = append(options, auto.EnvVars(map[string]string{"PULUMI_BACKEND_URL": backendURL}))
	}
	stack, err := auto.UpsertStackInlineSource(ctx, stackName, "magelift", awsstack.Program(spec), options...)
	if err != nil {
		return nil, err
	}
	return &stack, nil
}
