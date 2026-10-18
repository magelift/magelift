package cli

import (
	"fmt"

	"github.com/magelift/magelift/internal/config"
	"github.com/spf13/cobra"
)

func compatibilityCommand(o *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "compatibility",
		Short: "Inspect the Adobe and MageLift compatibility catalog",
	}

	catalog := &cobra.Command{
		Use:   "catalog [release]",
		Short: "List release and service requirements",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return o.write(config.CurrentCompatibilityCatalog())
			}
			requirements := config.RequirementsForRelease(args[0])
			if len(requirements) == 0 {
				return invalid(fmt.Errorf("release %q is not in the compatibility catalog", args[0]))
			}
			return o.write(requirements)
		},
	}

	var (
		release   string
		component string
		option    string
		version   string
	)
	validate := &cobra.Command{
		Use:   "validate",
		Short: "Validate one catalog service choice",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if release == "" || component == "" || option == "" {
				return invalid(fmt.Errorf("--release, --component, and --option are required"))
			}
			selection := config.CompatibilitySelection{
				Release: release, Component: config.CompatibilityComponent(component), Option: option, Version: version,
			}
			requirement, err := config.ValidateCompatibilitySelection(selection)
			if err != nil {
				return invalid(err)
			}
			return o.write(requirement)
		},
	}
	validate.Flags().StringVar(&release, "release", "", "exact Adobe Commerce release, such as 2.4.8-p5")
	validate.Flags().StringVar(&component, "component", "", "catalog component, such as database or search")
	validate.Flags().StringVar(&option, "option", "", "service option, such as mysql or opensearch")
	validate.Flags().StringVar(&version, "version", "", "service version when the catalog lists versions")

	command.AddCommand(catalog, validate)
	return command
}
