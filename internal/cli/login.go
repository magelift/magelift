package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func loginCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "login", Short: "Verify cloud credentials for the selected environment", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		_, planned, err := o.planStack(false)
		if err != nil {
			return invalid(err)
		}
		boot, err := o.bootstrapPort()
		if err != nil {
			return err
		}
		if err := boot.VerifyAccount(cmd.Context(), planned); err != nil {
			if mapped := notSupported(err, planned, "login"); mapped != err {
				return mapped
			}
			return &exitError{code: 3, err: fmt.Errorf("verify credentials: %w", err)}
		}
		return o.write(map[string]any{
			"authenticated": true,
			"environment":   planned.Environment(),
			"provider":      planned.Provider(),
			"region":        planned.Region(),
		})
	}}
}
