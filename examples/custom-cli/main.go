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

	awsops "github.com/acourtiol/magelift/internal/cloud/aws/ops"
	gcpstack "github.com/acourtiol/magelift/internal/cloud/gcp/stack"
	"github.com/acourtiol/magelift/internal/platform"
)

func main() {
	registry := platform.NewModuleRegistry()
	if err := registry.RegisterModule(awsops.Module{}); err != nil {
		fail(err)
	}
	if err := registry.RegisterModule(gcpstack.Module{}); err != nil {
		fail(err)
	}
	// Community providers register the same way:
	//   registry.RegisterModule(community.Module{})
	fmt.Fprintln(os.Stdout, "registered stack modules: aws/ecs-fargate, gcp/gke-autopilot")
	fmt.Fprintln(os.Stdout, "wire this registry into your CLI main the same way cmd/magelift does")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
