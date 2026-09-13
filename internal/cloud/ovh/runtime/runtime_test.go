package runtime

import (
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Ceiling: the same pool-before-workload DependsOn assertion applies to the
// Scaleway Kapsule and GCP GKE adapters. Lift it into Phase 6's shared
// Kubernetes layer rather than duplicating the harness three more times here.

type recordedResource struct {
	typeToken    string
	name         string
	dependencies []string
	inputs       resource.PropertyMap
}

type runtimeMocks struct {
	mu        sync.Mutex
	resources []recordedResource
}

func (m *runtimeMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	rec := recordedResource{typeToken: args.TypeToken, name: args.Name, inputs: args.Inputs.Copy()}
	if args.RegisterRPC != nil {
		rec.dependencies = append([]string(nil), args.RegisterRPC.GetDependencies()...)
	}
	m.mu.Lock()
	m.resources = append(m.resources, rec)
	m.mu.Unlock()

	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "ovh:CloudProject/kube:Kube":
		state["name"] = resource.NewStringProperty(args.Name)
		state["kubeconfig"] = resource.NewStringProperty("apiVersion: v1\nkind: Config\n")
		state["url"] = resource.NewStringProperty("https://k8s.example.ovh.net")
		state["status"] = resource.NewStringProperty("READY")
	case "ovh:CloudProject/kubeNodePool:KubeNodePool":
		state["name"] = resource.NewStringProperty(args.Name)
		state["status"] = resource.NewStringProperty("READY")
	case "kubernetes:core/v1:Service":
		state["status"] = resource.NewObjectProperty(resource.PropertyMap{
			"loadBalancer": resource.NewObjectProperty(resource.PropertyMap{
				"ingress": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"ip": resource.NewStringProperty("51.1.2.3"),
					}),
				}),
			}),
		})
	}
	return args.Name + "-id", state, nil
}

func (*runtimeMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (m *runtimeMocks) one(t *testing.T, typeToken, nameSuffix string) recordedResource {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rec := range m.resources {
		if rec.typeToken == typeToken && strings.HasSuffix(rec.name, nameSuffix) {
			return rec
		}
	}
	t.Fatalf("resource %s ending in %q not found (graph shape changed)", typeToken, nameSuffix)
	return recordedResource{}
}

func (m *runtimeMocks) byType(typeToken string) []recordedResource {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]recordedResource, 0)
	for _, rec := range m.resources {
		if rec.typeToken == typeToken {
			result = append(result, rec)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

func assertDependsOn(t *testing.T, rec recordedResource, substrings ...string) {
	t.Helper()
	for _, want := range substrings {
		found := false
		for _, dep := range rec.dependencies {
			if strings.Contains(dep, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s %s missing dependency containing %q (ordering regressed); deps=%v",
				rec.typeToken, rec.name, want, rec.dependencies)
		}
	}
}

func validArgs() Args {
	return Args{
		ServiceName:         "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		Region:              "GRA9",
		ProjectName:         "shop",
		Environment:         "preview",
		Image:               "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		ApplicationMode:     "integrated",
		WebRuntime:          "nginx-fpm",
		DatabaseWriter:      pulumi.String("mysql://db.example.ovh.net:3306"),
		DatabaseName:        "magento",
		DatabaseUsername:    "magento",
		DatabasePassword:    pulumi.String("generated-password-value-32chars"),
		CacheEndpoint:       pulumi.String("redis://cache.example.ovh.net:6379"),
		SessionEndpoint:     pulumi.String(""),
		EncryptionKeySecret: "encryption-key",
		DesiredWebReplicas:  1,
		NodeFlavor:          "b3-8",
		NodeCount:           1,
		AvailabilityZones:   []string{"gra9"},
	}
}

func deploy(t *testing.T, args Args) *runtimeMocks {
	t.Helper()
	mocks := &runtimeMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop", args)
		return err
	}, pulumi.WithMocks("magelift", "shop-preview", mocks))
	if err != nil {
		t.Fatal(err)
	}
	return mocks
}

