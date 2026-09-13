//go:build floci_gcp

package flocigcp_test

import (
	"os"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cloud/gcp/endpoint"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func requireFlociGCP(t *testing.T) string {
	t.Helper()
	if os.Getenv("MAGELIFT_FLOCI_GCP") != "1" {
		t.Skip("set MAGELIFT_FLOCI_GCP=1 to run the floci-gcp integration test")
	}
	override, err := endpoint.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if override == "" {
		override = "http://127.0.0.1:4588"
	}
	return strings.TrimPrefix(strings.TrimPrefix(override, "https://"), "http://")
}

func flociGCPClientOptions(host string) []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(host),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
}
