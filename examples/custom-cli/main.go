// Command magelift-ext shows how a community or experimental provider registers
// into a custom CLI binary. It is not part of the default magelift release
// artifact.
//
// Build:
//
//	go build -o magelift-ext ./examples/custom-cli
//
// Replace stubModule with an external module that implements a real Plan/Program
// when shipping a community provider.
package main

import (
	"fmt"
	"os"

	"github.com/magelift/magelift/cli"
)

func main() {
	command, err := cli.NewWithExtensions(stubModule{})
	if err != nil {
		fail(err)
	}
	if err := command.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
