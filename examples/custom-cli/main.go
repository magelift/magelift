// Command magelift-ext shows how a community or experimental provider registers
// into a custom CLI binary (ADR 0007). It is not part of the default magelift
// release artifact.
//
// Build:
//
//	go build -o magelift-ext ./examples/custom-cli
//
// Replace stubModule with an external (or in-module) StackModule that
// implements a real Plan/Program when shipping a community provider.
package main

import (
	"fmt"
	"os"

	"github.com/acourtiol/magelift/internal/cli"
	awseksops "github.com/acourtiol/magelift/internal/cloud/aws/eksops"
	awsops "github.com/acourtiol/magelift/internal/cloud/aws/ops"
	gcpops "github.com/acourtiol/magelift/internal/cloud/gcp/ops"
	"github.com/acourtiol/magelift/internal/platform"
)

func main() {
	modules := platform.NewModuleRegistry()
	for _, module := range []platform.StackModule{
		awsops.Module{},
		awseksops.Module{},
		gcpops.Module{},
		stubModule{}, // community registration demo (Plan refuses deploy)
	} {
		if err := modules.RegisterModule(module); err != nil {
			fail(err)
		}
	}
	if err := cli.NewWithModules(modules).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
