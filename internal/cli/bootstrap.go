package cli

import (
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/usererr"
	"github.com/spf13/cobra"
)

func bootstrapCommand(o *options) *cobra.Command {
	var accessLogBucket string
	var githubOwner string
	var githubRepo string
	command := &cobra.Command{
		Use:   "bootstrap",
		Short: "Prepare this cloud account for Magento",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, planned, err := o.planStack(false)
			if err != nil {
				return invalid(err)
			}
			planned, err = o.admitPlanned(cmd.Context(), planned)
			if err != nil {
				return invalid(fmt.Errorf("provider plan admission: %w", err))
			}
			boot, err := o.bootstrapPort()
			if err != nil {
				return err
			}
			if err := boot.VerifyAccount(cmd.Context(), planned); err != nil {
				return bootstrapFailure(planned, err)
			}
			result, err := boot.Ensure(cmd.Context(), planned, platform.BootstrapRequest{
				AccessLogBucket: accessLogBucket,
				GitHubOwner:     githubOwner,
				GitHubRepo:      githubRepo,
			})
			if err != nil {
				return bootstrapFailure(planned, err)
			}
			return o.write(bootstrapOutput{
				BackendURL: result.BackendURL,
				KeyRef:     result.KeyRef,
				Details:    result.Details,
				Next:       "magelift deploy --env " + planned.Environment() + " --yes",
			})
		},
	}
	command.Flags().StringVar(&accessLogBucket, "access-log-bucket", "", "existing AWS log bucket (AWS only)")
	command.Flags().StringVar(&githubOwner, "github-owner", "", "optional GitHub owner for Actions deploys")
	command.Flags().StringVar(&githubRepo, "github-repo", "", "optional GitHub repository for Actions deploys")
	return command
}

type bootstrapOutput struct {
	BackendURL string         `json:"backendURL" yaml:"backendURL"`
	KeyRef     string         `json:"keyRef,omitempty" yaml:"keyRef,omitempty"`
	Details    map[string]any `json:"details,omitempty" yaml:"details,omitempty"`
	Next       string         `json:"next,omitempty" yaml:"next,omitempty"`
}

func bootstrapFailure(planned platform.PlannedStack, err error) error {
	if mapped := notSupported(err, planned, "bootstrap"); mapped != err {
		return mapped
	}
	if _, ok := usererr.As(err); !ok && strings.Contains(err.Error(), "access-log-bucket") {
		env := "preview"
		if planned != nil && planned.Environment() != "" {
			env = planned.Environment()
		}
		err = usererr.Wrap(err, "AWS bootstrap needs an existing log bucket", "magelift bootstrap --env "+env+" --access-log-bucket <existing-bucket>", "docs/getting-started.md")
	}
	return &exitError{code: 3, err: err}
}
