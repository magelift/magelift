package plugin

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestDescribeAdvertisesOnlyImplementedOperations(t *testing.T) {
	var response sdk.DescribeResponse
	server := &Server{Version: "v0.0.0-test"}
	if err := server.Describe(&sdk.DescribeRequest{ProtocolVersion: sdk.ProtocolV1}, &response); err != nil {
		t.Fatal(err)
	}
	if response.ProviderID != "azure" || len(response.Runtimes) != 1 || response.Runtimes[0].Runtime != "aks" {
		t.Fatalf("unexpected provider descriptor: %#v", response)
	}
	if err := sdk.RequireOperation(response.Operations, sdk.OpDescribe, "1.0"); err != nil {
		t.Fatal(err)
	}
	if err := sdk.RequireOperation(response.Operations, sdk.OpDeployAppPhase, "1.0"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateConfig(t *testing.T) {
	server := &Server{}
	for _, test := range []struct {
		name, runtime, target string
		valid                 bool
		problem               string
	}{
		{"valid", "aks", "subscriptionId: sub\nresourceGroup: shop\n", true, ""},
		{"missing group", "aks", "subscriptionId: sub\n", false, "resourceGroup"},
		{"wrong runtime", "gke", "subscriptionId: sub\nresourceGroup: shop\n", false, "runtime"},
		{"unknown field", "aks", "subscriptionId: sub\nresourceGroup: shop\nunknown: value\n", false, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var response sdk.ValidateConfigResult
			err := server.ValidateConfig(&sdk.ValidateConfigRequest{ProtocolVersion: sdk.ProtocolV1, Runtime: test.runtime, TargetBlock: []byte(test.target)}, &response)
			if err != nil {
				t.Fatal(err)
			}
			if response.Valid != test.valid {
				t.Fatalf("valid = %v, result = %#v", response.Valid, response)
			}
			if test.problem != "" {
				message := strings.Join(response.Problems, "; ")
				if response.Error != nil {
					message += response.Error.Message
				}
				if !strings.Contains(message, test.problem) {
					t.Fatalf("problem = %q, want %q", message, test.problem)
				}
			}
		})
	}
}
