package main

import (
	"fmt"
	"os"

	internalcli "github.com/magelift/magelift/internal/cli"
	scwstack "github.com/magelift/magelift/internal/cloud/scaleway/stack"
	"github.com/magelift/magelift/internal/platform"
)

func main() {
	modules := platform.NewModuleRegistry()
	if err := modules.RegisterModule(scwstack.Module{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	command := internalcli.NewWithModules(modules)
	if err := internalcli.Execute(command); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(internalcli.ExitCode(err))
	}
}
