// Command magelift-provider-azure serves the Azure provider plugin.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	azureplugin "github.com/magelift/magelift/providers/azure/plugin"
)

var Version = "v0.0.0-dev"

func main() {
	showVersion := flag.Bool("version", false, "print the provider version")
	flag.Parse()
	if *showVersion {
		fmt.Println(Version)
		return
	}
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: azureplugin.HandshakeConfig,
		Plugins:         azureplugin.PluginMap(Version),
		Logger:          hclog.New(&hclog.LoggerOptions{Level: hclog.Warn, Output: os.Stderr}),
	})
}
