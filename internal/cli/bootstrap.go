package cli

import (
	"context"
	"errors"
	"fmt"

	awsbootstrap "github.com/acourtiol/magelift/internal/cloud/aws/bootstrap"
	"github.com/acourtiol/magelift/internal/config"
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
			if accessLogBucket == "" {
				return invalid(errors.New("--access-log-bucket is required"))
			}
			if githubOwner == "" || githubRepo == "" {
				return invalid(errors.New("--github-owner and --github-repo are required"))
			}
			file, err := o.load()
			if err != nil {
				return invalid(err)
			}
			environment, err := o.selectEnvironment(file)
			if err != nil {
				return invalid(err)
			}
			effective, err := file.Resolve(environment, config.ResolveOptions{})
			if err != nil {
				return invalid(err)
			}
			if effective.Config.Target.Provider != "aws" {
				return invalid(errors.New("bootstrap currently supports the AWS target only"))
			}
			if effective.Config.Account == "" {
				return invalid(errors.New("selected environment must define an AWS account"))
			}
			verifyAccount := o.verifyAccount
			if verifyAccount == nil {
				verifyAccount = awsbootstrap.VerifyAccount
			}
			if err := verifyAccount(cmd.Context(), effective.Config.Defaults.Region, effective.Config.Account); err != nil {
				return &exitError{code: 3, err: err}
			}
			plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
				Project:         effective.Config.Project.Name,
				Environment:     environment,
				AccountID:       effective.Config.Account,
				Region:          effective.Config.Defaults.Region,
				AccessLogBucket: accessLogBucket,
			})
			if err != nil {
				return invalid(err)
			}
			newBootstrap := o.newBootstrap
			if newBootstrap == nil {
				newBootstrap = func(ctx context.Context, region string) (awsbootstrap.Ensurer, error) {
					return awsbootstrap.NewAWS(ctx, region)
				}
			}
			backend, err := newBootstrap(cmd.Context(), effective.Config.Defaults.Region)
			if err != nil {
				return &exitError{code: 3, err: fmt.Errorf("initialize AWS bootstrap clients: %w", err)}
			}
			result, err := backend.Ensure(cmd.Context(), plan, effective.Config.Defaults.Region)
			if err != nil {
				return &exitError{code: 3, err: err}
			}
			newIdentity := o.newIdentity
			if newIdentity == nil {
				newIdentity = func(ctx context.Context, region string) (awsbootstrap.IdentityEnsurer, error) {
					return awsbootstrap.NewAWSIdentity(ctx, region)
				}
			}
			identityPlan, err := awsbootstrap.BuildIdentityPlan(awsbootstrap.IdentitySpec{
				Project: effective.Config.Project.Name, Environment: environment, AccountID: effective.Config.Account,
				Region: effective.Config.Defaults.Region, GitHubOwner: githubOwner, GitHubRepo: githubRepo,
				StateBucket: result.Plan.StateBucket, KMSKeyARN: result.KeyARN,
			})
			if err != nil {
				return invalid(err)
			}
			identity, err := newIdentity(cmd.Context(), effective.Config.Defaults.Region)
			if err != nil {
				return &exitError{code: 3, err: fmt.Errorf("initialize AWS identity clients: %w", err)}
			}
			if err := identity.Ensure(cmd.Context(), identityPlan); err != nil {
				return &exitError{code: 3, err: err}
			}
			return o.write(struct {
				State    awsbootstrap.Result
				Identity awsbootstrap.IdentityPlan
			}{State: result, Identity: identityPlan})
		},
	}
	command.Flags().StringVar(&accessLogBucket, "access-log-bucket", "", "existing S3 bucket for state access logs")
	command.Flags().StringVar(&githubOwner, "github-owner", "", "GitHub repository owner for the deployment role")
	command.Flags().StringVar(&githubRepo, "github-repo", "", "GitHub repository name for the deployment role")
	return command
}
