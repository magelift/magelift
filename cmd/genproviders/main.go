// Command genproviders writes per-platform magelift.providers.lock files
// from a GoReleaser dist directory. The release workflow runs it after
// signing; every emitted entry carries the schema and protocol marker the
// CLI loader requires.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/magelift/magelift/internal/providerhost"
)

func main() {
	tag := flag.String("tag", "", "release tag stamped into the lockfiles (required)")
	dist := flag.String("dist", "dist", "GoReleaser dist directory")
	flag.Parse()
	if err := run(*tag, *dist); err != nil {
		fmt.Fprintf(os.Stderr, "genproviders: %v\n", err)
		os.Exit(1)
	}
}

func run(tag, dist string) error {
	locks, err := providerhost.GenerateLockfiles(dist, tag)
	if err != nil {
		return err
	}
	platforms := make([]string, 0, len(locks))
	for platform := range locks {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	for _, platform := range platforms {
		encoded, err := json.MarshalIndent(locks[platform], "", "  ")
		if err != nil {
			return fmt.Errorf("encode %s lockfile: %w", platform, err)
		}
		path := filepath.Join(dist, "magelift.providers.lock."+platform)
		if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Printf("wrote %s\n", path)
	}
	return nil
}
