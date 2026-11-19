package stack

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestSelfHostedRedisCacheAndSession(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Services = map[string]sdk.ServiceBinding{
		"cache":   {Family: "redis", Ownership: "self-hosted", Version: "7.2", EvictionPolicy: "allkeys-lru"},
		"session": {Family: "redis", Ownership: "self-hosted", Version: "7.2", Image: "registry.example.invalid/redis:7.2", EvictionPolicy: "noeviction"},
	}
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-staging", mocks)); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range mocks.resources {
		if r.TypeToken == "gcp:memorystore/instance:Instance" {
			t.Fatal("self-hosted Redis provisioned Memorystore")
		}
		if r.TypeToken != "kubernetes:apps/v1:StatefulSet" {
			continue
		}
		raw, _ := json.Marshal(r.Inputs.Mappable())
		text := string(raw)
		for _, tc := range []struct{ suffix, image, policy string }{{"-cache", "redis:7.2", "allkeys-lru"}, {"-session", "registry.example.invalid/redis:7.2", "noeviction"}} {
			if strings.HasSuffix(r.Name, tc.suffix) {
				seen[tc.suffix] = true
				for _, term := range []string{tc.image, tc.policy, "redis-server", "--requirepass", "--maxmemory"} {
					if !strings.Contains(text, term) {
						t.Errorf("%s missing %s", r.Name, term)
					}
				}
				if strings.Contains(text, "valkey-server") {
					t.Errorf("Redis using Valkey executable: %s", r.Name)
				}
			}
		}
	}
	if !seen["-cache"] || !seen["-session"] {
		t.Fatalf("missing stores: %v", seen)
	}
}
