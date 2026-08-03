package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/acourtiol/magelift/internal/benchmark"
	"github.com/spf13/cobra"
)

func benchmarkCommand(o *options) *cobra.Command {
	var targetURL, mix, edition, magentoVersion, phpVersion, catalogName, catalogRevision string
	var database, search, queue, cache string
	var duration time.Duration
	var concurrency int
	command := &cobra.Command{Use: "benchmark", Short: "Measure a Magento workload for preset sizing"}
	run := &cobra.Command{
		Use:  "run",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			requests, err := parseBenchmarkMix(mix)
			if err != nil {
				return invalid(err)
			}
			report, err := benchmark.Run(cmd.Context(), benchmark.Config{
				URL: targetURL, Duration: duration, Concurrency: concurrency, Requests: requests,
				Catalog: benchmark.Catalog{
					Name: catalogName, Revision: catalogRevision, Edition: edition,
					MagentoVersion: magentoVersion, PHPVersion: phpVersion,
					Database: database, Search: search, Queue: queue, Cache: cache,
				},
			})
			if err != nil {
				return invalid(err)
			}
			return o.write(report)
		},
	}
	run.Flags().StringVar(&targetURL, "url", "", "Magento base URL to measure")
	run.Flags().DurationVar(&duration, "duration", 30*time.Second, "measurement duration")
	run.Flags().IntVar(&concurrency, "concurrency", 10, "number of concurrent workers")
	run.Flags().StringVar(&mix, "mix", "/=100", "request mix as comma-separated path=weight entries")
	run.Flags().StringVar(&catalogName, "catalog", "", "benchmark catalog name")
	run.Flags().StringVar(&catalogRevision, "catalog-revision", "", "benchmark catalog revision or commit")
	run.Flags().StringVar(&edition, "edition", "open-source", "Magento edition: open-source or commerce")
	run.Flags().StringVar(&magentoVersion, "magento-version", "", "Magento version under test")
	run.Flags().StringVar(&phpVersion, "php-version", "", "PHP version under test")
	run.Flags().StringVar(&database, "database", "", "database engine and version under test")
	run.Flags().StringVar(&search, "search", "", "search engine and version under test")
	run.Flags().StringVar(&queue, "queue", "", "queue implementation and version under test")
	run.Flags().StringVar(&cache, "cache", "", "cache implementation and version under test")
	command.AddCommand(run)
	return command
}

func parseBenchmarkMix(value string) ([]benchmark.Request, error) {
	var requests []benchmark.Request
	for _, item := range strings.Split(value, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("benchmark mix entry %q must be path=weight", item)
		}
		weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("benchmark mix entry %q has invalid weight", item)
		}
		requests = append(requests, benchmark.Request{Path: strings.TrimSpace(parts[0]), Weight: weight})
	}
	return requests, nil
}
