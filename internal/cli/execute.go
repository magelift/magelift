package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
)

// commandProviderClosers reaps provider processes for the root command
// Execute was given. The command tree does not expose the module registry,
// so construction records the closer and Execute runs it on every return.
var commandProviderClosers sync.Map

func trackProviderCloser(root *cobra.Command, close func()) {
	if root == nil || close == nil {
		return
	}
	commandProviderClosers.Store(root, close)
}

func closeTrackedProviders(root *cobra.Command) {
	if root == nil {
		return
	}
	close, ok := commandProviderClosers.LoadAndDelete(root)
	if !ok {
		return
	}
	close.(func())()
}

// Execute runs a production CLI with a cancellation context. The signal
// handler gives deployment defers time to clean up candidate resources and
// release distributed locks before the process exits.
func Execute(command *cobra.Command) error {
	if command == nil {
		return errors.New("CLI command is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return executeContext(ctx, command)
}

// executeContext runs the command and reaps provider processes afterwards,
// including when the command returns an error or ctx is already cancelled.
func executeContext(ctx context.Context, command *cobra.Command) error {
	defer closeTrackedProviders(command)
	return command.ExecuteContext(ctx)
}
