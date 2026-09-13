package kube

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

func TestPrepareTunnelQueueManagementIsLoopbackBound(t *testing.T) {
	obs := NewObserve(nil)
	target, err := obs.PrepareTunnel(context.Background(), nil, map[string]any{
		platform.OutputServiceName: "shop-web",
		platform.OutputQueueHost:   "shop-staging-rabbitmq",
		platform.OutputKubeconfig:  "apiVersion: v1\nkind: Config\n",
	}, platform.TunnelQuery{Target: platform.TunnelTargetQueueUI, LocalPort: 18080, RemotePort: 15672})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(target.CleanupPaths[0]) }()
	if target.Launcher != "kubectl" {
		t.Fatalf("launcher = %q, want kubectl", target.Launcher)
	}
	want := []string{"--kubeconfig", target.Args[1], "port-forward", "-n", "default", "--address", "127.0.0.1", "service/shop-staging-rabbitmq", "18080:15672"}
	if strings.Join(target.Args, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %#v, want %#v", target.Args, want)
	}
	if len(target.CleanupPaths) != 1 {
		t.Fatalf("cleanup paths = %#v, want one temporary kubeconfig", target.CleanupPaths)
	}
	if _, err := os.Stat(target.CleanupPaths[0]); err != nil {
		t.Fatalf("temporary kubeconfig was not created: %v", err)
	}
}

func TestPrepareTunnelSupportsApplicationAndSearchAPI(t *testing.T) {
	obs := NewObserve(nil)
	base := map[string]any{
		platform.OutputServiceName:    "shop-web",
		platform.OutputSearchEndpoint: "shop-search",
		platform.OutputKubeconfig:     "apiVersion: v1\nkind: Config\n",
	}
	for _, test := range []struct {
		name, target, service, portSpec string
		local, remote                   int
	}{
		{"app", platform.TunnelTargetApp, "service/shop-web", "18000:80", 18000, 80},
		{"search", platform.TunnelTargetSearch, "service/shop-search", "19200:9200", 19200, 9200},
	} {
		t.Run(test.name, func(t *testing.T) {
			outputs := map[string]any{}
			for key, value := range base {
				outputs[key] = value
			}
			target, err := obs.PrepareTunnel(context.Background(), nil, outputs, platform.TunnelQuery{Target: test.target, LocalPort: test.local, RemotePort: test.remote})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Remove(target.CleanupPaths[0]) }()
			if got := target.Args[len(target.Args)-2]; got != test.service {
				t.Fatalf("service arg = %q, want %q", got, test.service)
			}
			if got := target.Args[len(target.Args)-1]; got != test.portSpec {
				t.Fatalf("port spec = %q, want %q", got, test.portSpec)
			}
		})
	}
}

func TestPrepareTunnelRejectsUnprovisionedTargets(t *testing.T) {
	obs := NewObserve(nil)
	outputs := map[string]any{platform.OutputKubeconfig: "apiVersion: v1\nkind: Config\n", platform.OutputServiceName: "shop-web"}
	for _, target := range []string{platform.TunnelTargetDatabase, platform.TunnelTargetDatabaseUI, platform.TunnelTargetSearchUI} {
		_, err := obs.PrepareTunnel(context.Background(), nil, outputs, platform.TunnelQuery{Target: target, LocalPort: 10000, RemotePort: platform.DefaultTunnelRemotePort(target)})
		if !errors.Is(err, platform.ErrNotSupported) {
			t.Fatalf("target %q error = %v, want ErrNotSupported", target, err)
		}
	}
	_, err := obs.PrepareTunnel(context.Background(), nil, outputs, platform.TunnelQuery{Target: platform.TunnelTargetQueueUI, LocalPort: 10000, RemotePort: 15672})
	if !errors.Is(err, platform.ErrNotSupported) {
		t.Fatalf("missing queue error = %v, want ErrNotSupported", err)
	}
}

func TestPrepareTunnelHonorsCancellationBeforeWritingKubeconfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewObserve(nil).PrepareTunnel(ctx, nil, map[string]any{}, platform.TunnelQuery{Target: platform.TunnelTargetApp, LocalPort: 8080, RemotePort: 80})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
