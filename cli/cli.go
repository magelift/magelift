// Package cli exposes the supported custom-binary entry point for MageLift
// community extensions.
package cli

import (
	"fmt"

	internalcli "github.com/magelift/magelift/internal/cli"
	"github.com/magelift/magelift/internal/registry"
	"github.com/magelift/magelift/sdk"
	"github.com/spf13/cobra"
)

// New returns a command with the first-party MageLift modules registered.
func New() (*cobra.Command, error) {
	return NewWithExtensions()
}

// ExitCode preserves the CLI's stable user-error exit code contract for
// custom binaries.
func ExitCode(err error) int {
	return internalcli.ExitCode(err)
}

// Execute runs a command with graceful SIGINT/SIGTERM cancellation.
func Execute(command *cobra.Command) error {
	return internalcli.Execute(command)
}

// NewWithExtensions returns a command with first-party modules plus the
// explicitly linked community modules. It never loads extensions from disk.
func NewWithExtensions(extensions ...sdk.Module) (*cobra.Command, error) {
	return NewWithExtensionsAndHooks(registry.RegisterHooks(), extensions...)
}

// NewWithExtensionsAndHooks returns a command with first-party modules plus the
// explicitly linked community modules, with provider hooks supplied by the
// caller instead of the default first-party set.
func NewWithExtensionsAndHooks(hooks internalcli.Hooks, extensions ...sdk.Module) (*cobra.Command, error) {
	modules, err := registry.NewDefault()
	if err != nil {
		return nil, fmt.Errorf("register first-party modules: %w", err)
	}
	for _, extension := range extensions {
		if err := modules.RegisterPublicModule(extension); err != nil {
			return nil, err
		}
	}
	return internalcli.NewWithModulesAndHooks(modules, hooks), nil
}
