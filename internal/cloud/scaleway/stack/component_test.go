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
	if err == nil || !strings.Contains(err.Error(), "Scaleway stack requires") {
		t.Fatalf("expected Scaleway target rejection, got %v", err)
	}
}

func TestModuleRegistersRequiredOutputs(t *testing.T) {
	registry := platform.NewModuleRegistry()
	if err := registry.RegisterModule(Module{}); err != nil {
		t.Fatal(err)
	}
	module, found := registry.Module("scaleway", "kapsule")
	if !found || module.CertificationTier() != platform.TierExperimental {
		t.Fatal("expected experimental Scaleway module")
	}
}

func TestProgramBuildsMockGraph(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", ScalewayProject: "11111111-1111-1111-1111-111111111111", Environment: "preview",
			Region: "fr-par", Zone: "fr-par-1", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/acourtiol/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "172.16.0.0/22", Zones: []string{"fr-par-1"}},
		Catalog: CatalogSelection{
			DatabaseNodeType: "DB-DEV-S", RedisNodeType: "RED1-MICRO", CacheMode: "redis",
			KapsuleVersion: "1.29.1", NodeType: "DEV1-M", NodeCount: 2,
			CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 1,
		},
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
		"scaleway:network/vpc:Vpc",
		"scaleway:network/privateNetwork:PrivateNetwork",
		"scaleway:databases/instance:Instance",
		"scaleway:redis/cluster:Cluster",
		"scaleway:kubernetes/cluster:Cluster",
		"scaleway:kubernetes/pool:Pool",
		"kubernetes:apps/v1:Deployment",
		"kubernetes:core/v1:Service",
		"kubernetes:batch/v1:Job",
		"random:index/randomPassword:RandomPassword",
	}
	seen := make(map[string]bool, len(mocks.resources))
	for _, resource := range mocks.resources {
		seen[resource.TypeToken] = true
		if strings.HasPrefix(resource.TypeToken, "aws:") || strings.HasPrefix(resource.TypeToken, "gcp:") {
			t.Fatalf("Scaleway program registered a non-Scaleway cloud resource: %s", resource.TypeToken)
		}
	}
	for _, token := range wantTokens {
		if !seen[token] {
			t.Fatalf("expected resource token %q in graph, got %v", token, mocks.tokens())
		}
	}
}

type stackMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *stackMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "scaleway:network/vpc:Vpc":
		state["name"] = resource.NewStringProperty(args.Name)
	case "scaleway:network/privateNetwork:PrivateNetwork":
		state["name"] = resource.NewStringProperty(args.Name)
	case "scaleway:databases/instance:Instance":
		state["name"] = resource.NewStringProperty(args.Name)
		state["endpointIp"] = resource.NewStringProperty("172.16.0.5")
		state["loadBalancer"] = resource.NewObjectProperty(resource.PropertyMap{
			"ip": resource.NewStringProperty("172.16.0.6"),
		})
	case "scaleway:redis/cluster:Cluster":
		state["name"] = resource.NewStringProperty(args.Name)
		state["connectionString"] = resource.NewStringProperty("redis://magelift:secret@172.16.0.7:6379/0")
	case "scaleway:kubernetes/cluster:Cluster":
		state["name"] = resource.NewStringProperty(args.Name)
		state["version"] = resource.NewStringProperty("1.29.1")
		state["apiserverUrl"] = resource.NewStringProperty("https://cluster.pub.k8s.fr-par.scw.cloud:6443")
		state["kubeconfigs"] = resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{
				"host":                 resource.NewStringProperty("https://cluster.pub.k8s.fr-par.scw.cloud:6443"),
				"token":                resource.NewStringProperty("mock-scw-token"),
				"clusterCaCertificate": resource.NewStringProperty("Y2E="),
			}),
		})
	case "scaleway:kubernetes/pool:Pool":
		state["name"] = resource.NewStringProperty(args.Name)
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

func (m *stackMocks) tokens() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	tokens := make([]string, len(m.resources))
	for index, resource := range m.resources {
		tokens[index] = resource.TypeToken
	}
	return tokens
}
