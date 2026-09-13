// Command magento-seed-probe requires the observed magelift_seed_probe label
// to match the known migrate fixture. The default GCP dump is tiny.sql
// (label tiny-fixture); it has no catalog products.
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
		fmt.Fprintf(os.Stderr, "magento seed probe failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("magento-seed-probe", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	observed := flags.String("observed", "", "exec output containing the seed probe label")
	expect := flags.String("expect", "", "known magelift_seed_probe label")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := certification.VerifyMagentoSeedProbe(*observed, *expect); err != nil {
		return err
	}
	fmt.Printf("magento seed probe PASS label=%s\n", *expect)
	return nil
}
