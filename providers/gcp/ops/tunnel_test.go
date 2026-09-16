package ops_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/providers/gcp/ops"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
)

func TestRuntimeTunnelUsesPrivateCloudSQLProxy(t *testing.T) {
	tunnel := (ops.Tunnel{})
	target, err := tunnel.PrepareTunnel(context.Background(), gcpstack.Spec{}, map[string]any{
		platform.OutputDatabaseConnectionName: "example-project:europe-west1:shop-staging-sql",
	}, platform.TunnelQuery{Target: platform.TunnelTargetDatabase, LocalPort: 13306, RemotePort: 3306})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--private-ip", "--address", "127.0.0.1", "--port", "13306", "example-project:europe-west1:shop-staging-sql"}
	if strings.Join(target.Args, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %#v, want %#v", target.Args, want)
	}
	if target.Launcher != "cloud-sql-proxy" {
		t.Fatalf("launcher = %q, want cloud-sql-proxy", target.Launcher)
	}
}

func TestRuntimeTunnelRejectsDatabaseManagementUI(t *testing.T) {
	_, err := (ops.Tunnel{}).PrepareTunnel(context.Background(), gcpstack.Spec{}, nil, platform.TunnelQuery{
		Target: platform.TunnelTargetDatabaseUI, LocalPort: 18080, RemotePort: 8080,
	})
	if !errors.Is(err, platform.ErrNotSupported) {
		t.Fatalf("error = %v, want ErrNotSupported", err)
	}
}

func TestRuntimeTunnelRejectsMalformedCloudSQLConnectionName(t *testing.T) {
	_, err := (ops.Tunnel{}).PrepareTunnel(context.Background(), gcpstack.Spec{}, map[string]any{
		platform.OutputDatabaseConnectionName: "project:europe-west1:instance?port=3306",
	}, platform.TunnelQuery{Target: platform.TunnelTargetDatabase, LocalPort: 13306, RemotePort: 3306})
	if err == nil || !strings.Contains(err.Error(), "connection name") {
		t.Fatalf("error = %v, want malformed connection-name error", err)
	}
}
