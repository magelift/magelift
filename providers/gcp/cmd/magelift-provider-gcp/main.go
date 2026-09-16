// Command magelift-provider-gcp is the autonomous GCP provider plugin. It
// serves the versioned typed operations over go-plugin net/rpc. The full
// server wires up as the provider implementation lands; this skeleton
// establishes the module, the binary name, and version reporting.
package main

import (
	"flag"
	"fmt"
	"os"
)

// Version is set by the release build. The development default identifies
// unreleased local builds.
var Version = "v0.0.0-dev"

func main() {
	showVersion := flag.Bool("version", false, "print the provider version")
	flag.Parse()
	if *showVersion {
		fmt.Println(Version)
		return
	}
	fmt.Fprintln(os.Stderr, "magelift-provider-gcp: plugin server not yet wired")
	os.Exit(2)
}
