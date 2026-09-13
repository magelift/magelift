package platform

import (
	"context"
	"fmt"
	"strings"
)

const (
	TunnelTargetApp        = "app"
	TunnelTargetDatabase   = "db"
	TunnelTargetDatabaseUI = "db-ui"
	TunnelTargetQueue      = "queue"
	TunnelTargetQueueUI    = "queue-ui"
	TunnelTargetSearch     = "search"
	TunnelTargetSearchUI   = "search-ui"
)

// TunnelQuery selects one provider-owned private endpoint. Ports are local
// listener and provider-side service ports respectively.
type TunnelQuery struct {
	Target     string
	LocalPort  int
	RemotePort int
}

// RuntimeTunnel is an optional day-2 capability. It returns a long-running
// launcher target so the CLI can apply the same cancellation and cleanup rules
// as remote exec without making RuntimeObserve a breaking interface.
type RuntimeTunnel interface {
	PrepareTunnel(ctx context.Context, planned PlannedStack, outputs map[string]any, query TunnelQuery) (ExecTarget, error)
}

// HasRuntimeTunnel is implemented by modules that expose verified private
// service forwarding or a provider-native tunnel proxy.
type HasRuntimeTunnel interface {
	RuntimeTunnel() RuntimeTunnel
}

// ModuleRuntimeTunnel returns the optional tunnel capability of a module.
func ModuleRuntimeTunnel(module StackModule) RuntimeTunnel {
	if module == nil {
		return nil
	}
	provider, ok := module.(HasRuntimeTunnel)
	if !ok {
		return nil
	}
	return provider.RuntimeTunnel()
}

// NormalizeTunnelTarget accepts the short names used in CLI commands and a
// small set of provider-neutral aliases.
func NormalizeTunnelTarget(target string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case TunnelTargetApp, "application", "web":
		return TunnelTargetApp, nil
	case TunnelTargetDatabase, "database":
		return TunnelTargetDatabase, nil
	case TunnelTargetDatabaseUI, "database-ui":
		return TunnelTargetDatabaseUI, nil
	case TunnelTargetQueue, "rabbitmq":
		return TunnelTargetQueue, nil
	case TunnelTargetQueueUI, "rabbitmq-ui", "rabbitmq-management":
		return TunnelTargetQueueUI, nil
	case TunnelTargetSearch, "opensearch":
		return TunnelTargetSearch, nil
	case TunnelTargetSearchUI, "opensearch-ui", "opensearch-dashboards":
		return TunnelTargetSearchUI, nil
	default:
		return "", fmt.Errorf("unsupported tunnel target %q (want app, db, db-ui, queue, queue-ui, search, or search-ui)", target)
	}
}

// DefaultTunnelRemotePort provides the fixed service port for known logical
// targets. Unsupported UI targets still receive a conventional port so the
// adapter can return a precise unprovisioned-capability error.
func DefaultTunnelRemotePort(target string) int {
	switch target {
	case TunnelTargetApp:
		return 80
	case TunnelTargetDatabase:
		return 3306
	case TunnelTargetDatabaseUI:
		return 8080
	case TunnelTargetQueue:
		return 5672
	case TunnelTargetQueueUI:
		return 15672
	case TunnelTargetSearch:
		return 9200
	case TunnelTargetSearchUI:
		return 5601
	default:
		return 0
	}
}

// DefaultTunnelLocalPort keeps the application convenient in a browser while
// preserving the provider service port for protocol-oriented targets.
func DefaultTunnelLocalPort(target string) int {
	if target == TunnelTargetApp {
		return 8080
	}
	return DefaultTunnelRemotePort(target)
}

// ValidateTunnelQuery checks the portable portion of the tunnel request before
// an adapter creates temporary access material or launches a process.
func ValidateTunnelQuery(query TunnelQuery) error {
	target, err := NormalizeTunnelTarget(query.Target)
	if err != nil {
		return err
	}
	if query.LocalPort < 1 || query.LocalPort > 65535 {
		return fmt.Errorf("local tunnel port for %q must be between 1 and 65535", target)
	}
	if query.RemotePort < 1 || query.RemotePort > 65535 {
		return fmt.Errorf("remote tunnel port for %q must be between 1 and 65535", target)
	}
	return nil
}
