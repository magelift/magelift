package stack

import (
	"strings"
	"sync"
	"testing"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestPlanFromConfigRejectsAWSTarget(t *testing.T) {
	_, err := PlanFromConfig(config.Config{
		Target: config.Target{Provider: "aws", Runtime: "ecs-fargate"},
	}, "staging")
	if err == nil || !strings.Contains(err.Error(), "OVH stack requires") {
		t.Fatalf("expected OVH target rejection, got %v", err)
	}
}

func TestModuleRegistersRequiredOutputs(t *testing.T) {
	registry := platform.NewModuleRegistry()
	if err := registry.RegisterModule(Module{}); err != nil {
		t.Fatal(err)
	}
	module, found := registry.Module("ovh", "mks")
	if !found || module.CertificationTier() != platform.TierExperimental {
		t.Fatal("expected experimental OVH module")
	}
}

func TestProgramBuildsMockGraph(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", ServiceName: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Environment: "preview",
			Region: "GRA9", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application:  Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     Artifact{ImageDigest: "ghcr.io/acourtiol/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:       NetworkPolicy{NetworkCIDR: "10.30.0.0/16", Zones: []string{"GRA9"}},
		Catalog:      CatalogSelection{DatabaseFlavor: "db1-4", DatabasePlan: "essential", ValkeyFlavor: "db1-4", ValkeyPlan: "essential", NodeFlavor: "b3-8", NodeCount: 1, CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 1},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-preview", mocks))
	if err != nil {
		t.Fatal(err)
	}
	if len(mocks.resources) == 0 {
		t.Fatal("expected mock resources")
	}

	wantTokens := []string{
		"ovh:CloudProject/networkPrivate:NetworkPrivate",
		"ovh:CloudProject/networkPrivateSubnetV2:NetworkPrivateSubnetV2",
		"ovh:CloudProject/database:Database",
		"ovh:CloudProject/kube:Kube",
		"ovh:CloudProject/kubeNodePool:KubeNodePool",
		"kubernetes:apps/v1:Deployment",
		"kubernetes:batch/v1:Job",
		"kubernetes:core/v1:Service",
		"random:index/randomPassword:RandomPassword",
	}
	seen := map[string]bool{}
	for _, r := range mocks.resources {
		seen[r.TypeToken] = true
		if strings.HasPrefix(r.TypeToken, "aws:") || strings.HasPrefix(r.TypeToken, "gcp:") {
			t.Fatalf("unexpected foreign provider token %s", r.TypeToken)
		}
	}
	for _, token := range wantTokens {
		if !seen[token] {
			t.Fatalf("missing expected token %s in %#v", token, seen)
		}
	}
	if mocks.databaseCount < 2 {
		t.Fatalf("expected MySQL + Valkey database resources, got %d", mocks.databaseCount)
	}
}

type stackMocks struct {
	mu            sync.Mutex
	resources     []pulumi.MockResourceArgs
	databaseCount int
}

func (m *stackMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	if args.TypeToken == "ovh:CloudProject/database:Database" {
		m.databaseCount++
	}
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "ovh:CloudProject/networkPrivate:NetworkPrivate":
		state["name"] = resource.NewStringProperty(args.Name)
		state["status"] = resource.NewStringProperty("ACTIVE")
	case "ovh:CloudProject/networkPrivateSubnetV2:NetworkPrivateSubnetV2":
		state["name"] = resource.NewStringProperty(args.Name)
		state["cidr"] = resource.NewStringProperty("10.30.0.0/24")
		state["gatewayIp"] = resource.NewStringProperty("10.30.0.1")
	case "ovh:CloudProject/database:Database":
		state["endpoints"] = resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{
				"domain": resource.NewStringProperty("db.example.ovh.net"),
				"port":   resource.NewNumberProperty(3306),
			}),
		})
		state["status"] = resource.NewStringProperty("READY")
	case "ovh:CloudProject/kube:Kube":
		state["name"] = resource.NewStringProperty(args.Name)
		state["kubeconfig"] = resource.NewStringProperty("apiVersion: v1\nkind: Config\n")
		state["url"] = resource.NewStringProperty("https://k8s.example.ovh.net")
		state["status"] = resource.NewStringProperty("READY")
		state["nodesSubnetId"] = resource.NewStringProperty("subnet-id")
	case "ovh:CloudProject/kubeNodePool:KubeNodePool":
		state["name"] = resource.NewStringProperty(args.Name)
		state["status"] = resource.NewStringProperty("READY")
	case "kubernetes:core/v1:Service":
		state["status"] = resource.NewObjectProperty(resource.PropertyMap{
			"loadBalancer": resource.NewObjectProperty(resource.PropertyMap{
				"ingress": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{"ip": resource.NewStringProperty("51.1.2.3")}),
				}),
			}),
		})
		state["metadata"] = resource.NewObjectProperty(resource.PropertyMap{"name": resource.NewStringProperty(args.Name)})
	case "random:index/randomPassword:RandomPassword":
		state["result"] = resource.NewStringProperty("generated-password-value-32chars!!")
	}
	return args.Name + "-id", state, nil
}

func (m *stackMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}
