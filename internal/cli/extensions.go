package cli

import (
	"github.com/magelift/magelift/internal/webruntime"
	"github.com/magelift/magelift/sdk"
	"github.com/spf13/cobra"
)

type extensionInventory struct {
	Modules     []sdk.ExtensionDescriptor `json:"modules" yaml:"modules"`
	WebRuntimes []webruntime.ListedPlugin `json:"webRuntimes" yaml:"webRuntimes"`
	Providers   []providerProvenance      `json:"providers" yaml:"providers"`
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
			inventory := extensionInventory{WebRuntimes: webruntime.List()}
			if o.modules != nil {
				inventory.Modules = o.modules.Extensions()
			}
			inventory.Providers = o.subprocessProvenance(cmd.Context())
			return o.write(inventory)
		},
	})
	return command
}
