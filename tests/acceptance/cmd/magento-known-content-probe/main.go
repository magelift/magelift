// Command magento-known-content-probe GETs a Magento storefront URL and
// requires a known marker in the HTML. HA cells use it instead of treating
// setup:db:status as application-integrity proof.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/magelift/magelift/internal/certification"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "magento known-content probe failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("magento-known-content-probe", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	url := flags.String("url", "", "absolute Magento storefront URL")
	expect := flags.String("expect", "", "single-line marker that must appear in the 200 response body")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := certification.VerifyMagentoKnownContent(ctx, nil, certification.MagentoKnownContentRequest{URL: *url, Expect: *expect}); err != nil {
		return err
	}
	fmt.Printf("magento known-content PASS url=%s\n", *url)
	return nil
}
