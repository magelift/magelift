// Command failed-deployment-probe validates the current and fault immutable
// image digests used by the failed-deployment drill. Mutation stays in the
// acceptance harness so a bad artifact is never applied without restore.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/magelift/magelift/internal/certification"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "failed-deployment probe failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("failed-deployment-probe", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	current := flags.String("current", "", "immutable repository@sha256 digest currently running")
	fault := flags.String("fault", "", "immutable repository@sha256 digest that must fail runtime health")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := certification.ValidateFailedDeploymentRequest(certification.FailedDeploymentRequest{
		CurrentDigest: *current, FaultDigest: *fault,
	}); err != nil {
		return err
	}
	fmt.Printf("failed-deployment digests ok current=%s\n", *current)
	return nil
}
