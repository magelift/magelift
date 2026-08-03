package pipeline

import "testing"

func TestContainerAdapterDisablesFinalizeNetwork(t *testing.T) {
	adapter := ContainerAdapter{Network: "bridge"}
	if got := adapter.runner(pinnedBuilder, adapter.Network).Network; got != "bridge" {
		t.Fatalf("prepare network = %q", got)
	}
	if got := adapter.runner(pinnedBuilder, "none").Network; got != "none" {
		t.Fatalf("finalize network = %q", got)
	}
}
