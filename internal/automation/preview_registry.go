package automation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

// ListPreviewRecords reads ownership records from the configured Pulumi
// backend without selecting a provider program or touching cloud resources.
func ListPreviewRecords(ctx context.Context, backendURL, identityProject string) ([]PreviewRecord, error) {
	if ctx == nil {
		return nil, errors.New("automation context is required")
	}
	if strings.TrimSpace(identityProject) == "" {
		return nil, errors.New("Pulumi project is required")
	}

	options := []auto.LocalWorkspaceOption{
		auto.Project(workspace.Project{Name: tokens.PackageName(pulumiProjectName)}),
	}
	if strings.TrimSpace(backendURL) != "" {
		options = append(options, auto.EnvVars(map[string]string{"PULUMI_BACKEND_URL": backendURL}))
	}
	workspace, err := auto.NewLocalWorkspace(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create Pulumi workspace for preview sweep: %w", err)
	}
	stacks, err := workspace.ListStacks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Pulumi preview stacks: %w", err)
	}

	records := make([]PreviewRecord, 0, len(stacks))
	for _, stack := range stacks {
		tags, err := workspace.ListTags(ctx, stack.Name)
		if err != nil {
			return nil, fmt.Errorf("read tags for Pulumi stack %q: %w", stack.Name, err)
		}
		value, ok := tags[previewOwnershipTag]
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		metadata, err := decodePreviewMetadata(value)
		if err != nil {
			return nil, fmt.Errorf("read preview record for Pulumi stack %q: %w", stack.Name, err)
		}
		if metadata.Project != identityProject {
			continue
		}
		records = append(records, PreviewRecord{StackName: stack.Name, Metadata: metadata})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].StackName < records[j].StackName
	})
	return records, nil
}
