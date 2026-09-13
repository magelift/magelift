package cli

import (
	"github.com/magelift/magelift/internal/webruntime"
	v1 "github.com/magelift/magelift/sdk/v1"
	"github.com/spf13/cobra"
)

type extensionInventory struct {
	Modules     []v1.ExtensionDescriptor  `json:"modules" yaml:"modules"`
	WebRuntimes []webruntime.ListedPlugin `json:"webRuntimes" yaml:"webRuntimes"`
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
		RunE: func(_ *cobra.Command, _ []string) error {
			inventory := extensionInventory{WebRuntimes: webruntime.List()}
			if o.modules != nil {
				inventory.Modules = o.modules.Extensions()
			}
			return o.write(inventory)
		},
	})
	return command
}
