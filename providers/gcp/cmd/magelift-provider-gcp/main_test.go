package main

import (
	"testing"
)

func TestNewProductionServerWiresFreshCredentials(t *testing.T) {
	t.Parallel()
	server := newProductionServer()
	if server.Version != Version {
		t.Fatalf("version = %q, want %q", server.Version, Version)
	}
	if server.KubeClients == nil {
		t.Fatal("production server has no client factory (would fall back to saved kubeconfig)")
	}
	// Exec/tunnel tokens resolve through ambient ADC automatically; the
	// live expiry phase covers ADC integration, unit tests cover mechanics.
}
