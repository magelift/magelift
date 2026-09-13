package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// Execute runs a production CLI with a cancellation context. The signal
// handler gives deployment defers time to clean up candidate resources and
// release distributed locks before the process exits.
func Execute(command *cobra.Command) error {
	if command == nil {
		return errors.New("CLI command is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return command.ExecuteContext(ctx)
}
