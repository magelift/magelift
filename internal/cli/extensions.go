package cli

import (
	"github.com/magelift/magelift/sdk"
	"github.com/spf13/cobra"
)

type extensionInventory struct {
	ServiceAdapters []sdk.ServiceAdapterDescriptor `json:"serviceAdapters,omitempty" yaml:"serviceAdapters,omitempty"`
	Modules         []sdk.ExtensionDescriptor      `json:"modules" yaml:"modules"`
	Providers       []providerProvenance           `json:"providers" yaml:"providers"`
}

func extensionsCommand(o *options) *cobra.Command {
	command := &cobra.Command{
		Use:     "extensions",
		Aliases: []string{"extension"},
		Short:   "Inspect explicitly registered provider extensions",
	}
	command.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List extension provenance and targets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inventory := extensionInventory{}
			if o.modules != nil {
				inventory.Modules = o.modules.Extensions()
				inventory.ServiceAdapters = o.modules.ServiceAdapters()
			}
			inventory.Providers = o.subprocessProvenance(cmd.Context())
			return o.write(inventory)
		},
	})
	return command
}
