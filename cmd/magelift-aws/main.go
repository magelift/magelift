package main

import (
	"fmt"
	"os"

	internalcli "github.com/magelift/magelift/internal/cli"
	awsops "github.com/magelift/magelift/internal/cloud/aws/ops"
	"github.com/magelift/magelift/internal/platform"
)

func main() {
	modules := platform.NewModuleRegistry()
	if err := modules.RegisterModule(awsops.Module{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	command := internalcli.NewWithModules(modules)
	if err := internalcli.Execute(command); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(internalcli.ExitCode(err))
	}
}
