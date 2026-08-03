package cli

import (
	"errors"
	"fmt"

	"github.com/acourtiol/magelift/internal/platform"
	"github.com/acourtiol/magelift/internal/usererr"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/spf13/cobra"
)

func costCommand(o *options) *cobra.Command {
	var live bool
	command := &cobra.Command{
		Use:   "cost",
		Short: "Describe the selected environment's cost inputs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, planned, err := o.planStack(false)
			if err != nil {
				return invalid(err)
			}
			estimator, err := o.costEstimator()
			if err != nil {
				return err
			}
			effective, _, err := o.resolveWithEnvironment()
			if err != nil {
				return invalid(err)
			}
			report, err := estimator.Estimate(cmd.Context(), planned, effective.Config, platform.CostOptions{Live: live})
			if err != nil {
				if mapped := notSupported(err, planned, "cost estimation"); mapped != err {
					return mapped
				}
				return &exitError{code: 3, err: usererr.Wrap(err, "cost estimation failed", "Check catalog fields and provider credentials, then retry.", "")}
			}
			return o.write(report)
		},
	}
	command.Flags().BoolVar(&live, "live", false, "query current provider on-demand prices when the adapter supports it")
	return command
}

func (o *options) costEstimator() (platform.CostEstimator, error) {
	if o.testCostEstimator != nil {
		return o.testCostEstimator, nil
	}
	if o.modules == nil {
		return nil, errors.New("stack module registry is required")
	}
	effective, _, err := o.resolveWithEnvironment()
	if err != nil {
		return nil, invalid(err)
	}
	module, found := o.modules.Module(sdk.ProviderID(effective.Config.Target.Provider), sdk.RuntimeID(effective.Config.Target.Runtime))
	if !found {
		return nil, invalid(fmt.Errorf("no stack module for %s/%s", effective.Config.Target.Provider, effective.Config.Target.Runtime))
	}
	estimator := platform.ModuleCostEstimator(module)
	if estimator == nil {
		return nil, invalid(fmt.Errorf("%s", notSupportedForModule(module, "cost estimation")))
	}
	return estimator, nil
}
