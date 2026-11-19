// Command magelift-provider-aws serves the ECS and EKS adapters.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	awsplugin "github.com/magelift/magelift/providers/aws/plugin"
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
		HandshakeConfig: awsplugin.HandshakeConfig,
		Plugins:         awsplugin.PluginMap(&awsplugin.Server{Version: Version}),
		Logger:          hclog.New(&hclog.LoggerOptions{Level: hclog.Warn, Output: os.Stderr}),
	})
}
