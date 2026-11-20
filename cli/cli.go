// Package cli exposes the supported custom-binary entry point for MageLift
// community extensions.
package cli

import (
	"fmt"
	"os"
	"strings"

	internalcli "github.com/magelift/magelift/internal/cli"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/registry"
	"github.com/magelift/magelift/sdk"
	"github.com/spf13/cobra"
)

// ExperimentalProvidersEnv enables cloud runtimes that require opt-in, including EKS.
const ExperimentalProvidersEnv = "MAGELIFT_EXPERIMENTAL_PROVIDERS"

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
	return newWithDefaultComponents(nil, extensions)
}

// NewWithExtensionsAndHooks returns a command with first-party modules plus the
// explicitly linked community modules, with provider hooks supplied by the
// caller instead of the default first-party set.
func NewWithExtensionsAndHooks(hooks internalcli.Hooks, extensions ...sdk.Module) (*cobra.Command, error) {
	return newWithComponents(hooks, nil, extensions)
}

// NewWithServiceAdapters builds a CLI with explicitly linked per-service
// connection adapters and optional additional target modules.
func NewWithServiceAdapters(adapters []sdk.ServiceAdapter, extensions ...sdk.Module) (*cobra.Command, error) {
	return newWithDefaultComponents(adapters, extensions)
}

func newWithComponents(hooks internalcli.Hooks, adapters []sdk.ServiceAdapter, extensions []sdk.Module) (*cobra.Command, error) {
	newDefault := registry.NewDefault
	if strings.TrimSpace(os.Getenv(ExperimentalProvidersEnv)) != "" {
		newDefault = registry.NewDefaultWithExperimental
	}
	modules, err := newDefault()
	if err != nil {
		return nil, fmt.Errorf("register first-party modules: %w", err)
	}
	return finishComponents(modules, hooks, adapters, extensions)
}

func newWithDefaultComponents(adapters []sdk.ServiceAdapter, extensions []sdk.Module) (*cobra.Command, error) {
	experimental := strings.TrimSpace(os.Getenv(ExperimentalProvidersEnv)) != ""
	var modules *platform.ModuleRegistry
	var hooks internalcli.Hooks
	var err error
	if experimental {
		modules, hooks, err = registry.NewDefaultExperimentalSet()
	} else {
		modules, hooks, err = registry.NewDefaultSet()
	}
	if err != nil {
		return nil, fmt.Errorf("register first-party modules: %w", err)
	}
	return finishComponents(modules, hooks, adapters, extensions)
}

func finishComponents(modules *platform.ModuleRegistry, hooks internalcli.Hooks, adapters []sdk.ServiceAdapter, extensions []sdk.Module) (*cobra.Command, error) {
	for _, extension := range extensions {
		if err := modules.RegisterPublicModule(extension); err != nil {
			return nil, err
		}
	}
	for _, adapter := range adapters {
		if err := modules.RegisterServiceAdapter(adapter); err != nil {
			return nil, err
		}
	}
	return internalcli.NewWithModulesAndHooks(modules, hooks), nil
}
