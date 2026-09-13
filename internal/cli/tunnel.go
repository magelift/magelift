package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	"github.com/spf13/cobra"
)

type tunnelCommandResult struct {
	Environment string   `json:"environment" yaml:"environment"`
	Target      string   `json:"target" yaml:"target"`
	LocalPort   int      `json:"localPort" yaml:"localPort"`
	RemotePort  int      `json:"remotePort" yaml:"remotePort"`
	Launcher    string   `json:"launcher" yaml:"launcher"`
	Args        []string `json:"args" yaml:"args"`
}

func tunnelCommand(o *options) *cobra.Command {
	var target string
	var localPort, remotePort int
	var sessionOnly bool
	command := &cobra.Command{
		Use:   "tunnel",
		Short: "Forward a private Magento service to localhost",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o == nil {
				return &exitError{code: 3, err: errors.New("tunnel options are required")}
			}
			result, execTarget, err := o.prepareTunnel(cmd.Context(), target, localPort, remotePort)
			if err != nil {
				return err
			}
			if sessionOnly {
				return o.write(result)
			}
			return o.runExecTarget(cmd.Context(), execTarget)
		},
	}
	command.Flags().StringVar(&target, "target", platform.TunnelTargetApp, "private target: app, db, db-ui, queue, queue-ui, search, or search-ui")
	command.Flags().IntVar(&localPort, "local-port", 0, "local loopback port (defaults to the target port, or 8080 for app)")
	command.Flags().IntVar(&remotePort, "remote-port", 0, "provider-side port (defaults to the target's verified service port)")
	command.Flags().BoolVar(&sessionOnly, "session-only", false, "print the resolved tunnel command without starting it")
	return command
}

func (o *options) runtimeTunnel() (platform.RuntimeTunnel, error) {
	if o.testRuntimeTunnel != nil {
		return o.testRuntimeTunnel, nil
	}
	module, err := o.resolveModule()
	if err != nil {
		return nil, err
	}
	tunnel := platform.ModuleRuntimeTunnel(module)
	if tunnel == nil {
		return nil, &exitError{code: 3, err: fmt.Errorf("%s", notSupportedForModule(module, "private tunnels"))}
	}
	return tunnel, nil
}

func (o *options) prepareTunnel(ctx context.Context, requestedTarget string, localPort, remotePort int) (tunnelCommandResult, platform.ExecTarget, error) {
	target, err := platform.NormalizeTunnelTarget(requestedTarget)
	if err != nil {
		return tunnelCommandResult{}, platform.ExecTarget{}, invalid(err)
	}
	if localPort == 0 {
		localPort = platform.DefaultTunnelLocalPort(target)
	}
	if remotePort == 0 {
		remotePort = platform.DefaultTunnelRemotePort(target)
	}
	query := platform.TunnelQuery{Target: target, LocalPort: localPort, RemotePort: remotePort}
	if err := platform.ValidateTunnelQuery(query); err != nil {
		return tunnelCommandResult{}, platform.ExecTarget{}, invalid(err)
	}
	tunnel, err := o.runtimeTunnel()
	if err != nil {
		return tunnelCommandResult{}, platform.ExecTarget{}, err
	}
	environment, planned, outputs, err := o.plannedOutputs(ctx)
	if err != nil {
		return tunnelCommandResult{}, platform.ExecTarget{}, err
	}
	execTarget, err := tunnel.PrepareTunnel(ctx, planned, outputs, query)
	if err != nil {
		return tunnelCommandResult{}, platform.ExecTarget{}, &exitError{code: 3, err: err}
	}
	return tunnelCommandResult{
		Environment: environment,
		Target:      target,
		LocalPort:   localPort,
		RemotePort:  remotePort,
		Launcher:    execTarget.Launcher,
		Args:        previewTunnelArgs(execTarget.Args),
	}, execTarget, nil
}

func previewTunnelArgs(args []string) []string {
	preview := append([]string(nil), args...)
	for index := 0; index+1 < len(preview); index++ {
		if strings.TrimSpace(preview[index]) == "--kubeconfig" {
			preview[index+1] = "<temporary-kubeconfig>"
		}
	}
	return preview
}
