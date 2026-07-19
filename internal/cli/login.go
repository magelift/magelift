package cli

import (
	"errors"
	"fmt"

	awsbootstrap "github.com/acourtiol/magelift/internal/cloud/aws/bootstrap"
	"github.com/spf13/cobra"
)

func loginCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "login", Short: "Verify AWS credentials for the selected environment", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		effective, environment, err := o.resolveWithEnvironment()
		if err != nil {
			return invalid(err)
		}
		if effective.Config.Target.Provider != "aws" {
			return invalid(errors.New("login currently supports the AWS target only"))
		}
		if effective.Config.Account == "" {
			return invalid(errors.New("selected environment must define an AWS account"))
		}
		verify := o.verifyAccount
		if verify == nil {
			verify = awsbootstrap.VerifyAccount
		}
		if err := verify(cmd.Context(), effective.Config.Defaults.Region, effective.Config.Account); err != nil {
			return &exitError{code: 3, err: fmt.Errorf("verify AWS credentials: %w", err)}
		}
		return o.write(map[string]any{"authenticated": true, "environment": environment, "account": effective.Config.Account, "region": effective.Config.Defaults.Region})
	}}
}
