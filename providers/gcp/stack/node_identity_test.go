package stack

import (
	"slices"
	"testing"

	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestNewGKEGraphsUseOwnedCreationOnlyNodeIdentity(t *testing.T) {
	for _, runtimeID := range []sdk.RuntimeID{"gke-autopilot", "gke-standard"} {
		t.Run(string(runtimeID), func(t *testing.T) {
			in := gcpDeploymentInputs()
			in.Runtime = runtimeID
			spec, err := PlanFromInputs(in)
			if err != nil {
				t.Fatal(err)
			}
			mocks := &stackMocks{}
			if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-staging", mocks)); err != nil {
				t.Fatal(err)
			}
			clusters, pools := 0, 0
			for _, res := range mocks.resources {
				if res.TypeToken != "gcp:container/cluster:Cluster" && res.TypeToken != "gcp:container/nodePool:NodePool" {
					continue
				}
				if res.TypeToken == "gcp:container/cluster:Cluster" {
					clusters++
				} else {
					pools++
				}
				node := res.Inputs["nodeConfig"].ObjectValue()
				if node["serviceAccount"].StringValue() != "ml-shop-staging-nodes@example-gcp-project.iam.gserviceaccount.com" {
					t.Fatalf("shared or wrong node identity: %v", node["serviceAccount"])
				}
				cloudPlatform := false
				for _, scope := range node["oauthScopes"].ArrayValue() {
					if scope.StringValue() == "https://www.googleapis.com/auth/cloud-platform" {
						cloudPlatform = true
					}
				}
				if !cloudPlatform {
					t.Fatal("custom node identity cannot use IAM-controlled scopes")
				}
				if res.RegisterRPC == nil {
					t.Fatal("missing resource options")
				}
				for _, path := range []string{"nodeConfig.serviceAccount", "nodeConfig.oauthScopes"} {
					if !slices.Contains(res.RegisterRPC.GetIgnoreChanges(), path) {
						t.Fatalf("legacy immutable node identity could be replaced: %v", res.RegisterRPC.GetIgnoreChanges())
					}
				}
			}
			if clusters != 1 || (runtimeID == "gke-standard" && pools != 1) || (runtimeID == "gke-autopilot" && pools != 0) {
				t.Fatalf("clusters=%d pools=%d", clusters, pools)
			}
		})
	}
}
