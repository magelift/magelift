package stack

import (
	"slices"
	"testing"

	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ownedNodeServiceAccount = "ml-shop-staging-nodes@example-gcp-project.iam.gserviceaccount.com"

var (
	autopilotClusterIgnoreChanges = []string{
		"nodeConfig.serviceAccount",
		"nodeConfig.oauthScopes",
		"clusterAutoscaling.autoProvisioningDefaults.serviceAccount",
		"clusterAutoscaling.autoProvisioningDefaults.oauthScopes",
	}
	standardClusterIgnoreChanges = []string{
		"nodeConfig.serviceAccount",
		"nodeConfig.oauthScopes",
	}
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
					if runtimeID == "gke-autopilot" {
						assertAutopilotClusterNodeIdentity(t, res)
					} else {
						assertStandardClusterNodeIdentity(t, res)
					}
					continue
				}
				pools++
				assertNodePoolNodeIdentity(t, res)
			}
			if clusters != 1 || (runtimeID == "gke-standard" && pools != 1) || (runtimeID == "gke-autopilot" && pools != 0) {
				t.Fatalf("clusters=%d pools=%d", clusters, pools)
			}
		})
	}
}

func assertNodeIdentityOnNodeConfig(t *testing.T, node resource.PropertyMap) {
	if node["serviceAccount"].StringValue() != ownedNodeServiceAccount {
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
}

func assertIgnoreChangesExact(t *testing.T, res pulumi.MockResourceArgs, expected []string) {
	if res.RegisterRPC == nil {
		t.Fatal("missing resource options")
	}
	got := res.RegisterRPC.GetIgnoreChanges()
	if !slices.Equal(got, expected) {
		t.Fatalf("unexpected ignoreChanges: got %v want %v", got, expected)
	}
}

func assertAutopilotClusterNodeIdentity(t *testing.T, res pulumi.MockResourceArgs) {
	node := res.Inputs["nodeConfig"].ObjectValue()
	assertNodeIdentityOnNodeConfig(t, node)

	autoscalingVal, ok := res.Inputs["clusterAutoscaling"]
	if !ok || !autoscalingVal.IsObject() {
		t.Fatal("Autopilot cluster must set clusterAutoscaling.autoProvisioningDefaults")
	}
	autoscaling := autoscalingVal.ObjectValue()
	if enabled, ok := autoscaling["enabled"]; ok && enabled.HasValue() && enabled.BoolValue() {
		t.Fatal("Autopilot must not enable standard node auto-provisioning")
	}
	defaults := autoscaling["autoProvisioningDefaults"].ObjectValue()
	if defaults["serviceAccount"].StringValue() != ownedNodeServiceAccount {
		t.Fatalf("Autopilot auto_provisioning_defaults service account: %v", defaults["serviceAccount"])
	}
	cloudPlatform := false
	for _, scope := range defaults["oauthScopes"].ArrayValue() {
		if scope.StringValue() == "https://www.googleapis.com/auth/cloud-platform" {
			cloudPlatform = true
		}
	}
	if !cloudPlatform {
		t.Fatal("Autopilot auto_provisioning_defaults missing cloud-platform scope")
	}
	assertIgnoreChangesExact(t, res, autopilotClusterIgnoreChanges)
}

func assertStandardClusterNodeIdentity(t *testing.T, res pulumi.MockResourceArgs) {
	node := res.Inputs["nodeConfig"].ObjectValue()
	assertNodeIdentityOnNodeConfig(t, node)
	if _, ok := res.Inputs["clusterAutoscaling"]; ok {
		t.Fatal("GKE Standard cluster must not set clusterAutoscaling")
	}
	assertIgnoreChangesExact(t, res, standardClusterIgnoreChanges)
}

func assertNodePoolNodeIdentity(t *testing.T, res pulumi.MockResourceArgs) {
	node := res.Inputs["nodeConfig"].ObjectValue()
	assertNodeIdentityOnNodeConfig(t, node)
	assertIgnoreChangesExact(t, res, standardClusterIgnoreChanges)
}
