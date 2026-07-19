package cli

import (
	"github.com/acourtiol/magelift/internal/platform"
	"github.com/spf13/cobra"
)

func bootstrapCommand(o *options) *cobra.Command {
	var accessLogBucket string
	var githubOwner string
	var githubRepo string
	command := &cobra.Command{
		Use:   "bootstrap",
		Short: "Create or reconcile the Pulumi state backend",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, planned, err := o.planStack(false)
			if err != nil {
				return invalid(err)
			}
			boot, err := o.bootstrapPort()
			if err != nil {
				return err
			}
			if err := boot.VerifyAccount(cmd.Context(), planned); err != nil {
				if mapped := notSupported(err, planned, "bootstrap"); mapped != err {
					return mapped
				}
				return &exitError{code: 3, err: err}
			}
			result, err := boot.Ensure(cmd.Context(), planned, platform.BootstrapRequest{
				AccessLogBucket: accessLogBucket,
				GitHubOwner:     githubOwner,
				GitHubRepo:      githubRepo,
			})
			if err != nil {
				if mapped := notSupported(err, planned, "bootstrap"); mapped != err {
					return mapped
				}
				return &exitError{code: 3, err: err}
			}
			return o.write(result)
		},
	}
	command.Flags().StringVar(&accessLogBucket, "access-log-bucket", "", "existing object-storage bucket for state access logs (AWS)")
	command.Flags().StringVar(&githubOwner, "github-owner", "", "GitHub repository owner for the deployment role")
	command.Flags().StringVar(&githubRepo, "github-repo", "", "GitHub repository name for the deployment role")
	return command
}
