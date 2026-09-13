package main

import (
	"fmt"
	"os"

	"github.com/magelift/magelift/cli"
)

func main() {
	command, err := cli.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := cli.Execute(command); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}
