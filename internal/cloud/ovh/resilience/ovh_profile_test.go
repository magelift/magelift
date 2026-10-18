package resilience

import (
	"context"
	"testing"
)

func TestNewOVHDatabaseNativeAPIRequiresInjectedClient(t *testing.T) {
	_, err := NewOVHDatabaseNativeAPI(context.Background(), NativeAPIConfig{DatabaseProjectID: "8728028545db487baeee2e472e7e96dd"}, nil)
	if err == nil || err.Error() != "OVHcloud Public Cloud Database API client is required" {
		t.Fatalf("nil client error = %v", err)
	}
}

func TestNewOVHClientFromProfileRejectsUnsafeName(t *testing.T) {
	_, err := NewOVHClientFromProfile("bad\nprofile")
	if err == nil {
		t.Fatal("expected unsafe profile name to be rejected")
	}
}
