package providerhost

import (
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
)

// Serve is the subprocess entrypoint for magelift-provider-* binaries.
// Describe returns first-party stack identity. Plan and Program speak the
// public sdk.Module JSON contract. Magento cells stay in-process until the
// CLI is wired to Dial.
func Serve(api API) {
	if api == nil {
		panic("providerhost.Serve requires a provider API")
	}
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         pluginSet(api),
		GRPCServer:      plugin.DefaultGRPCServer,
		Logger:          hclog.NewNullLogger(),
	})
}
