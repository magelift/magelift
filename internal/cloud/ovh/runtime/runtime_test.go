package runtime

import (
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
}

type runtimeMocks struct {
	mu        sync.Mutex
	resources []recordedResource
}

func (m *runtimeMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	rec := recordedResource{typeToken: args.TypeToken, name: args.Name}
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
		CacheEndpoint:       pulumi.String("redis://cache.example.ovh.net:6379"),
		SessionEndpoint:     pulumi.String(""),
		EncryptionKeySecret: "encryption-key",
		DesiredWebReplicas:  1,
		NodeFlavor:          "b3-8",
		NodeCount:           1,
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
