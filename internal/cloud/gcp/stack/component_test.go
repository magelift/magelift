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
	if err == nil || !strings.Contains(err.Error(), "GCP stack requires") {
		t.Fatalf("expected GCP target rejection, got %v", err)
	}
}

func TestModuleRegistersRequiredOutputs(t *testing.T) {
	registry := platform.NewModuleRegistry()
	if err := registry.RegisterModule(Module{}); err != nil {
		t.Fatal(err)
	}
	module, found := registry.Module("gcp", "gke-autopilot")
	if !found || module.CertificationTier() != platform.TierExperimental {
		t.Fatal("expected experimental GCP module")
	}
}

func TestProgramBuildsMockGraph(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", GCPProject: "digital-lab-341608", Environment: "preview",
			Region: "europe-west1", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application:  Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     Artifact{ImageDigest: "ghcr.io/acourtiol/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:       NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c"}},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-custom-1-3840", CloudSQLAvailability: "ZONAL",
			MemorystoreNodeType: "SHARED_CORE_NANO", AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi",
			DesiredWebReplicas: 1, SearchMode: "opensearch", SearchReplicas: 1, QueueMode: "database",
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
}

func TestProgramBuildsStandardPresetGraph(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", GCPProject: "digital-lab-341608", Environment: "staging",
			Region: "europe-west1", EnvironmentClass: "staging", Preset: "standard",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/acourtiol/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c"}},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-custom-2-7680", CloudSQLAvailability: "REGIONAL",
			MemorystoreNodeType: "STANDARD_SMALL", MemorystoreReplicas: 1,
			AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi", DesiredWebReplicas: 2,
			SearchMode: "opensearch", SearchReplicas: 1, QueueMode: "rabbitmq", QueueReplicas: 1,
			QueueConsumerCount: 1, EnableCloudArmor: true,
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-staging", mocks)); err != nil {
		t.Fatal(err)
	}
	var sawBucket, sawArmor, sawSQL bool
	for _, res := range mocks.resources {
		switch res.TypeToken {
		case "gcp:storage/bucket:Bucket":
			sawBucket = true
		case "gcp:compute/securityPolicy:SecurityPolicy":
			sawArmor = true
		case "gcp:sql/databaseInstance:DatabaseInstance":
			sawSQL = true
			if got := res.Inputs["settings"].ObjectValue()["availabilityType"].StringValue(); got != "REGIONAL" {
				t.Fatalf("standard Cloud SQL availability = %q", got)
			}
		}
	}
	if !sawBucket || !sawArmor || !sawSQL {
		t.Fatalf("missing production resources bucket=%v armor=%v sql=%v (n=%d)", sawBucket, sawArmor, sawSQL, len(mocks.resources))
	}
}

func TestProgramBuildsHighAvailabilityPresetGraph(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", GCPProject: "digital-lab-341608", Environment: "prod",
			Region: "europe-west1", EnvironmentClass: "production", Preset: "high-availability",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/acourtiol/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c", "europe-west1-d"}},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-custom-2-7680", CloudSQLAvailability: "REGIONAL",
			MemorystoreNodeType: "STANDARD_SMALL", MemorystoreReplicas: 2,
			AutopilotCPURequest: "1", AutopilotMemoryRequest: "2Gi", DesiredWebReplicas: 3,
			SearchMode: "opensearch", SearchReplicas: 3, QueueMode: "rabbitmq", QueueReplicas: 2,
			QueueConsumerCount: 2, EnableCloudArmor: true,
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-prod", mocks)); err != nil {
		t.Fatal(err)
	}
	if len(mocks.resources) < 20 {
		t.Fatalf("expected a dense HA graph, got %d resources", len(mocks.resources))
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
	case "gcp:compute/network:Network":
		state["selfLink"] = resource.NewStringProperty("https://www.googleapis.com/compute/v1/projects/p/global/networks/n")
		state["name"] = resource.NewStringProperty(args.Name)
	case "gcp:compute/subnetwork:Subnetwork":
		state["name"] = resource.NewStringProperty(args.Name)
		state["selfLink"] = resource.NewStringProperty("https://www.googleapis.com/compute/v1/projects/p/regions/r/subnetworks/" + args.Name)
	case "gcp:sql/databaseInstance:DatabaseInstance":
		state["privateIpAddress"] = resource.NewStringProperty("10.20.1.5")
		state["connectionName"] = resource.NewStringProperty("p:europe-west1:sql")
		state["name"] = resource.NewStringProperty(args.Name)
	case "gcp:networkconnectivity/serviceConnectionPolicy:ServiceConnectionPolicy":
		state["name"] = resource.NewStringProperty(args.Name)
	case "gcp:memorystore/instance:Instance":
		state["instanceId"] = resource.NewStringProperty(args.Name)
		state["endpoints"] = resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{
				"connections": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"pscAutoConnection": resource.NewObjectProperty(resource.PropertyMap{
							"ipAddress": resource.NewStringProperty("10.20.2.8"),
						}),
					}),
				}),
			}),
		})
	case "gcp:container/cluster:Cluster":
		state["name"] = resource.NewStringProperty(args.Name)
		state["endpoint"] = resource.NewStringProperty("1.2.3.4")
		state["masterAuth"] = resource.NewObjectProperty(resource.PropertyMap{
			"clusterCaCertificate": resource.NewStringProperty("Y2E="),
		})
	case "gcp:storage/bucket:Bucket":
		state["name"] = resource.NewStringProperty(args.Name)
		state["url"] = resource.NewStringProperty("gs://" + args.Name)
	case "gcp:compute/securityPolicy:SecurityPolicy":
		state["name"] = resource.NewStringProperty(args.Name)
	case "kubernetes:core/v1:Service":
		state["status"] = resource.NewObjectProperty(resource.PropertyMap{
			"loadBalancer": resource.NewObjectProperty(resource.PropertyMap{
				"ingress": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{"ip": resource.NewStringProperty("35.1.2.3")}),
				}),
			}),
		})
		state["metadata"] = resource.NewObjectProperty(resource.PropertyMap{"name": resource.NewStringProperty(args.Name)})
	case "kubernetes:apps/v1:Deployment":
		state["metadata"] = resource.NewObjectProperty(resource.PropertyMap{"name": resource.NewStringProperty(args.Name)})
	case "random:index/randomPassword:RandomPassword":
		state["result"] = resource.NewStringProperty("generated-password-value-32chars!!")
	case "gcp:secretmanager/secret:Secret":
		state["secretId"] = resource.NewStringProperty(args.Name)
	}
	return args.Name + "-id", state, nil
}

func (m *stackMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	switch args.Token {
	case "gcp:organizations/getClientConfig:getClientConfig":
		return resource.PropertyMap{
			"accessToken": resource.MakeSecret(resource.NewStringProperty("mock-gcp-access-token")),
			"project":     resource.NewStringProperty("digital-lab-341608"),
		}, nil
	default:
		return resource.PropertyMap{}, nil
	}
}
