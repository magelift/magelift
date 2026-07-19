package main

import (
	"fmt"
	"os"

	"github.com/acourtiol/magelift/internal/cli"
)

func main() {
	if err := cli.New().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}
