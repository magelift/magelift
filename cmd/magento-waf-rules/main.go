package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/magelift/magelift/internal/edge/waf"
)

func main() {
	document := flag.String("document", "rules", "document to emit: rules, association, or armor")
	metricPrefix := flag.String("metric-prefix", "magelift-waf", "CloudWatch metric prefix for the rate-limit rule")
	description := flag.String("description", "Magento-safe Cloud Armor WAF", "Cloud Armor policy description")
	flag.Parse()
	var (
		payload []byte
		err     error
	)
	switch *document {
	case "rules":
		payload, err = waf.MarshalAWSRulesJSON(*metricPrefix)
	case "association":
		payload, err = waf.MarshalAWSAssociationJSON()
	case "armor":
		payload, err = waf.MarshalArmorPolicyJSON(*description)
	default:
		err = fmt.Errorf("unsupported document %q", *document)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "magento-waf-rules: %v\n", err)
		os.Exit(2)
	}
	if _, err := os.Stdout.Write(payload); err != nil {
		fmt.Fprintf(os.Stderr, "magento-waf-rules: write output: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write([]byte("\n")); err != nil {
		fmt.Fprintf(os.Stderr, "magento-waf-rules: write newline: %v\n", err)
		os.Exit(1)
	}
}
