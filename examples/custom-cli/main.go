// Command magelift-ext shows how a community or experimental provider registers
// into a custom CLI binary (ADR 0007). It is not part of the default magelift
// release artifact.
//
// Build:
//
//	go build -o magelift-ext ./examples/custom-cli
//
// Replace the stub module with an external Go module that implements
// platform.StackModule (and optionally platform.HasOps).
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
		// Community providers register the same way:
		// community.Module{},
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
