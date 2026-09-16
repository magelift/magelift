package main

import (
	"fmt"
	"os"

	"github.com/magelift/magelift/cli"
	"github.com/magelift/magelift/internal/registry"
)

func main() {
	command, err := cli.NewWithExtensionsAndHooks(registry.RegisterHooks())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := cli.Execute(command); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}