func TestMKSNodePoolDependencyOrdering(t *testing.T) {
	t.Parallel()
	mocks := deploy(t, validArgs())

	pool := mocks.one(t, "ovh:CloudProject/kubeNodePool:KubeNodePool", "-pool")
	assertDependsOn(t, pool, "-cluster")

	provider := mocks.one(t, "pulumi:providers:kubernetes", "-k8s")
	assertDependsOn(t, provider, "-pool")

	web := mocks.one(t, "kubernetes:apps/v1:Deployment", "-web")
	assertDependsOn(t, web, "-pool")

	svc := mocks.one(t, "kubernetes:core/v1:Service", "-web-svc")
	assertDependsOn(t, svc, "-pool", "-web")

	cron := mocks.one(t, "kubernetes:apps/v1:Deployment", "-cron")
	assertDependsOn(t, cron, "-pool")
}

func TestMKSNodePoolsDistributeAcrossAvailabilityZones(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.MKSPlan = "standard"
	args.NodeCount = 5
	args.AvailabilityZones = []string{"eu-west-par-a", "eu-west-par-b", "eu-west-par-c"}
	mocks := deploy(t, args)

	pools := mocks.byType("ovh:CloudProject/kubeNodePool:KubeNodePool")
	if len(pools) != 3 {
		t.Fatalf("created %d node pools, want 3: %#v", len(pools), pools)
	}
	wantNodes := []int{2, 2, 1}
	for index, pool := range pools {
		zones := pool.inputs["availabilityZones"].ArrayValue()
		if len(zones) != 1 {
			t.Fatalf("pool %d availability zones = %v, want one zone", index, zones)
		}
		if got := zones[0].StringValue(); got != args.AvailabilityZones[index] {
			t.Fatalf("pool %d availability zone = %q, want %q", index, got, args.AvailabilityZones[index])
		}
		if got := pool.inputs["desiredNodes"].NumberValue(); got != float64(wantNodes[index]) {
			t.Fatalf("pool %d desired nodes = %.0f, want %d", index, got, wantNodes[index])
		}
	}
}

func TestPrivateNetworkConfigurationUsesDocumentedDHCPGatewayMode(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.NetworkID = pulumi.String("regional-network-id")
	args.SubnetID = pulumi.String("nodes-subnet-id")
	args.PrivateNetworkRoutingAsDefault = true
	mocks := deploy(t, args)

	cluster := mocks.one(t, "ovh:CloudProject/kube:Kube", "-cluster")
	configuration, ok := cluster.inputs["privateNetworkConfiguration"]
	if !ok || !configuration.IsObject() {
		t.Fatalf("cluster is missing private network configuration: %v", cluster.inputs)
	}
	fields := configuration.ObjectValue()
	if got := fields["defaultVrackGateway"].StringValue(); got != "" {
		t.Fatalf("defaultVrackGateway = %q, want empty DHCP-gateway selector", got)
	}
	if got := fields["privateNetworkRoutingAsDefault"].BoolValue(); !got {
		t.Fatal("privateNetworkRoutingAsDefault = false, want true")
	}
}

func TestFloatingIPModeAttachesNodePoolIPsWithoutCustomGateway(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.NetworkID = pulumi.String("regional-network-id")
	args.SubnetID = pulumi.String("nodes-subnet-id")
	args.AttachFloatingIPs = true
	mocks := deploy(t, args)

	cluster := mocks.one(t, "ovh:CloudProject/kube:Kube", "-cluster")
	if _, ok := cluster.inputs["privateNetworkConfiguration"]; ok {
		t.Fatalf("floating-IP mode must not set privateNetworkConfiguration: %v", cluster.inputs)
	}

	pool := mocks.one(t, "ovh:CloudProject/kubeNodePool:KubeNodePool", "-pool")
	configuration, ok := pool.inputs["attachFloatingIps"]
	if !ok || !configuration.IsObject() {
		t.Fatalf("node pool is missing attachFloatingIps: %v", pool.inputs)
	}
	if got := configuration.ObjectValue()["enabled"].BoolValue(); !got {
		t.Fatal("attachFloatingIps.enabled = false, want true")
	}
}
