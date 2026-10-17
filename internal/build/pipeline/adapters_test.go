package pipeline

import "testing"

func TestContainerAdapterDisablesFinalizeNetwork(t *testing.T) {
	adapter := ContainerAdapter{Network: "bridge", Platform: "linux/amd64"}
	if got := adapter.runner(pinnedBuilder, adapter.Network).Network; got != "bridge" {
		t.Fatalf("prepare network = %q", got)
	}
	if got := adapter.runner(pinnedBuilder, "none").Network; got != "none" {
		t.Fatalf("finalize network = %q", got)
	}
	if got := adapter.runner(pinnedBuilder, adapter.Network).Platform; got != "linux/amd64" {
		t.Fatalf("prepare platform = %q", got)
	}
}

func TestContainerAdapterPropagatesMageLiftVersion(t *testing.T) {
	adapter := ContainerAdapter{MageLiftVersion: "0.0.0-dev", Platform: "linux/amd64"}
	if got := adapter.runner(pinnedBuilder, adapter.Network).MageLiftVersion; got != "0.0.0-dev" {
		t.Fatalf("MageLift version = %q", got)
	}
	if got := adapter.runner(pinnedBuilder, "none").MageLiftVersion; got != "0.0.0-dev" {
		t.Fatalf("finalize MageLift version = %q", got)
	}
}
