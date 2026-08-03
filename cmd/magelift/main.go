package main

import (
	"fmt"
	"os"

	"github.com/acourtiol/magelift/internal/cli"
	awseksops "github.com/acourtiol/magelift/internal/cloud/aws/eksops"
	awsops "github.com/acourtiol/magelift/internal/cloud/aws/ops"
	gcpops "github.com/acourtiol/magelift/internal/cloud/gcp/ops"
	ovhstack "github.com/acourtiol/magelift/internal/cloud/ovh/stack"
	scwstack "github.com/acourtiol/magelift/internal/cloud/scaleway/stack"
	"github.com/acourtiol/magelift/internal/platform"
)

func main() {
	modules := platform.NewModuleRegistry()
	for _, module := range []platform.StackModule{
		awsops.Module{},
		awseksops.Module{},
		gcpops.Module{},
		ovhstack.Module{},
		scwstack.Module{},
	} {
		if err := modules.RegisterModule(module); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := cli.NewWithModules(modules).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}
