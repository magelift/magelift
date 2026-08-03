package cli

import (
	"github.com/acourtiol/magelift/internal/config"
	"github.com/spf13/cobra"
)

func statusCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the selected environment configuration",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
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
			return o.write(map[string]any{
				"environment": environment,
				"project":     effective.Config.Project.Name,
				"provider":    effective.Config.Target.Provider,
				"runtime":     effective.Config.Target.Runtime,
				"class":       effective.Config.Class,
				"preset":      effective.Config.Defaults.Preset,
				"domain":      effective.Config.Domain,
				"protected":   effective.Config.Protection,
			})
		},
	}
}
