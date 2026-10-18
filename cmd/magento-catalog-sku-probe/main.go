// Command magento-catalog-sku-probe requires the observed Magento catalog SKU
// to match a known product. HA cells use it when HTTP storefront proof is
// unavailable; setup:db:status is not a substitute.
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
		fmt.Fprintf(os.Stderr, "magento catalog SKU probe failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("magento-catalog-sku-probe", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	observed := flags.String("observed", "", "exec output containing the catalog SKU")
	expect := flags.String("expect", "", "known Magento product SKU")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := certification.VerifyMagentoCatalogSKU(*observed, *expect); err != nil {
		return err
	}
	fmt.Printf("magento catalog SKU PASS sku=%s\n", *expect)
	return nil
}
