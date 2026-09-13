package main

import (
	"fmt"
	"os"

	internalcli "github.com/magelift/magelift/internal/cli"
	gcpops "github.com/magelift/magelift/internal/cloud/gcp/ops"
	gcptarget "github.com/magelift/magelift/internal/cloud/gcp/target"
	"github.com/magelift/magelift/internal/platform"
)

func main() {
	modules := platform.NewModuleRegistry()
	if err := modules.RegisterModule(gcpops.Module{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := modules.RegisterModule(gcpops.Module{RuntimeID: gcptarget.RuntimeStandardID}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	command := internalcli.NewWithModules(modules)
	if err := internalcli.Execute(command); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(internalcli.ExitCode(err))
	}
}
