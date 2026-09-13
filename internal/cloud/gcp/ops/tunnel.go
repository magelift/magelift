package ops

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/platform"
)

// RuntimeTunnel provides GCP's provider-native Cloud SQL proxy and delegates
// Kubernetes service targets to the shared GKE observer.
func (Module) RuntimeTunnel() platform.RuntimeTunnel {
	return Tunnel{observe: kube.NewObserveWithFactory(kube.ClientFromOutputs)}
}

type Tunnel struct {
	observe *kube.Observe
}

func (t Tunnel) PrepareTunnel(ctx context.Context, planned platform.PlannedStack, outputs map[string]any, query platform.TunnelQuery) (platform.ExecTarget, error) {
	target, err := platform.NormalizeTunnelTarget(query.Target)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	query.Target = target
	if err := platform.ValidateTunnelQuery(query); err != nil {
		return platform.ExecTarget{}, err
	}
	if target == platform.TunnelTargetDatabaseUI {
		return platform.ExecTarget{}, fmt.Errorf("GCP does not provision a database management UI; use the db tunnel with a local client: %w", platform.ErrNotSupported)
	}
	if target != platform.TunnelTargetDatabase {
		if t.observe == nil {
			t.observe = kube.NewObserveWithFactory(kube.ClientFromOutputs)
		}
		return t.observe.PrepareTunnel(ctx, planned, outputs, query)
	}
	if query.RemotePort != platform.DefaultTunnelRemotePort(target) {
		return platform.ExecTarget{}, fmt.Errorf("remote port %d is not supported for tunnel target %q (want %d): %w", query.RemotePort, target, platform.DefaultTunnelRemotePort(target), platform.ErrNotSupported)
	}
	if err := ctx.Err(); err != nil {
		return platform.ExecTarget{}, err
	}
	connectionName, err := platform.RequireStringOutput(outputs, platform.OutputDatabaseConnectionName)
	if err != nil {
		return platform.ExecTarget{}, fmt.Errorf("GCP Cloud SQL connection name is unavailable: %w", err)
	}
	if err := validateCloudSQLConnectionName(connectionName); err != nil {
		return platform.ExecTarget{}, err
	}
	return platform.ExecTarget{
		Launcher: "cloud-sql-proxy",
		Args: []string{
			"--private-ip", "--address", "127.0.0.1", "--port", strconv.Itoa(query.LocalPort), connectionName,
		},
	}, nil
}

func validateCloudSQLConnectionName(value string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return fmt.Errorf("GCP Cloud SQL connection name %q is invalid", value)
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || strings.ContainsAny(part, "\r\n\t /?") {
			return fmt.Errorf("GCP Cloud SQL connection name %q is invalid", value)
		}
	}
	return nil
}
