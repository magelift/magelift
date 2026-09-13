package runtime

import (
	"reflect"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/cloud/scaleway/naming"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestPoolPlacementsDistributeNodesAcrossAvailabilityZones(t *testing.T) {
	t.Parallel()

	placements, err := poolPlacements("shop-preview-app", 5, []string{"fr-par-1", "fr-par-2", "fr-par-3"})
	if err != nil {
		t.Fatal(err)
	}
	want := []poolPlacement{
		{Name: "shop-preview-app-pool-1", Zone: "fr-par-1", Size: 2},
		{Name: "shop-preview-app-pool-2", Zone: "fr-par-2", Size: 2},
		{Name: "shop-preview-app-pool-3", Zone: "fr-par-3", Size: 1},
	}
	if !reflect.DeepEqual(placements, want) {
		t.Fatalf("pool placements = %#v, want %#v", placements, want)
	}
}

func TestPoolPlacementsPreserveSinglePoolDefault(t *testing.T) {
	t.Parallel()

	placements, err := poolPlacements("shop-preview-app", 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []poolPlacement{{Name: "shop-preview-app-pool", Size: 4}}
	if !reflect.DeepEqual(placements, want) {
		t.Fatalf("pool placements = %#v, want %#v", placements, want)
	}
}

func TestPoolPlacementsRejectInsufficientOrInvalidZones(t *testing.T) {
	t.Parallel()

	for name, zones := range map[string][]string{
		"insufficient nodes": {"fr-par-1", "fr-par-2", "fr-par-3"},
		"empty zone":         {"fr-par-1", ""},
		"duplicate zone":     {"fr-par-1", "fr-par-1"},
	} {
		t.Run(name, func(t *testing.T) {
			nodeCount := 3
			if name == "insufficient nodes" {
				nodeCount = 2
			}
			if _, err := poolPlacements("shop-preview-app", nodeCount, zones); err == nil {
				t.Fatal("invalid multi-zone placement was accepted")
			}
		})
	}
}

func TestZoneSpreadConstraintsRequireEnoughReplicas(t *testing.T) {
	t.Parallel()
	labels := pulumi.StringMap{"app": pulumi.String("shop-web")}
	if got := zoneSpreadConstraints(1, []string{"fr-par-1", "fr-par-2"}, labels); got != nil {
		t.Fatal("single replica unexpectedly received a strict multi-zone spread constraint")
	}
	if got := zoneSpreadConstraints(2, []string{"fr-par-1", "fr-par-2"}, labels); len(got) != 1 {
		t.Fatalf("spread constraints = %#v, want one constraint", got)
	}
}

func TestBuildKubeconfigUsesStaticTokenNotExecPlugin(t *testing.T) {
	t.Parallel()
	kubeconfig := kube.BuildStaticTokenKubeconfig("magelift_shop-preview-kapsule", naming.FormatURL("https://xxx.pub.k8s.fr-par.scw.cloud:6443"), "Y2E=", "mock-scw-token")

	if strings.Contains(kubeconfig, "exec:") {
		t.Fatal("kubeconfig must not depend on an exec auth plugin")
	}
	if !strings.Contains(kubeconfig, "token: mock-scw-token") {
		t.Fatalf("kubeconfig missing static token auth: %s", kubeconfig)
	}
	if !strings.Contains(kubeconfig, "certificate-authority-data: Y2E=") {
		t.Fatal("kubeconfig missing cluster CA")
	}
	if !strings.Contains(kubeconfig, "server: https://xxx.pub.k8s.fr-par.scw.cloud:6443") {
		t.Fatal("kubeconfig missing cluster endpoint")
	}
	if !strings.Contains(kubeconfig, "name: magelift_shop-preview-kapsule") {
		t.Fatal("kubeconfig missing expected context name")
	}
}

func TestBuildKubeconfigAddsSchemeWhenHostHasNone(t *testing.T) {
	t.Parallel()
	kubeconfig := kube.BuildStaticTokenKubeconfig("magelift_shop-preview-kapsule", naming.FormatURL("1.2.3.4:6443"), "Y2E=", "mock-scw-token")
	if !strings.Contains(kubeconfig, "server: https://1.2.3.4:6443") {
		t.Fatalf("expected an https scheme to be added: %s", kubeconfig)
	}
}
