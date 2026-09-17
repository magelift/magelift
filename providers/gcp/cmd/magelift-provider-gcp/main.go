// Command magelift-provider-gcp is the autonomous GCP provider plugin. It
// serves the versioned typed operations over go-plugin net/rpc.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"

	gcpauth "github.com/magelift/magelift/providers/gcp/auth"
	gcpplugin "github.com/magelift/magelift/providers/gcp/plugin"
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
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: gcpplugin.HandshakeConfig,
		Plugins:         gcpplugin.PluginMap(newProductionServer()),
		Logger:          hclog.New(&hclog.LoggerOptions{Level: hclog.Warn, Output: os.Stderr}),
	})
}

// newProductionServer wires the released plugin: version stamp plus fresh
// ambient credentials for every Kubernetes client the server builds.
// Exec/tunnel launch tokens resolve through ambient ADC automatically.
func newProductionServer() *gcpplugin.Server {
	return &gcpplugin.Server{Version: Version, KubeClients: gcpauth.NewClientFactory()}
}
